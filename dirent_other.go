//go:build linux || darwin
// +build linux darwin

package main

import "syscall"

func GetIno(dirent *syscall.Dirent) uint64 {
	return dirent.Ino
}
