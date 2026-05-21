package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsDir_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	if err := IsDir(dir); err != nil {
		t.Fatalf("expected nil error for existing dir, got: %v", err)
	}
}

func TestIsDir_RegularFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(fpath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := IsDir(fpath)
	if err == nil {
		t.Fatal("expected non-nil error for regular file")
	}
}

func TestIsDir_NotExists(t *testing.T) {
	err := IsDir(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected non-nil error for non-existent path")
	}
}

func TestExpandHomePath_WithTilde(t *testing.T) {
	homedir := GetHomeDir()
	if homedir == "" {
		t.Skip("no home dir available")
	}
	result := ExpandHomePath("~/foo")
	expected := filepath.Join(homedir, "foo")
	if result != expected {
		t.Errorf("ExpandHomePath(~%s) = %q, want %q", "/foo", result, expected)
	}
}

func TestExpandHomePath_AbsolutePath(t *testing.T) {
	abs := "/tmp/something"
	result := ExpandHomePath(abs)
	if result != abs {
		t.Errorf("absolute path should be unchanged: got %q", result)
	}
}

func TestExpandHomePath_RelativeNoTilde(t *testing.T) {
	rel := "foo/bar"
	result := ExpandHomePath(rel)
	if result != rel {
		t.Errorf("relative non-tilde path should be unchanged: got %q", result)
	}
}

func TestExpandHomePath_JustTilde(t *testing.T) {
	result := ExpandHomePath("~")
	if result != "~" {
		t.Errorf("bare ~ should not be expanded: got %q", result)
	}
}

func TestGetHomeDir_NotEmpty(t *testing.T) {
	homedir := GetHomeDir()
	if homedir == "" {
		t.Error("GetHomeDir returned empty string")
	}
}
