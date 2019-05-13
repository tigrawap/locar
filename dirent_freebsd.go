// +build freebsd

package main

import "syscall"

func GetIno(dirent *syscall.Dirent) uint64 {
	return dirent.Fileno
}
