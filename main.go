package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sync/semaphore"

	"github.com/gobwas/glob"
	"github.com/jessevdk/go-flags"
)

/*
For reference.

type Dirent struct {
	Ino       uint64
	Off       int64
	Reclen    uint16
	Type      uint8
	Name      [256]int8
	Pad_cgo_0 [5]byte
}
*/

type null struct{}

var nullv = null{}

type controlChannel chan null

const direntNameOffset = uint64(unsafe.Offsetof(syscall.Dirent{}.Name))

var timeoutError = errors.New("timed out")
var Version = "v0.1.0"

type dirStore struct {
	sync.Mutex
	store []string
}

type resultStore struct {
	sync.Mutex
	store  []Result
	length int
}

type Result struct {
	name  string
	ino   uint64
	atime time.Time
	mtime time.Time
	ctime time.Time
}

// TimeCondition represents conditions to filter by a specific time type
type TimeCondition struct {
	OlderThan time.Duration
	NewerThan time.Duration
}

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
	doneDirectoriesFlag bool
	ctx                 context.Context
	excludes            []glob.Glob
	includes            []glob.Glob
	flushStoreRequest   controlChannel
	threads             int64
	rateLimiter         chan null
	buffPool            sync.Pool
	resultsPool         sync.Pool
	debugInFlight       int64

	atimeOlderThan time.Duration
	atimeNewerThan time.Duration
	mtimeOlderThan time.Duration
	mtimeNewerThan time.Duration
	ctimeOlderThan time.Duration
	ctimeNewerThan time.Duration

	delete         bool
	deleteAll      bool
	includeDirs    bool
	includeFiles   bool
	includeLinks   bool
	includeSocket  bool
	includeAny     bool
	started        bool
	resultsThreads int
	withSizes      bool
	withTimes      bool
}

func NewExplorer(ctx context.Context) *Explorer {
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
	return e
}
func (e *Explorer) SetIncludedTypes(types []string) {
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

func (e *Explorer) SetThreads(threads int) {
	if e.started {
		log.Fatalln("Can't change number of threads after start")
	}
	e.threads = int64(threads)
	chanBuff := e.threads
	if chanBuff < 4096 {
		chanBuff = 4096
	}
	e.directories = make(chan string, chanBuff)
	//go func() {
	//	for {
	//		time.Sleep(100 * time.Millisecond)
	//		log.Println(e.debugInFlight)
	//	}
	//}()
}

// checkTimeCondition checks if a given timestamp meets the specified TimeCondition
func checkTimeCondition(timestamp time.Time, condition TimeCondition) bool {
	now := time.Now()

	// Check "older than" condition
	if condition.OlderThan != 0 {
		targetTime := now.Add(-condition.OlderThan)
		if timestamp.After(targetTime) {
			return false
		}
	}

	// Check "newer than" condition
	if condition.NewerThan != 0 {
		targetTime := now.Add(-condition.NewerThan)
		if timestamp.Before(targetTime) {
			return false
		}
	}

	return true
}

// checkFileTimeConditions retrieves file times and checks them against the given conditions
func (e *Explorer) checkFileTimeConditions(fullpath string) (Result, bool, error) {
	// Retrieve atime, ctime, and mtime of the file
	atime, mtime, ctime, err := GetFileTimes(fullpath)
	if err != nil {
		log.Println(err)
		return Result{}, false, err
	}

	// Create time conditions based on the Explorer's settings
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

	// All conditions passed
	return Result{
		name:  fullpath,
		atime: atime,
		mtime: mtime,
		ctime: ctime,
	}, true, nil
}

// createTimeConditions creates and returns the TimeCondition structs for time
func createTimeConditions(olderThanInput, newerThanInput *time.Duration) (timeCond TimeCondition) {
	// Define default durations
	defaultOlderThan := 0 * time.Second
	defaultNewerThan := 0 * time.Second

	// Use input if provided, otherwise use default values
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

// GetFileTimes returns the atime, mtime, and ctime of a file
func GetFileTimes(path string) (atime, mtime, ctime time.Time, err error) {

	fileInfo, err := os.Stat(path)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, err
	}

	stat := fileInfo.Sys().(*syscall.Stat_t)

	// Extract access time (atime)
	atime = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)

	// Extract modification time (mtime)
	mtime = time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec)

	// Extract change time (ctime)
	ctime = time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)

	return atime, mtime, ctime, nil
}

func (e *Explorer) dumpResults() {
	defer func() { e.doneTails <- nullv }()
	var done int64
	var outputBuffer bytes.Buffer
	var result Result

	var writeSliceLock sync.WaitGroup
	var writeLock sync.Mutex
	resultsWorkers := semaphore.NewWeighted(int64(e.resultsThreads))

	flush := func() {
		fmt.Print(outputBuffer.String())
		outputBuffer.Truncate(0)
	}
	defer flush()
	defer writeSliceLock.Wait()
	ctx := context.TODO()

	writeData := func(data []Result) {
		writeLock.Lock()
		for _, result = range data {
			done++
			if e.raw {
				outputBuffer.WriteString(fmt.Sprintf("%#v", result.name))
			} else {
				outputBuffer.WriteString(result.name)
			}
			if e.inodes {
				outputBuffer.WriteString(" " + strconv.FormatUint(result.ino, 10))
			}
			if e.inodesHex {
				outputBuffer.WriteString(" 0x" + strconv.FormatUint(result.ino, 16))
			}
			// TODO: Once adding another stat-based processor,
			// 		 put this into interface for processing and put on outer level
			//		 But need to make sure not to increase Result struct and do it on the fly
			if e.withSizes {
				fileStat, err := os.Lstat(result.name)
				if err != nil {
					log.Println(err)
					outputBuffer.WriteString("0")
				} else {
					outputBuffer.WriteString(fmt.Sprintf(" %d", fileStat.Size()))
				}
			}
			// Show atime, mtime, ctime
			if e.withTimes {
				outputBuffer.WriteString(fmt.Sprintf(" %d %d %d", result.atime.Unix(), result.mtime.Unix(), result.ctime.Unix()))
			}

			// Delete ignore non empty dir
			if e.delete {
				err := os.Remove(result.name)
				if err != nil {
					log.Printf("Delete failed: %s - Error: %v\n", result.name, err)
					outputBuffer.WriteString(" [delete_failed]")
				} else {
					log.Printf("Delete success: %s\n", result.name)
					outputBuffer.WriteString(" [delete_success]")
				}
			}

			// Delete not ignore non empty dir
			if e.deleteAll {
				err := os.RemoveAll(result.name)
				if err != nil {
					log.Printf("Delete failed: %s - Error: %v\n", result.name, err)
					outputBuffer.WriteString(" [delete_failed]")
				} else {
					log.Printf("Delete success: %s\n", result.name)
					outputBuffer.WriteString(" [delete_success]")
				}
			}
			outputBuffer.WriteString("\n")
			if outputBuffer.Len() > 4*1024 {
				flush()
			}
		}
		writeLock.Unlock()
		writeSliceLock.Done()
		resultsWorkers.Release(1)
	}

	flushSlice := func(data []Result) {
		writeSliceLock.Add(1)
		_ = resultsWorkers.Acquire(ctx, 1)
		go writeData(data)
	}

	for {
		if e.resultStore.length != 0 {
			e.resultStore.Lock()
			flushSlice(e.resultStore.store)
			e.resultStore.store = make([]Result, 0)
			e.resultStore.length = 0
			e.resultStore.Unlock()
		} else {
			time.Sleep(10 * time.Microsecond)
			if e.resultStore.length == 0 {
				e.resultStore.Lock()
				if e.resultStore.length == 0 && e.doneDirectoriesFlag {
					e.resultStore.Unlock()
					return
				}
				e.resultStore.Unlock()
			}
		}
	}
}

func (e *Explorer) requestStoreFlush() {
	select {
	case e.flushStoreRequest <- nullv:
	default:
		return
	}
}

func (e *Explorer) flushStoreLoop() {
	for {
		select {
		case <-e.flushStoreRequest:
		case <-time.After(10 * time.Millisecond):
		}
		e.dirStore.Lock()
		numFlushed := 0
	FLUSHLOOP:
		for {
			if len(e.dirStore.store)-numFlushed > 0 {
				select {
				case e.directories <- e.dirStore.store[len(e.dirStore.store)-1-numFlushed]:
					numFlushed++
				default:
					break FLUSHLOOP
				}
			} else {
				break
			}
		}
		if numFlushed > 0 {
			e.dirStore.store = e.dirStore.store[:len(e.dirStore.store)-numFlushed]
		}
		e.dirStore.Unlock()
	}
}

func (e *Explorer) addResults(results []Result) {
	e.resultStore.Lock()
	e.resultStore.store = append(e.resultStore.store, results...)
	e.resultStore.length = len(e.resultStore.store)
	e.resultStore.Unlock()
}

func (e *Explorer) addDir(dir string) {
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

func (e *Explorer) start() {
	if e.threads == 0 {
		e.SetThreads(1)
	}
	e.started = true
	go e.dumpResults()
	go e.flushStoreLoop()
	e.rateLimiter = make(chan null, e.threads)
	go func() {
		for directory := range e.directories {
			e.rateLimiter <- nullv
			go func(dir string) {
				e.readdir(dir)
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
		e.doneDirectoriesFlag = true
		<-e.doneTails
		allDone <- nullv
	}()
	return allDone
}

func (e *Explorer) isNotIncluded(path string) bool {
	if len(e.includes) != 0 {
		for _, include := range e.includes {
			if include.Match(path) {
				return false
			}
		}
		return true
	}
	return false
}

func (e *Explorer) isExcluded(path string) bool {
	for _, exclude := range e.excludes {
		if exclude.Match(path) {
			return true
		}
	}
	return false
}

func (e *Explorer) readdir(dir string) {
	if e.ctx.Err() != nil {
		return
	}
	file, err := OpenWithDeadline(dir, e.timeout)
	if err != nil {
		if err == timeoutError {
			err = errors.New(fmt.Sprintf("dir open: %s", err.Error()))
		}
		if e.resilient {
			log.Println(dir, err)
			return
		} else {
			log.Fatalln(dir, err)
		}
	}
	defer file.Close()
	fd := int(file.Fd())

	buff := e.buffPool.Get().([]byte)
	defer e.buffPool.Put(buff)

	results := e.resultsPool.Get().([]Result)
	defer e.resultsPool.Put(results)

	clearResults := func() {
		if len(results) != 0 {
			e.addResults(results)
		}
		results = results[:0]
	}
	defer clearResults()

	var name []byte
	var fullpath string
	var omittedByInclude bool
	for e.ctx.Err() == nil {
		omittedByInclude = false
		dirlength, err := ReadDirentWithDeadline(fd, buff, e.timeout)
		if err != nil {
			if err == timeoutError {
				err = errors.New(fmt.Sprintf("readdir: %s", err.Error()))
			}
			if e.resilient {
				log.Println(dir, err)
				return
			} else {
				log.Fatalln(dir, err)
			}
		}
		if dirlength == 0 {
			break
		}
		var offset uint64
	MAINLOOP:
		for offset = 0; offset < uint64(dirlength); {
			dirent := (*syscall.Dirent)(unsafe.Pointer(&buff[offset]))

			for i, c := range buff[offset+direntNameOffset:] {
				if c == 0 {
					name = buff[offset+direntNameOffset : offset+direntNameOffset+uint64(i)]
					break
				}
			}
			offset += uint64(dirent.Reclen)
			// Special cases for common things:
			nameLen := len(name)
			if nameLen == 1 && name[0] == '.' {
				continue
			} else if nameLen == 2 && name[0] == '.' && name[1] == '.' {
				continue
			}

			fullpath = filepath.Join(dir, string(name))

			isDir := dirent.Type == syscall.DT_DIR
			omittedByInclude = e.isNotIncluded(fullpath)
			if omittedByInclude && !isDir {
				continue MAINLOOP
			}
			if e.isExcluded(fullpath) {
				continue MAINLOOP
			}
			if isDir {
				e.addDir(fullpath)
			}

			if omittedByInclude {
				continue MAINLOOP
			}

			switch dirent.Type {
			case syscall.DT_DIR:
				if e.includeDirs || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						// Check times and get the Result struct
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath + string(filepath.Separator), GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			case syscall.DT_REG:
				if e.includeFiles || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						// Check times and get the Result struct
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			case syscall.DT_LNK:
				if e.includeLinks || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						// Check times and get the Result struct
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			case syscall.DT_SOCK:
				if e.includeSocket || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						// Check times and get the Result struct
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			default:
				if e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						// Check times and get the Result struct
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				} else {
					log.Printf("Skipped record: %s iNode<%d>[type:%s]\n", fullpath, GetIno(dirent), entryType(dirent.Type))
				}
			}
			if len(results) == 1024 {
				clearResults()
			}
		}
	}
}

type Options struct {
	Resilient      bool          `long:"resilient" description:"DEPRECATED and ignored, resilient is a default, use --stop-on-error if it is undesired behaviour"`
	StopOnError    bool          `long:"stop-on-error" description:"Aborts scan on any error"`
	Inodes         bool          `long:"inodes" description:"Output inodes (decimal) along with filenames"`
	InodesHex      bool          `long:"inodes-hex" description:"Output inodes (hexadecimal) along with filenames"`
	Raw            bool          `long:"raw" description:"Output filenames as escaped strings"`
	Threads        int           `short:"j" long:"jobs" description:"Number of jobs(threads)" default:"128"`
	WithSizes      bool          `long:"with-size" description:"Output file sizes along with filenames"`
	WithTimes      bool          `long:"with-times" description:"Output file with atime, mtime, ctime along with filenames"`
	AtimeOlderThan time.Duration `long:"atime-older" description:"Filter files by access time older than this duration (e.g., 24h5m25s)" default:"0s"`
	AtimeNewerThan time.Duration `long:"atime-newer" description:"Filter files by access time newer than this duration (e.g., 24h5m25s)" default:"0s"`
	MtimeOlderThan time.Duration `long:"mtime-older" description:"Filter files by modification time older than this duration (e.g., 24h5m25s)" default:"0s"`
	MtimeNewerThan time.Duration `long:"mtime-newer" description:"Filter files by modification time newer than this duration (e.g., 24h5m25s)" default:"0s"`
	CtimeOlderThan time.Duration `long:"ctime-older" description:"Filter files by change time older than this duration (e.g., 24h5m25s)" default:"0s"`
	CtimeNewerThan time.Duration `long:"ctime-newer" description:"Filter files by change time newer than this duration (e.g., 24h5m25s)" default:"0s"`
	ResultThreads  int           `long:"result-jobs" description:"Number of jobs for processing results, like doing stats to get file sizes" default:"128"`
	Delete         bool          `long:"delete" description:"Delete found files. Non empty directories will be ignored"`
	DeleteAll      bool          `long:"delete-all" description:"Delete found files. Non empty directories will be removed with ALL their contents!!!"`
	Version        bool          `short:"v" long:"version" description:"Show version"`

	Exclude []string `short:"x" long:"exclude" description:"Patterns to exclude. Can be specified multiple times"`
	Filter  []string `short:"f" long:"filter" description:"Patterns to filter by. Can be specified multiple times"`

	Type []string `short:"t" long:"type" default:"file" default:"dir" default:"link" default:"socket" description:"Search entries of specific type \nPossible values: file, dir, link, socket, all. Can be specified multiple times"`

	Args struct {
		Directories []string `positional-arg-name:"directories" description:"Directories to search, using current directory if missing"`
	} `positional-args:"yes"`

	Timeout time.Duration `long:"timeout" default:"5m" description:"Timeout for readdir operations. Error will be reported, but os thread will be kept hanging"`
}

func getOpts() *Options {
	opts := &Options{}
	_, err := flags.Parse(opts)
	if opts.Version {
		fmt.Printf("%s version %s\n", path.Base(os.Args[0]), Version)
		os.Exit(0)
	}
	if flagsErr, ok := err.(*flags.Error); ok {
		if flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		log.Fatalln(flagsErr)
		os.Exit(1)
	}

	if err != nil {
		log.Fatalln(err.Error())
	}

	if len(opts.Args.Directories) == 0 {
		opts.Args.Directories = []string{"."}
	}
	return opts
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := getOpts()

	//TODO: Refactor Explorer to Lib with proper public API and sane defaults, so none of this calls will be necessary
	explorer := NewExplorer(ctx)
	explorer.resilient = !opts.StopOnError
	explorer.SetIncludedTypes(opts.Type)
	explorer.SetThreads(opts.Threads)
	explorer.inodes = opts.Inodes
	explorer.inodesHex = opts.InodesHex
	explorer.raw = opts.Raw
	explorer.timeout = opts.Timeout
	explorer.resultsThreads = opts.ResultThreads
	explorer.withSizes = opts.WithSizes
	explorer.withTimes = opts.WithTimes
	explorer.atimeOlderThan = opts.AtimeOlderThan
	explorer.atimeNewerThan = opts.AtimeNewerThan
	explorer.mtimeOlderThan = opts.MtimeOlderThan
	explorer.mtimeNewerThan = opts.MtimeNewerThan
	explorer.ctimeOlderThan = opts.CtimeOlderThan
	explorer.ctimeNewerThan = opts.CtimeNewerThan
	explorer.delete = opts.Delete
	explorer.deleteAll = opts.DeleteAll

	for _, exclude := range opts.Exclude {
		explorer.excludes = append(explorer.excludes, glob.MustCompile(exclude))
	}
	for _, filter := range opts.Filter {
		explorer.includes = append(explorer.includes, glob.MustCompile(filter))
	}

	for _, directory := range opts.Args.Directories {
		seed := ExpandHomePath(directory)
		if err := IsDir(seed); err != nil {
			log.Fatalln(seed, err)
		}
		explorer.addDir(seed)
	}

	go func() {
		<-quitOnInterrupt()
		cancel()
		<-time.After(100 * time.Millisecond)
		os.Exit(130)
	}()

	explorer.start()
	//TODO: Check how much pprof adds to the binary, if not much - listen for a user signal to dump goroutines
	//go func() {
	//	<-time.After(5 * time.Second)
	//	pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	//}()
	<-explorer.done()
	if ctx.Err() == context.Canceled {
		os.Exit(130)
	}

}

func ReadDirentWithDeadline(fd int, buf []byte, timeout time.Duration) (n int, err error) {
	doneEvent := make(controlChannel)
	go func() {
		n, err = syscall.ReadDirent(fd, buf)
		doneEvent <- nullv
	}()
	select {
	case <-time.After(timeout):
		return 0, timeoutError
	case <-doneEvent:
		return
	}
}

func OpenWithDeadline(name string, timeout time.Duration) (f *os.File, e error) {
	doneEvent := make(controlChannel)
	go func() {
		f, e = os.Open(name)
		doneEvent <- nullv
	}()
	select {
	case <-time.After(timeout):
		return nil, timeoutError
	case <-doneEvent:
		return
	}
}

func entryType(direntType uint8) string {
	switch direntType {
	case syscall.DT_DIR:
		return "dir"
	case syscall.DT_REG:
		return "file"
	case syscall.DT_LNK:
		return "link"
	case syscall.DT_SOCK:
		return "socket"
	case syscall.DT_CHR:
		return "char"
	default:
		return fmt.Sprintf("unknown(%v)", direntType)
	}
}
