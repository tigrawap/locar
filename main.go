package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/tigrawap/locar/internal/cli"
	"github.com/tigrawap/locar/pkg/engine"
	"github.com/tigrawap/locar/pkg/fsutil"
)

var Version = "v0.1.0"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := cli.Parse(Version)

	eng, err := engine.New(ctx, opts.EngineConfig())
	if err != nil {
		log.Fatalln(err)
	}

	for _, directory := range opts.Args.Directories {
		seed := fsutil.ExpandHomePath(directory)
		if err := fsutil.IsDir(seed); err != nil {
			log.Fatalln(seed, err)
		}
		eng.AddDir(seed)
	}

	go func() {
		<-cli.QuitOnInterrupt()
		cancel()
		<-time.After(100 * time.Millisecond)
		os.Exit(130)
	}()

	eng.Start()
	eng.Wait()

	if ctx.Err() == context.Canceled {
		os.Exit(130)
	}
}
