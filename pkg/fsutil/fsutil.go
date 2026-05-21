package fsutil

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
)

// IsDir checks whether filename is an existing directory.
func IsDir(filename string) error {
	fi, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return errors.New(filename + ": No such file or directory")
	} else if os.IsPermission(err) {
		return errors.New(filename + ": Permission denied")
	} else if err != nil {
		return err
	}
	switch fmode := fi.Mode(); {
	case fmode.IsDir():
		return nil
	default:
		return errors.New(filename + " is not directory" + ", it's a " + fmode.String())
	}
}

// GetHomeDir returns the home directory of the current user.
func GetHomeDir() string {
	currentUser, err := user.Current()
	var homedir string
	if err != nil {
		homedir = os.Getenv("HOME")
	} else {
		homedir = currentUser.HomeDir
	}
	return homedir
}

// ExpandHomePath expands a leading ~ to the user's home directory.
func ExpandHomePath(path string) string {
	if len(path) < 2 || path[:2] != "~"+string(os.PathSeparator) {
		return path
	}
	return filepath.Join(GetHomeDir(), path[2:])
}
