//go:build darwin || freebsd

package platform

import "syscall"

// statTimespecs extracts atime, mtime, and ctime from a BSD-family stat struct.
// macOS and FreeBSD name these fields *spec, unlike Linux's *tim.
func statTimespecs(stat *syscall.Stat_t) (atime, mtime, ctime syscall.Timespec) {
	return stat.Atimespec, stat.Mtimespec, stat.Ctimespec
}
