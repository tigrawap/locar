package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

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

type dirStore struct {
	sync.RWMutex
	store []string
}

type Explorer struct {
	results           chan string
	directories       chan string
	dirStore          dirStore
	inFlight          int64
	resilient         bool
	doneTails         controlChannel
	doneDirectories   controlChannel
	ctx               context.Context
	excludes          []glob.Glob
	includes          []glob.Glob
	flushStoreRequest controlChannel
	threads           int64
	buffPool          sync.Pool

	includeDirs  bool
	includeFiles bool
	includeLinks bool
	includeAny   bool
	started      bool
}

func NewExplorer(ctx context.Context) *Explorer {
	e := &Explorer{}
	e.results = make(chan string, 1024)
	e.doneTails = make(controlChannel)
	e.doneDirectories = make(controlChannel)
	e.ctx = ctx
	e.buffPool.New = func() interface{} {
		return make([]byte, 64*1024)
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
	e.directories = make(chan string, chanBuff)
}

func (e *Explorer) dumpResults() {
	defer func() { e.doneTails <- nullv }()
	for {
		select {
		case file, ok := <-e.results:
			if ok {
				fmt.Println(file)
			} else {
				return
			}
		case <-e.ctx.Done():
			return
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
	rateLimiter := make(chan null, e.threads)
	go func() {
		for directory := range e.directories {
			rateLimiter <- nullv
			go func(dir string) {
				e.readdir(dir)
				<-rateLimiter
				current := atomic.AddInt64(&e.inFlight, -1)
				if current == 0 {
					close(e.directories)
				}
			}(directory)
		}
		e.doneDirectories <- nullv
		close(e.results)
	}()
}

func (e *Explorer) done() controlChannel {
	allDone := make(controlChannel)
	go func() {
		<-e.doneDirectories
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
	file, err := os.Open(dir)
	if err != nil {
		if e.resilient {
			log.Println(err)
			return
		} else {
			log.Fatalln(err)
		}
	}
	defer file.Close()
	fd := int(file.Fd())

	buff := e.buffPool.Get().([]byte)
	defer e.buffPool.Put(buff)
	var name []byte
	var fullpath string
	var omittedByInclude bool
	for e.ctx.Err() == nil {
		omittedByInclude = false
		dirlength, err := syscall.ReadDirent(fd, buff)
		if dirlength == 0 {
			break
		}
		if err != nil {
			log.Fatalln(err)
		}
		var offset uint64
	MAINLOOP:
		for offset = 0; offset < uint64(dirlength); {
			if err != nil {
				log.Fatalln(err)
			}
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
					e.results <- fullpath + string(filepath.Separator)
				}
			case syscall.DT_REG:
				if e.includeFiles || e.includeAny {
					e.results <- fullpath
				}
			case syscall.DT_LNK:
				if e.includeLinks || e.includeAny {
					e.results <- fullpath
				}
			default:
				if e.includeAny {
					e.results <- fullpath
				} else {
					log.Printf("Skipped record: %s[%s]\n", string(name), entryType(dirent.Type))
				}
			}
		}
	}
}

type Options struct {
	Resilient bool `long:"resilient" description:"Do not stop on errors, instead print to stderr"`
	Threads   int  `short:"j" long:"jobs" description:"Number of jobs(threads)" default:"128"`

	Exclude []string `short:"x" long:"exclude" description:"Patterns to exclude. Can be specified multiple times"`
	Filter  []string `short:"f" long:"filter" description:"Patterns to filter by. Can be specified multiple times"`

	Type []string `short:"t" long:"type" default:"file" default:"dir" default:"link" description:"Search entries of specific type \nPossible values: file, dir, link, all. Can be specified multiple times"`

	Args struct {
		Directories []string `positional-arg-name:"directories" description:"Directories to search, using current directory if missing"`
	} `positional-args:"yes"`
}

func getOpts() *Options {
	opts := &Options{}
	_, err := flags.Parse(opts)

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
	explorer.resilient = opts.Resilient
	explorer.SetIncludedTypes(opts.Type)
	explorer.SetThreads(opts.Threads)

	for _, exclude := range opts.Exclude {
		explorer.excludes = append(explorer.excludes, glob.MustCompile(exclude))
	}
	for _, filter := range opts.Filter {
		explorer.includes = append(explorer.includes, glob.MustCompile(filter))
	}

	for _, directory := range opts.Args.Directories {
		seed := ExpandHomePath(directory)
		if err := IsDir(seed); err != nil {
			log.Fatalln(err)
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

func entryType(direntType uint8) string {
	switch direntType {
	case syscall.DT_DIR:
		return "dir"
	case syscall.DT_REG:
		return "file"
	case syscall.DT_LNK:
		return "link"
	case syscall.DT_CHR:
		return "char"
	default:
		return "unknown"
	}
}
