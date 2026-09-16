package engine

import (
	"io"
	"time"

	"github.com/tigrawap/locar/pkg/metrics"
)

// Config holds the configuration for building an Explorer.
type Config struct {
	Threads                        int
	ResultThreads                  int
	StopOnError                    bool
	Timeout                        time.Duration
	Quiet                          bool
	Inodes                         bool
	InodesHex                      bool
	Raw                            bool
	WithSizes                      bool
	WithTimes                      bool
	Output                         io.Writer
	Types                          []string
	Excludes                       []string
	Includes                       []string
	AtimeOlderThan, AtimeNewerThan time.Duration
	MtimeOlderThan, MtimeNewerThan time.Duration
	CtimeOlderThan, CtimeNewerThan time.Duration
	Delete                         bool
	DeleteAll                      bool
	// Metrics, when set, receives counters for filesystem operations
	// (opendir, readdir, stat, lstat, delete). Nil disables metrics.
	Metrics *metrics.Metrics
}
