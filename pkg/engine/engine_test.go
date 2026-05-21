package engine

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tigrawap/locar/internal/testutil"
)

// outputLines returns a sorted slice of non-empty lines from the captured output.
func outputLines(sb *testutil.SyncBuffer) []string {
	raw := sb.String()
	lines := strings.Split(raw, "\n")
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// pathsFromLines extracts just the path portion from each output line
// (before any inode/size/times columns). Lines like:
//
//	/tmp/.../path 12345
//	"/tmp/.../path"
//	/tmp/.../dir/
//
// We strip quotes from raw mode and everything after the first space.
func pathsFromLines(lines []string) []string {
	var paths []string
	for _, l := range lines {
		// Handle raw mode: "#v" wraps in double quotes
		if strings.HasPrefix(l, "\"") && strings.HasSuffix(l, "\"") && len(l) >= 2 {
			l = l[1 : len(l)-1]
		}
		// Drop anything after a space (inode, size, times)
		if idx := strings.Index(l, " "); idx >= 0 {
			l = l[:idx]
		}
		paths = append(paths, l)
	}
	sort.Strings(paths)
	return paths
}

func TestEngine_TraverseAllTypes(t *testing.T) {
	s := testutil.New(t)

	// Build a tree
	s.Mkdir("subdir")
	s.WriteFile("file1.txt", "hello")
	s.WriteFile("subdir/file2.txt", "world")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file", "dir", "link", "socket"},
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	got := pathsFromLines(lines)

	// Expect: subdir/, file1.txt, subdir/file2.txt
	// (root dir itself is not emitted — only entries within the scanned dir)
	want := []string{
		s.Root + "/file1.txt",
		s.Root + "/subdir/",
		s.Root + "/subdir/file2.txt",
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Errorf("got %d entries, want %d\ngot: %v\nwant: %v", len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mismatch at %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEngine_TypeFilter_FilesOnly(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("subdir")
	s.WriteFile("a.txt", "a")
	s.WriteFile("subdir/b.txt", "b")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	paths := pathsFromLines(outputLines(&sb))
	for _, p := range paths {
		if strings.HasSuffix(p, "/") {
			t.Errorf("expected only files, got dir: %q", p)
		}
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(paths), paths)
	}
}

func TestEngine_TypeFilter_DirsOnly(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("subdir")
	s.WriteFile("a.txt", "a")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"dir"},
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	paths := pathsFromLines(outputLines(&sb))
	for _, p := range paths {
		if !strings.HasSuffix(p, "/") {
			t.Errorf("expected only dirs, got file: %q", p)
		}
	}
}

func TestEngine_IncludeGlob(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "data")
	s.WriteFile("b.go", "code")
	s.WriteFile("c.txt", "data2")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		Includes:      []string{"*.go"},
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	paths := pathsFromLines(outputLines(&sb))
	if len(paths) != 1 {
		t.Errorf("expected 1 file matching *.go, got %d: %v", len(paths), paths)
	}
	if paths[0] != s.Root+"/b.go" {
		t.Errorf("expected b.go, got %s", paths[0])
	}
}

func TestEngine_ExcludeGlob(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.log", "logdata")
	s.WriteFile("b.txt", "data")
	s.WriteFile("c.log", "another")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		Excludes:      []string{"*.log"},
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	paths := pathsFromLines(outputLines(&sb))
	if len(paths) != 1 {
		t.Errorf("expected 1 file after excluding *.log, got %d: %v", len(paths), paths)
	}
}

func TestEngine_MtimeOlderThan(t *testing.T) {
	s := testutil.New(t)
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-30 * time.Minute)

	s.Touch("old.txt", old)
	s.Touch("recent.txt", recent)

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:        4,
		ResultThreads:  4,
		Output:         &sb,
		Types:          []string{"file"},
		MtimeOlderThan: 1 * time.Hour,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	paths := pathsFromLines(outputLines(&sb))
	if len(paths) != 1 {
		t.Errorf("expected 1 file older than 1h, got %d: %v", len(paths), paths)
	}
	if len(paths) == 1 && paths[0] != s.Root+"/old.txt" {
		t.Errorf("expected old.txt, got %s", paths[0])
	}
}

func TestEngine_Delete(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("keep.txt", "keep")
	s.WriteFile("delete.txt", "delete")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		Excludes:      []string{"**/keep.txt"},
		Delete:        true,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	if s.Exists("keep.txt") == false {
		t.Error("keep.txt should still exist")
	}
	if s.Exists("delete.txt") {
		t.Error("delete.txt should have been deleted")
	}
	// Root must still exist
	if !s.Exists(".") {
		t.Error("sandbox root should still exist")
	}
}

func TestEngine_DeleteAll(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("subdir")
	s.WriteFile("subdir/a.txt", "a")
	s.WriteFile("subdir/b.txt", "b")
	s.WriteFile("other.txt", "other")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"dir"},
		Includes:      []string{"*/subdir"},
		DeleteAll:     true,
	}
	ctx := context.Background()
	// We can't use context.Background() here because it might get cancelled before delete workers finish.
	// Use a context with a longer timeout.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	e, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	if s.Exists("subdir") {
		t.Error("subdir should have been removed by DeleteAll")
	}
	// other.txt not in "subdir" so should survive (even though DeleteAll matched the dir, it only deletes what the engine finds)
	_ = sb.String()
}

func TestEngine_Symlink(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("target.txt", "data")
	s.Symlink(s.Abs("target.txt"), "the_link")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file", "link"},
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	paths := pathsFromLines(outputLines(&sb))

	foundLink := false
	foundTarget := false
	for _, p := range paths {
		if strings.HasSuffix(p, "/the_link") {
			foundLink = true
		}
		if strings.HasSuffix(p, "/target.txt") {
			foundTarget = true
		}
	}
	if !foundLink {
		t.Error("symlink not found in results")
	}
	if !foundTarget {
		t.Error("target file not found in results")
	}
}

func TestEngine_Quiet(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "hello")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		Quiet:         true,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	if len(lines) != 0 {
		t.Errorf("quiet mode should produce no output, got %d lines", len(lines))
	}
}

func TestEngine_WithSizes(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "hello world") // 11 bytes

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		WithSizes:     true,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], " 11") {
		t.Errorf("expected size 11 in output: %q", lines[0])
	}
}

func TestEngine_WithTimes(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "data")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		WithTimes:     true,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Output format: path atime_unix mtime_unix ctime_unix
	// With --with-times, dirs don't get trailing slash (time condition triggers checkFileTimeConditions)
	fields := strings.Fields(lines[0])
	if len(fields) != 4 {
		t.Errorf("expected 4 fields (path + 3 timestamps), got %d: %q", len(fields), lines[0])
	}
}

func TestEngine_Inodes(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "data")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		Inodes:        true,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Format: path <ino_decimal>
	fields := strings.Fields(lines[0])
	if len(fields) != 2 {
		t.Errorf("expected 2 fields (path + inode), got %d: %q", len(fields), lines[0])
	}
}

func TestEngine_InodesHex(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "data")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		InodesHex:     true,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Format: path 0x<hex_ino>
	if !strings.Contains(lines[0], " 0x") {
		t.Errorf("expected 0x inode prefix in output: %q", lines[0])
	}
}

func TestEngine_Raw(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a file with spaces.txt", "data")

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       4,
		ResultThreads: 4,
		Output:        &sb,
		Types:         []string{"file"},
		Raw:           true,
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Raw mode wraps in double quotes via %#v
	if !strings.HasPrefix(lines[0], "\"") {
		t.Errorf("raw mode should wrap in quotes: %q", lines[0])
	}
}

func TestEngine_Concurrent(t *testing.T) {
	// Build a larger tree to exercise concurrency
	s := testutil.New(t)
	for i := 0; i < 10; i++ {
		sub := "d" + strings.Repeat("x", i)
		s.Mkdir(sub)
		for j := 0; j < 5; j++ {
			s.WriteFile(sub+"/f"+string(rune('a'+j))+".txt", "data")
		}
	}

	var sb testutil.SyncBuffer
	cfg := Config{
		Threads:       16,
		ResultThreads: 16,
		Output:        &sb,
		Types:         []string{"file", "dir"},
	}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.AddDir(s.Root)
	e.Start()
	e.Wait()

	lines := outputLines(&sb)
	if len(lines) < 60 {
		t.Errorf("expected at least 60 entries (10 dirs + 50 files), got %d", len(lines))
	}
}
