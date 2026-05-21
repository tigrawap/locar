package engine

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"github.com/tigrawap/locar/pkg/platform"
)

const direntNameOffset = uint64(unsafe.Offsetof(syscall.Dirent{}.Name))

var timeoutError = errors.New("timed out")

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
			if e.delete || e.deleteAll {
				batch := make([]string, len(results))
				for i := range results {
					batch[i] = results[i].name
				}
				e.deleteBatches <- batch
			}
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
				e.AddDir(fullpath)
			}

			if omittedByInclude {
				continue MAINLOOP
			}

			switch dirent.Type {
			case syscall.DT_DIR:
				if e.includeDirs || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath + string(filepath.Separator), platform.GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			case syscall.DT_REG:
				if e.includeFiles || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, platform.GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			case syscall.DT_LNK:
				if e.includeLinks || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, platform.GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			case syscall.DT_SOCK:
				if e.includeSocket || e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, platform.GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				}
			default:
				if e.includeAny {
					if e.atimeOlderThan != 0 || e.atimeNewerThan != 0 || e.ctimeOlderThan != 0 || e.ctimeNewerThan != 0 || e.mtimeOlderThan != 0 || e.mtimeNewerThan != 0 || e.withTimes {
						result, ok, err := e.checkFileTimeConditions(fullpath)
						if err != nil || !ok {
							continue
						}
						results = append(results, result)
					} else {
						results = append(results, Result{fullpath, platform.GetIno(dirent), time.Time{}, time.Time{}, time.Time{}})
					}
				} else {
					log.Printf("Skipped record: %s iNode<%d>[type:%s]\n", fullpath, platform.GetIno(dirent), entryType(dirent.Type))
				}
			}
			if len(results) == 1024 {
				clearResults()
			}
		}
	}
}
