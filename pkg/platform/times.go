package platform

import (
	"os"
	"syscall"
	"time"
)

// GetFileTimes returns the atime, mtime, and ctime of a file.
func GetFileTimes(path string) (atime, mtime, ctime time.Time, err error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, err
	}

	stat := fileInfo.Sys().(*syscall.Stat_t)

	// Stat_t time field names differ per OS; statTimespecs is defined per platform.
	atimespec, mtimespec, ctimespec := statTimespecs(stat)
	atime = time.Unix(atimespec.Unix())
	mtime = time.Unix(mtimespec.Unix())
	ctime = time.Unix(ctimespec.Unix())

	return atime, mtime, ctime, nil
}
