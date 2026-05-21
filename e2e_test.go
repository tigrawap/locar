package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tigrawap/locar/internal/testutil"
)

var locarBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "locar-e2e")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	locarBinary = filepath.Join(tmpDir, "locar")
	cmd := exec.Command("go", "build", "-o", locarBinary, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Stderr.Write(out)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}

// runLocar runs the built binary with the given flags and positional dirs.
// It asserts that every dir is within the sandbox before execution.
func runLocar(t *testing.T, s *testutil.Sandbox, flags []string, dirs ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	if len(dirs) == 0 {
		dirs = []string{s.Root}
	}
	for _, d := range dirs {
		s.AssertUnder(d)
	}
	args := append(flags, dirs...)
	cmd := exec.Command(locarBinary, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("runLocar: %v", err)
		}
	}
	stdout = outBuf.String()
	stderr = errBuf.String()
	return
}

// sortedLines returns sorted non-empty lines.
func sortedLines(s string) []string {
	lines := strings.Split(s, "\n")
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

// pathLines strips extra columns (inode, size, times) and quotes (raw mode) from output lines.
func pathLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, "\"") && strings.HasSuffix(l, "\"") && len(l) >= 2 {
			l = l[1 : len(l)-1]
		}
		if idx := strings.Index(l, " "); idx >= 0 {
			l = l[:idx]
		}
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

func TestE2E_DefaultRun(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("subdir")
	s.WriteFile("a.txt", "a")
	s.WriteFile("subdir/b.txt", "b")

	stdout, _, exitCode := runLocar(t, s, nil)
	if exitCode != 0 {
		t.Fatalf("exit code %d, want 0", exitCode)
	}

	paths := pathLines(sortedLines(stdout))
	want := []string{
		s.Root + "/a.txt",
		s.Root + "/subdir/",
		s.Root + "/subdir/b.txt",
	}
	sort.Strings(want)
	if len(paths) != len(want) {
		t.Errorf("got %d entries, want %d\ngot: %v\nwant: %v", len(paths), len(want), paths, want)
	}
	for i := range want {
		if i < len(paths) && paths[i] != want[i] {
			t.Errorf("mismatch at %d: got %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestE2E_FilterGlob(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "a")
	s.WriteFile("b.go", "b")
	s.WriteFile("c.txt", "c")

	stdout, _, _ := runLocar(t, s, []string{"-f", "*.go", "-t", "file"})
	paths := pathLines(sortedLines(stdout))
	if len(paths) != 1 || paths[0] != s.Root+"/b.go" {
		t.Errorf("expected only b.go, got %v", paths)
	}
}

func TestE2E_ExcludeGlob(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.log", "a")
	s.WriteFile("b.txt", "b")

	stdout, _, _ := runLocar(t, s, []string{"-x", "*.log", "-t", "file"})
	paths := pathLines(sortedLines(stdout))
	if len(paths) != 1 || paths[0] != s.Root+"/b.txt" {
		t.Errorf("expected only b.txt, got %v", paths)
	}
}

func TestE2E_TypeFile(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("subdir")
	s.WriteFile("a.txt", "a")

	stdout, _, _ := runLocar(t, s, []string{"-t", "file"})
	paths := pathLines(sortedLines(stdout))
	if len(paths) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(paths), paths)
	}
	// only files, no dirs
	for _, p := range paths {
		if strings.HasSuffix(p, "/") {
			t.Errorf("expected only files, got dir: %q", p)
		}
	}
}

func TestE2E_TypeDir(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("subdir")
	s.WriteFile("a.txt", "a")

	stdout, _, _ := runLocar(t, s, []string{"-t", "dir"})
	paths := pathLines(sortedLines(stdout))
	for _, p := range paths {
		if !strings.HasSuffix(p, "/") {
			t.Errorf("expected only dirs, got: %q", p)
		}
	}
}

func TestE2E_WithSize(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "hello world") // 11 bytes

	stdout, _, _ := runLocar(t, s, []string{"--with-size", "-t", "file"})
	lines := sortedLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], " 11") {
		t.Errorf("expected size 11 in output: %q", lines[0])
	}
}

func TestE2E_Inodes(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "data")

	stdout, _, _ := runLocar(t, s, []string{"--inodes", "-t", "file"})
	lines := sortedLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Format: path <ino_decimal>
	fields := strings.Fields(lines[0])
	if len(fields) != 2 {
		t.Errorf("expected 2 fields, got %d: %q", len(fields), lines[0])
	}
}

func TestE2E_InodesHex(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "data")

	stdout, _, _ := runLocar(t, s, []string{"--inodes-hex", "-t", "file"})
	lines := sortedLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Format: path 0x<hex_ino>
	if !strings.Contains(lines[0], " 0x") {
		t.Errorf("expected 0x prefix in output: %q", lines[0])
	}
}

func TestE2E_Raw(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("file with spaces.txt", "data")

	stdout, _, _ := runLocar(t, s, []string{"--raw", "-t", "file"})
	lines := sortedLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "\"") {
		t.Errorf("raw mode should wrap in quotes: %q", lines[0])
	}
}

func TestE2E_Quiet(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("a.txt", "a")

	stdout, _, _ := runLocar(t, s, []string{"-q"})
	if stdout != "" {
		t.Errorf("quiet mode should produce no stdout, got: %s", stdout)
	}
}

func TestE2E_Version(t *testing.T) {
	// version uses os.Exit(0), so capture via subprocess
	cmd := exec.Command(locarBinary, "-v")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		// os.Exit(0) is clean - no error
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() != 0 {
				t.Fatalf("exit code %d, want 0", ee.ExitCode())
			}
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	stdout := outBuf.String()
	if !strings.Contains(stdout, "version") {
		t.Errorf("version output should contain 'version': %q", stdout)
	}
}

func TestE2E_Help(t *testing.T) {
	cmd := exec.Command(locarBinary, "--help")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() != 0 {
				t.Fatalf("exit code %d, want 0", ee.ExitCode())
			}
		}
	}
	output := outBuf.String() + errBuf.String()
	if !strings.Contains(output, "--filter") && !strings.Contains(output, "-f") {
		t.Errorf("help should mention --filter or -f flag: %q", output)
	}
	if !strings.Contains(output, "--exclude") && !strings.Contains(output, "-x") {
		t.Errorf("help should mention --exclude or -x flag: %q", output)
	}
	if !strings.Contains(output, "--type") && !strings.Contains(output, "-t") {
		t.Errorf("help should mention --type or -t flag: %q", output)
	}
	if !strings.Contains(output, "--jobs") && !strings.Contains(output, "-j") {
		t.Errorf("help should mention --jobs or -j flag: %q", output)
	}
}

func TestE2E_MtimeOlder(t *testing.T) {
	s := testutil.New(t)
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-30 * time.Minute)

	s.Touch("old.txt", old)
	s.Touch("recent.txt", recent)

	stdout, _, _ := runLocar(t, s, []string{"--mtime-older", "1h", "-t", "file"})
	paths := pathLines(sortedLines(stdout))
	if len(paths) != 1 {
		t.Errorf("expected 1 file older than 1h, got %d: %v", len(paths), paths)
	}
	if len(paths) == 1 && paths[0] != s.Root+"/old.txt" {
		t.Errorf("expected old.txt, got %s", paths[0])
	}
}

func TestE2E_MultipleDirs(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("dir1")
	s.Mkdir("dir2")
	s.WriteFile("dir1/a.txt", "a")
	s.WriteFile("dir2/b.txt", "b")

	stdout, _, _ := runLocar(t, s, []string{"-t", "file"}, s.Abs("dir1"), s.Abs("dir2"))
	paths := pathLines(sortedLines(stdout))
	want := []string{s.Abs("dir1/a.txt"), s.Abs("dir2/b.txt")}
	sort.Strings(want)
	if len(paths) != len(want) {
		t.Errorf("got %d entries, want %d\ngot: %v\nwant: %v", len(paths), len(want), paths, want)
	}
	for i := range want {
		if i < len(paths) && paths[i] != want[i] {
			t.Errorf("mismatch at %d: got %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestE2E_NonexistentDir(t *testing.T) {
	s := testutil.New(t)
	nope := s.Abs("nope")
	_, stderr, exitCode := runLocar(t, s, nil, nope)
	if exitCode == 0 {
		t.Error("expected non-zero exit for non-existent dir")
	}
	// Error should be logged to stderr
	if stderr == "" {
		t.Error("expected error message on stderr")
	}
}

func TestE2E_Delete(t *testing.T) {
	s := testutil.New(t)
	s.WriteFile("keep.txt", "keep")
	s.WriteFile("delete.txt", "delete")

	_, _, exitCode := runLocar(t, s, []string{"-t", "file", "-x", "**/keep.txt", "--delete"})
	if exitCode != 0 {
		t.Fatalf("exit code %d, want 0", exitCode)
	}

	if !s.Exists("keep.txt") {
		t.Error("keep.txt should still exist")
	}
	if s.Exists("delete.txt") {
		t.Error("delete.txt should have been deleted")
	}
	if !s.Exists(".") {
		t.Error("sandbox root should still exist")
	}
}

func TestE2E_DeleteAll(t *testing.T) {
	s := testutil.New(t)
	s.Mkdir("subdir")
	s.WriteFile("subdir/a.txt", "a")
	s.WriteFile("subdir/b.txt", "b")
	s.WriteFile("other.txt", "other")

	// DeleteAll the subdir dir — but the Include glob must match full paths
	// We scan root and include "*/subdir" which will match ".../subdir"
	_, _, exitCode := runLocar(t, s, []string{"-t", "dir", "-f", "*/subdir", "--delete-all"})
	if exitCode != 0 {
		t.Fatalf("exit code %d, want 0", exitCode)
	}

	if s.Exists("subdir") {
		t.Error("subdir should have been removed by DeleteAll")
	}
}
