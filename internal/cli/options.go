package cli

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/jessevdk/go-flags"

	"github.com/tigrawap/locar/pkg/engine"
)

// Options holds all CLI flags and arguments.
type Options struct {
	Resilient      bool          `long:"resilient" description:"DEPRECATED and ignored, resilient is a default, use --stop-on-error if it is undesired behaviour"`
	StopOnError    bool          `long:"stop-on-error" description:"Aborts scan on any error"`
	Inodes         bool          `long:"inodes" description:"Output inodes (decimal) along with filenames"`
	InodesHex      bool          `long:"inodes-hex" description:"Output inodes (hexadecimal) along with filenames"`
	Raw            bool          `long:"raw" description:"Output filenames as escaped strings"`
	Threads        int           `short:"j" long:"jobs" description:"Number of jobs(threads)" default:"128"`
	WithSizes      bool          `long:"with-size" description:"Output file sizes along with filenames"`
	WithTimes      bool          `long:"with-times" description:"Output file with atime, mtime, ctime along with filenames"`
	AtimeOlderThan time.Duration `long:"atime-older" description:"Filter files by access time older than this duration (e.g., 24h5m25s)" default:"0s"`
	AtimeNewerThan time.Duration `long:"atime-newer" description:"Filter files by access time newer than this duration (e.g., 24h5m25s)" default:"0s"`
	MtimeOlderThan time.Duration `long:"mtime-older" description:"Filter files by modification time older than this duration (e.g., 24h5m25s)" default:"0s"`
	MtimeNewerThan time.Duration `long:"mtime-newer" description:"Filter files by modification time newer than this duration (e.g., 24h5m25s)" default:"0s"`
	CtimeOlderThan time.Duration `long:"ctime-older" description:"Filter files by change time older than this duration (e.g., 24h5m25s)" default:"0s"`
	CtimeNewerThan time.Duration `long:"ctime-newer" description:"Filter files by change time newer than this duration (e.g., 24h5m25s)" default:"0s"`
	ResultThreads  int           `long:"result-jobs" description:"Number of jobs for processing results: file stats, and concurrent directory deletes (at most one delete in flight per directory)" default:"128"`
	Delete         bool          `long:"delete" description:"Delete found files. Non empty directories will be ignored"`
	DeleteAll      bool          `long:"delete-all" description:"Delete found files. Non empty directories will be removed with ALL their contents!!!"`
	Quiet          bool          `short:"q" long:"quiet" description:"Suppress per-file output, only show stats"`
	Version        bool          `short:"v" long:"version" description:"Show version"`

	Exclude []string `short:"x" long:"exclude" description:"Patterns to exclude. Can be specified multiple times"`
	Filter  []string `short:"f" long:"filter" description:"Patterns to filter by. Can be specified multiple times"`

	Type []string `short:"t" long:"type" default:"file" default:"dir" default:"link" default:"socket" description:"Search entries of specific type \nPossible values: file, dir, link, socket, all. Can be specified multiple times"`

	Args struct {
		Directories []string `positional-arg-name:"directories" description:"Directories to search, using current directory if missing"`
	} `positional-args:"yes"`

	Timeout time.Duration `long:"timeout" default:"5m" description:"Timeout for readdir operations. Error will be reported, but os thread will be kept hanging"`
}

// Parse parses CLI flags and returns an Options. version is used for --version output.
func Parse(version string) *Options {
	opts := &Options{}
	_, err := flags.Parse(opts)
	if opts.Version {
		fmt.Printf("%s version %s\n", path.Base(os.Args[0]), version)
		os.Exit(0)
	}
	if flagsErr, ok := err.(*flags.Error); ok {
		if flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		log.Fatalln(flagsErr)
		os.Exit(1)
	}

	if err != nil {
		log.Fatalln(err.Error())
	}

	if len(opts.Args.Directories) == 0 {
		opts.Args.Directories = []string{"."}
	}
	return opts
}

// EngineConfig maps CLI options to an engine.Config.
func (o *Options) EngineConfig() engine.Config {
	return engine.Config{
		StopOnError:    o.StopOnError,
		Inodes:         o.Inodes,
		InodesHex:      o.InodesHex,
		Raw:            o.Raw,
		Threads:        o.Threads,
		WithSizes:      o.WithSizes,
		WithTimes:      o.WithTimes,
		AtimeOlderThan: o.AtimeOlderThan,
		AtimeNewerThan: o.AtimeNewerThan,
		MtimeOlderThan: o.MtimeOlderThan,
		MtimeNewerThan: o.MtimeNewerThan,
		CtimeOlderThan: o.CtimeOlderThan,
		CtimeNewerThan: o.CtimeNewerThan,
		ResultThreads:  o.ResultThreads,
		Delete:         o.Delete,
		DeleteAll:      o.DeleteAll,
		Quiet:          o.Quiet,
		Excludes:       o.Exclude,
		Includes:       o.Filter,
		Types:          o.Type,
		Timeout:        o.Timeout,
	}
}
