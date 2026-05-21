package cli

import (
	"os"
	"testing"
	"time"
)

func TestEngineConfig_Mapping(t *testing.T) {
	opts := &Options{
		StopOnError:    true,
		Inodes:         true,
		InodesHex:      true,
		Raw:            true,
		Threads:        42,
		WithSizes:      true,
		WithTimes:      true,
		AtimeOlderThan: 1 * time.Hour,
		AtimeNewerThan: 2 * time.Hour,
		MtimeOlderThan: 3 * time.Hour,
		MtimeNewerThan: 4 * time.Hour,
		CtimeOlderThan: 5 * time.Hour,
		CtimeNewerThan: 6 * time.Hour,
		ResultThreads:  7,
		Delete:         true,
		DeleteAll:      true,
		Quiet:          true,
		Exclude:        []string{"a", "b"},
		Filter:         []string{"c", "d"},
		Type:           []string{"file", "link"},
		Timeout:        10 * time.Minute,
	}

	cfg := opts.EngineConfig()

	// Check every field
	if cfg.StopOnError != opts.StopOnError {
		t.Errorf("StopOnError: want %v, got %v", opts.StopOnError, cfg.StopOnError)
	}
	if cfg.Inodes != opts.Inodes {
		t.Errorf("Inodes: want %v, got %v", opts.Inodes, cfg.Inodes)
	}
	if cfg.InodesHex != opts.InodesHex {
		t.Errorf("InodesHex: want %v, got %v", opts.InodesHex, cfg.InodesHex)
	}
	if cfg.Raw != opts.Raw {
		t.Errorf("Raw: want %v, got %v", opts.Raw, cfg.Raw)
	}
	if cfg.Threads != opts.Threads {
		t.Errorf("Threads: want %d, got %d", opts.Threads, cfg.Threads)
	}
	if cfg.WithSizes != opts.WithSizes {
		t.Errorf("WithSizes: want %v, got %v", opts.WithSizes, cfg.WithSizes)
	}
	if cfg.WithTimes != opts.WithTimes {
		t.Errorf("WithTimes: want %v, got %v", opts.WithTimes, cfg.WithTimes)
	}
	if cfg.AtimeOlderThan != opts.AtimeOlderThan {
		t.Errorf("AtimeOlderThan: want %v, got %v", opts.AtimeOlderThan, cfg.AtimeOlderThan)
	}
	if cfg.AtimeNewerThan != opts.AtimeNewerThan {
		t.Errorf("AtimeNewerThan: want %v, got %v", opts.AtimeNewerThan, cfg.AtimeNewerThan)
	}
	if cfg.MtimeOlderThan != opts.MtimeOlderThan {
		t.Errorf("MtimeOlderThan: want %v, got %v", opts.MtimeOlderThan, cfg.MtimeOlderThan)
	}
	if cfg.MtimeNewerThan != opts.MtimeNewerThan {
		t.Errorf("MtimeNewerThan: want %v, got %v", opts.MtimeNewerThan, cfg.MtimeNewerThan)
	}
	if cfg.CtimeOlderThan != opts.CtimeOlderThan {
		t.Errorf("CtimeOlderThan: want %v, got %v", opts.CtimeOlderThan, cfg.CtimeOlderThan)
	}
	if cfg.CtimeNewerThan != opts.CtimeNewerThan {
		t.Errorf("CtimeNewerThan: want %v, got %v", opts.CtimeNewerThan, cfg.CtimeNewerThan)
	}
	if cfg.ResultThreads != opts.ResultThreads {
		t.Errorf("ResultThreads: want %d, got %d", opts.ResultThreads, cfg.ResultThreads)
	}
	if cfg.Delete != opts.Delete {
		t.Errorf("Delete: want %v, got %v", opts.Delete, cfg.Delete)
	}
	if cfg.DeleteAll != opts.DeleteAll {
		t.Errorf("DeleteAll: want %v, got %v", opts.DeleteAll, cfg.DeleteAll)
	}
	if cfg.Quiet != opts.Quiet {
		t.Errorf("Quiet: want %v, got %v", opts.Quiet, cfg.Quiet)
	}
	if cfg.Timeout != opts.Timeout {
		t.Errorf("Timeout: want %v, got %v", opts.Timeout, cfg.Timeout)
	}

	// Exclude -> Excludes
	if len(cfg.Excludes) != 2 || cfg.Excludes[0] != "a" || cfg.Excludes[1] != "b" {
		t.Errorf("Excludes: want [a b], got %v", cfg.Excludes)
	}
	// Filter -> Includes
	if len(cfg.Includes) != 2 || cfg.Includes[0] != "c" || cfg.Includes[1] != "d" {
		t.Errorf("Includes: want [c d], got %v", cfg.Includes)
	}
	// Type -> Types
	if len(cfg.Types) != 2 || cfg.Types[0] != "file" || cfg.Types[1] != "link" {
		t.Errorf("Types: want [file link], got %v", cfg.Types)
	}
}

func TestParse_HappyPath(t *testing.T) {
	// Save and restore os.Args
	saveArgs := make([]string, len(os.Args))
	copy(saveArgs, os.Args)
	defer func() { os.Args = saveArgs }()

	dir := t.TempDir()
	os.Args = []string{"locar", "-j", "7", "--quiet", "--mtime-older", "1h", dir}

	opts := Parse("vtest")
	if opts.Version {
		t.Error("--version should be false when not passed")
	}
	if opts.Threads != 7 {
		t.Errorf("Threads = %d, want 7 (short -j flag)", opts.Threads)
	}
	if !opts.Quiet {
		t.Error("Quiet = false after --quiet")
	}
	if opts.MtimeOlderThan != time.Hour {
		t.Errorf("MtimeOlderThan = %v, want 1h", opts.MtimeOlderThan)
	}
	if len(opts.Args.Directories) != 1 || opts.Args.Directories[0] != dir {
		t.Errorf("Directories = %v, want [%s]", opts.Args.Directories, dir)
	}
}
