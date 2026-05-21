//go:build linux || darwin

package platform

import "syscall"

func GetIno(dirent *syscall.Dirent) uint64 {
	return dirent.Ino
}
