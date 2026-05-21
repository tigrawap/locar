//go:build linux

package main

import "syscall"

// statTimespecs extracts atime, mtime, and ctime from a Linux stat struct.
func statTimespecs(stat *syscall.Stat_t) (atime, mtime, ctime syscall.Timespec) {
	return stat.Atim, stat.Mtim, stat.Ctim
}
