package engine

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

func (e *Explorer) dumpResults() {
	defer func() { e.doneTails <- nullv }()
	var outputBuffer bytes.Buffer
	var result Result

	var writeSliceLock sync.WaitGroup
	var writeLock sync.Mutex
	resultsWorkers := semaphore.NewWeighted(int64(e.resultsThreads))

	flush := func() {
		if !e.quiet {
			fmt.Fprint(e.out, outputBuffer.String())
		}
		outputBuffer.Truncate(0)
	}
	defer flush()
	defer writeSliceLock.Wait()
	ctx := context.TODO()

	writeData := func(data []Result) {
		writeLock.Lock()
		for _, result = range data {
			atomic.AddInt64(&e.found, 1)
			if !e.quiet {
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
				if e.withSizes {
					fileStat, err := os.Lstat(result.name)
					if err != nil {
						log.Println(err)
						outputBuffer.WriteString("0")
					} else {
						outputBuffer.WriteString(fmt.Sprintf(" %d", fileStat.Size()))
					}
				}
				if e.withTimes {
					outputBuffer.WriteString(fmt.Sprintf(" %d %d %d", result.atime.Unix(), result.mtime.Unix(), result.ctime.Unix()))
				}
				outputBuffer.WriteString("\n")
				if outputBuffer.Len() > 4*1024 {
					flush()
				}
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
		if e.resultStore.length.Load() != 0 {
			e.resultStore.Lock()
			data := e.resultStore.store
			e.resultStore.store = make([]Result, 0)
			e.resultStore.length.Store(0)
			e.resultStore.Unlock()
			flushSlice(data)
		} else {
			time.Sleep(10 * time.Microsecond)
			if e.doneDirectoriesFlag.Load() && e.resultStore.length.Load() == 0 {
				return
			}
		}
	}
}

func (e *Explorer) addResults(results []Result) {
	e.resultStore.Lock()
	e.resultStore.store = append(e.resultStore.store, results...)
	e.resultStore.length.Store(int64(len(e.resultStore.store)))
	e.resultStore.Unlock()
}

func (e *Explorer) requestStoreFlush() {
	select {
	case e.flushStoreRequest <- nullv:
	default:
		return
	}
}

func (e *Explorer) flushStoreLoop() {
	e.flushStoreRequest = make(controlChannel)
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
