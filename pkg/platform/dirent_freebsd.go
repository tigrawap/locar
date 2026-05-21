//go:build freebsd

package platform

import "syscall"

func GetIno(dirent *syscall.Dirent) uint64 {
	return dirent.Fileno
}
