package main

import (
	"errors"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
)

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

func ExpandHomePath(path string) string {
	if len(path) < 2 || path[:2] != "~"+string(os.PathSeparator) {
		return path
	}
	return filepath.Join(GetHomeDir(), path[2:])
}

func quitOnInterrupt() chan bool {
	c := make(chan os.Signal, 2)
	quit := make(chan bool)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	signal.Notify(c, syscall.SIGQUIT)
	signal.Notify(c, syscall.SIGABRT)
	signal.Notify(c, syscall.SIGINT)
	go func() {
		<-c
		quit <- true
	}()
	return quit
}
