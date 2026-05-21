package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Sandbox provides a per-test temporary directory with safety guarantees
// that prevent any test from touching paths outside the sandbox root.
type Sandbox struct {
	TB   testing.TB
	Root string
}

// New creates a new Sandbox whose Root is a unique temp directory.
// It panics if the resulting Root would be unsafe (e.g. escaping the system temp).
func New(t testing.TB) *Sandbox {
	t.Helper()
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatalf("sandbox: could not resolve temp dir: %v", err)
	}
	root = filepath.Clean(root)
	assertSafeRoot(t, root)
	return &Sandbox{TB: t, Root: root}
}

// assertSafeRoot enforces that root is a real temp directory and not a
// system-critical path.
func assertSafeRoot(t testing.TB, root string) {
	t.Helper()
	if root == "" {
		t.Fatal("sandbox: Root must not be empty")
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("sandbox: Root must be absolute, got %q", root)
	}
	tmpDir, _ := filepath.Abs(os.TempDir())
	tmpDir = filepath.Clean(tmpDir)
	if root == tmpDir {
		t.Fatalf("sandbox: Root must not equal the system temp dir (%q)", tmpDir)
	}
	if root == "/" {
		t.Fatal("sandbox: Root must not be /")
	}
	if root == filepath.Dir(root) {
		t.Fatalf("sandbox: Root must not be a filesystem root (%q)", root)
	}

	// Must be under the OS temp directory.
	realTmp, err := filepath.EvalSymlinks(tmpDir)
	if err == nil {
		realRoot, err := filepath.EvalSymlinks(root)
		if err == nil {
			if !strings.HasPrefix(realRoot, realTmp+string(filepath.Separator)) && realRoot != realTmp {
				t.Fatalf("sandbox: Root %q is not under system temp dir %q", root, tmpDir)
			}
		}
	}

	// Must not be (or contain) the repo dir.
	repoDir, err := os.Getwd()
	if err == nil {
		repoDir, _ = filepath.Abs(repoDir)
		repoDir = filepath.Clean(repoDir)
		absRoot := root
		if strings.HasPrefix(absRoot, repoDir+string(filepath.Separator)) || absRoot == repoDir {
			t.Fatalf("sandbox: Root %q must not be inside the repo directory %q", root, repoDir)
		}
		if strings.HasPrefix(repoDir, absRoot+string(filepath.Separator)) || repoDir == absRoot {
			t.Fatalf("sandbox: Root %q must not contain the repo directory %q", root, repoDir)
		}
	}

	// Must not be (or contain) the user home dir.
	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		homeDir = filepath.Clean(homeDir)
		if strings.HasPrefix(root, homeDir+string(filepath.Separator)) || root == homeDir {
			// That's fine — t.TempDir() is often under $TMPDIR which may be under home.
			// Only reject if root *is* the home dir.
		}
		if root == homeDir {
			t.Fatalf("sandbox: Root must not be the home directory")
		}
	}
}

// Abs resolves rel against the sandbox Root and verifies the result stays
// within Root (no ".." escape).
func (s *Sandbox) Abs(rel string) string {
	s.TB.Helper()
	joined := filepath.Join(s.Root, rel)
	abs, err := filepath.Abs(joined)
	if err != nil {
		s.TB.Fatalf("sandbox.Abs(%q): %v", rel, err)
		return ""
	}
	abs = filepath.Clean(abs)

	// Ensure the raw (non-symlink-resolved) path is within Root.
	if !strings.HasPrefix(abs, s.Root+string(filepath.Separator)) && abs != s.Root {
		s.TB.Fatalf("sandbox.Abs(%q) = %q which escapes the sandbox root %q", rel, abs, s.Root)
	}
	return abs
}

// Mkdir creates all directories under rel within the sandbox.
func (s *Sandbox) Mkdir(rel string) {
	s.TB.Helper()
	dir := s.Abs(rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.TB.Fatalf("sandbox.Mkdir(%q): %v", rel, err)
	}
}

// WriteFile creates parent directories as needed and writes content to a file
// under the sandbox.
func (s *Sandbox) WriteFile(rel, content string) {
	s.TB.Helper()
	p := s.Abs(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.TB.Fatalf("sandbox.WriteFile(%q): mkdir parent: %v", rel, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		s.TB.Fatalf("sandbox.WriteFile(%q): %v", rel, err)
	}
}

// Touch creates an empty file under the sandbox and sets its mtime.
func (s *Sandbox) Touch(rel string, mtime time.Time) {
	s.TB.Helper()
	s.WriteFile(rel, "")
	p := s.Abs(rel)
	if err := os.Chtimes(p, time.Now(), mtime); err != nil {
		s.TB.Fatalf("sandbox.Touch(%q): %v", rel, err)
	}
}

// Symlink creates a symbolic link under the sandbox pointing to target.
func (s *Sandbox) Symlink(target, rel string) {
	s.TB.Helper()
	linkPath := s.Abs(rel)
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		s.TB.Fatalf("sandbox.Symlink(%q): mkdir parent: %v", rel, err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		s.TB.Fatalf("sandbox.Symlink(%q -> %q): %v", rel, target, err)
	}
}

// AssertUnder verifies that path (resolved) lies within the sandbox Root.
func (s *Sandbox) AssertUnder(path string) {
	s.TB.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		s.TB.Fatalf("sandbox.AssertUnder(%q): %v", path, err)
	}
	abs = filepath.Clean(abs)

	if !strings.HasPrefix(abs, s.Root+string(filepath.Separator)) && abs != s.Root {
		s.TB.Fatalf("sandbox.AssertUnder(%q): path %q is not under sandbox root %q", path, abs, s.Root)
	}
}

// Exists reports whether rel exists (any file type) within the sandbox.
func (s *Sandbox) Exists(rel string) bool {
	s.TB.Helper()
	p := s.Abs(rel)
	_, err := os.Stat(p)
	return err == nil
}

// SyncBuffer is a goroutine-safe bytes.Buffer for capturing engine output.
type SyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends to the buffer; safe for concurrent use.
func (b *SyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the full buffer contents; safe for concurrent use.
func (b *SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
