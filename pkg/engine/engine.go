package engine

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/glob"

	"github.com/tigrawap/locar/pkg/platform"
)

type null struct{}

var nullv = null{}

type controlChannel chan null

type dirStore struct {
	sync.Mutex
	store []string
}

type resultStore struct {
	sync.Mutex
	store  []Result
	length atomic.Int64
}

// Result holds information about a found file entry.
type Result struct {
	name  string
	ino   uint64
	atime time.Time
	mtime time.Time
	ctime time.Time
}

// TimeCondition represents conditions to filter by a specific time type.
type TimeCondition struct {
	OlderThan time.Duration
	NewerThan time.Duration
}

// Explorer is the parallel filesystem search engine.
type Explorer struct {
	directories         chan string
	dirStore            dirStore
	resultStore         resultStore
	inFlight            int64
	resilient           bool
	inodes              bool
	inodesHex           bool
	raw                 bool
	timeout             time.Duration
	doneTails           controlChannel
	doneDirectories     controlChannel
	doneDirectoriesFlag atomic.Bool
	ctx                 context.Context
	excludes            []glob.Glob
	includes            []glob.Glob
	flushStoreRequest   controlChannel
	threads             int64
	rateLimiter         chan null
	buffPool            sync.Pool
	resultsPool         sync.Pool
	debugInFlight       int64

	found             int64
	activeDeletes     int64
	totalDeleted      int64
	totalDeleteFailed int64
	deleteBatches     chan []string
	deletesDone       controlChannel

	atimeOlderThan time.Duration
	atimeNewerThan time.Duration
	mtimeOlderThan time.Duration
	mtimeNewerThan time.Duration
	ctimeOlderThan time.Duration
	ctimeNewerThan time.Duration

	delete         bool
	deleteAll      bool
	quiet          bool
	includeDirs    bool
	includeFiles   bool
	includeLinks   bool
	includeSocket  bool
	includeAny     bool
	resultsThreads int
	withSizes      bool
	withTimes      bool
	out            io.Writer
}

// New creates a new Explorer with the given context and configuration.
func New(ctx context.Context, cfg Config) (*Explorer, error) {
	e := &Explorer{}
	e.doneTails = make(controlChannel)
	e.doneDirectories = make(controlChannel)
	e.ctx = ctx
	e.buffPool.New = func() interface{} {
		return make([]byte, 64*1024)
	}
	e.resultsPool.New = func() interface{} {
		return make([]Result, 0, 1024)
	}

	// Apply configuration with defaults.
	e.resilient = !cfg.StopOnError
	e.setIncludedTypes(cfg.Types)
	e.setThreads(cfg.Threads)
	e.inodes = cfg.Inodes
	e.inodesHex = cfg.InodesHex
	e.raw = cfg.Raw
	if cfg.Timeout <= 0 {
		e.timeout = 5 * time.Minute
	} else {
		e.timeout = cfg.Timeout
	}
	if cfg.ResultThreads <= 0 {
		e.resultsThreads = 128
	} else {
		e.resultsThreads = cfg.ResultThreads
	}
	e.withSizes = cfg.WithSizes
	e.withTimes = cfg.WithTimes
	e.atimeOlderThan = cfg.AtimeOlderThan
	e.atimeNewerThan = cfg.AtimeNewerThan
	e.mtimeOlderThan = cfg.MtimeOlderThan
	e.mtimeNewerThan = cfg.MtimeNewerThan
	e.ctimeOlderThan = cfg.CtimeOlderThan
	e.ctimeNewerThan = cfg.CtimeNewerThan
	e.delete = cfg.Delete
	e.deleteAll = cfg.DeleteAll
	e.quiet = cfg.Quiet
	if cfg.Output != nil {
		e.out = cfg.Output
	} else {
		e.out = os.Stdout
	}

	for _, pat := range cfg.Excludes {
		g, err := glob.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", pat, err)
		}
		e.excludes = append(e.excludes, g)
	}
	for _, pat := range cfg.Includes {
		g, err := glob.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("invalid filter pattern %q: %w", pat, err)
		}
		e.includes = append(e.includes, g)
	}

	return e, nil
}

func (e *Explorer) setIncludedTypes(types []string) {
	for _, t := range types {
		switch t {
		case "file":
			e.includeFiles = true
		case "dir":
			e.includeDirs = true
		case "link":
			e.includeLinks = true
		case "socket":
			e.includeSocket = true
		case "all":
			e.includeAny = true
		}
	}
}

func (e *Explorer) setThreads(threads int) {
	e.threads = int64(threads)
	chanBuff := e.threads
	if chanBuff < 4096 {
		chanBuff = 4096
	}
	e.directories = make(chan string, chanBuff)
}

// AddDir adds a directory to be scanned.
func (e *Explorer) AddDir(dir string) {
	inFlight := atomic.AddInt64(&e.inFlight, 1)
	select {
	case e.directories <- dir:
	default:
		e.dirStore.Lock()
		e.dirStore.store = append(e.dirStore.store, dir)
		if inFlight-int64(len(e.dirStore.store)) < e.threads && len(e.dirStore.store) > 0 {
			e.requestStoreFlush()
		}
		e.dirStore.Unlock()
	}
}

// Start begins the scan operation.
func (e *Explorer) Start() {
	if e.delete || e.deleteAll {
		deleteWorkers := e.resultsThreads
		if deleteWorkers < 1 {
			deleteWorkers = 1
		}
		e.deleteBatches = make(chan []string, 4096)
		e.deletesDone = make(controlChannel)
		go e.runDeleteWorkers(deleteWorkers)
		go e.deleteStatsLoop()
	}
	go e.dumpResults()
	go e.flushStoreLoop()
	e.rateLimiter = make(chan null, e.threads)
	go func() {
		for directory := range e.directories {
			e.rateLimiter <- nullv
			go func(dir string) {
				atomic.AddInt64(&e.debugInFlight, 1)
				e.readdir(dir)
				atomic.AddInt64(&e.debugInFlight, -1)
				<-e.rateLimiter
				current := atomic.AddInt64(&e.inFlight, -1)
				if current == 0 {
					close(e.directories)
				}
			}(directory)
		}
		e.doneDirectories <- nullv
	}()
}

func (e *Explorer) done() controlChannel {
	allDone := make(controlChannel)
	go func() {
		<-e.doneDirectories
		e.doneDirectoriesFlag.Store(true)
		if e.delete || e.deleteAll {
			close(e.deleteBatches)
		}
		<-e.doneTails
		if e.delete || e.deleteAll {
			<-e.deletesDone
		}
		allDone <- nullv
	}()
	return allDone
}

// Wait blocks until the scan and all processing is complete.
func (e *Explorer) Wait() {
	<-e.done()
}

// checkTimeCondition checks if a given timestamp meets the specified TimeCondition.
func checkTimeCondition(timestamp time.Time, condition TimeCondition) bool {
	now := time.Now()

	if condition.OlderThan != 0 {
		targetTime := now.Add(-condition.OlderThan)
		if timestamp.After(targetTime) {
			return false
		}
	}

	if condition.NewerThan != 0 {
		targetTime := now.Add(-condition.NewerThan)
		if timestamp.Before(targetTime) {
			return false
		}
	}

	return true
}

// createTimeConditions creates and returns the TimeCondition structs for time.
func createTimeConditions(olderThanInput, newerThanInput *time.Duration) (timeCond TimeCondition) {
	defaultOlderThan := 0 * time.Second
	defaultNewerThan := 0 * time.Second

	timeOlderThan := defaultOlderThan
	timeNewerThan := defaultNewerThan

	if olderThanInput != nil {
		timeOlderThan = *olderThanInput
	}
	if newerThanInput != nil {
		timeNewerThan = *newerThanInput
	}

	timeCond = TimeCondition{
		OlderThan: timeOlderThan,
		NewerThan: timeNewerThan,
	}

	return timeCond
}

func (e *Explorer) checkFileTimeConditions(fullpath string) (Result, bool, error) {
	atime, mtime, ctime, err := platform.GetFileTimes(fullpath)
	if err != nil {
		log.Println(err)
		return Result{}, false, err
	}

	atimeCond := createTimeConditions(&e.atimeOlderThan, &e.atimeNewerThan)
	ctimeCond := createTimeConditions(&e.ctimeOlderThan, &e.ctimeNewerThan)
	mtimeCond := createTimeConditions(&e.mtimeOlderThan, &e.mtimeNewerThan)

	if !checkTimeCondition(atime, atimeCond) {
		return Result{}, false, nil
	}
	if !checkTimeCondition(ctime, ctimeCond) {
		return Result{}, false, nil
	}
	if !checkTimeCondition(mtime, mtimeCond) {
		return Result{}, false, nil
	}

	return Result{
		name:  fullpath,
		atime: atime,
		mtime: mtime,
		ctime: ctime,
	}, true, nil
}
