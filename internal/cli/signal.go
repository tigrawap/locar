package cli

import (
	"os"
	"os/signal"
	"syscall"
)

// QuitOnInterrupt returns a channel that receives true when an interrupt signal is caught.
func QuitOnInterrupt() chan bool {
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
