package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetFileTimes_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "testfile")
	if err := os.WriteFile(fpath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	atime, mtime, ctime, err := GetFileTimes(fpath)
	if err != nil {
		t.Fatalf("GetFileTimes: unexpected error: %v", err)
	}

	now := time.Now()
	tolerance := 5 * time.Minute

	if diff := now.Sub(mtime); diff < -tolerance || diff > tolerance {
		t.Errorf("mtime %v not within ±%v of now %v", mtime, tolerance, now)
	}

	_ = atime
	_ = ctime
}

func TestGetFileTimes_NotExists(t *testing.T) {
	_, _, _, err := GetFileTimes(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected non-nil error for non-existent file")
	}
}
