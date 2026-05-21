package engine

import (
	"context"
	"syscall"
	"testing"
	"time"
)

func TestCheckTimeCondition_OlderThan(t *testing.T) {
	oneHourAgo := time.Now().Add(-70 * time.Minute)
	cond := TimeCondition{OlderThan: 1 * time.Hour}

	if !checkTimeCondition(oneHourAgo, cond) {
		t.Error("should allow timestamp older than 1h")
	}
	if checkTimeCondition(time.Now().Add(-30*time.Minute), cond) {
		t.Error("should reject timestamp newer than 1h")
	}
}

func TestCheckTimeCondition_NewerThan(t *testing.T) {
	cond := TimeCondition{NewerThan: 1 * time.Hour}

	if !checkTimeCondition(time.Now().Add(-30*time.Minute), cond) {
		t.Error("should allow timestamp newer than 1h")
	}
	if checkTimeCondition(time.Now().Add(-2*time.Hour), cond) {
		t.Error("should reject timestamp older than 1h")
	}
}

func TestCheckTimeCondition_Both(t *testing.T) {
	// OlderThan=24h, NewerThan=48h means: must be older than 24h AND newer than 48h.
	// i.e. timestamp must be between 24h and 48h in the past.
	cond := TimeCondition{OlderThan: 24 * time.Hour, NewerThan: 48 * time.Hour}

	// 36h ago: older than 24h AND newer than 48h -> passes both
	if !checkTimeCondition(time.Now().Add(-36*time.Hour), cond) {
		t.Error("should allow 36h (older than 24h AND newer than 48h)")
	}
	// 10h ago: newer than 24h -> fails OlderThan check (not old enough)
	if checkTimeCondition(time.Now().Add(-10*time.Hour), cond) {
		t.Error("should reject 10h (not old enough for OlderThan=24h)")
	}
	// 72h ago: older than 48h -> fails NewerThan check (too old)
	if checkTimeCondition(time.Now().Add(-72*time.Hour), cond) {
		t.Error("should reject 72h (too old for NewerThan=48h)")
	}
}

func TestCheckTimeCondition_Neither(t *testing.T) {
	cond := TimeCondition{}
	if !checkTimeCondition(time.Now().Add(-1000*time.Hour), cond) {
		t.Error("empty conditions should always pass")
	}
	if !checkTimeCondition(time.Now(), cond) {
		t.Error("empty conditions should always pass (now)")
	}
}

func TestCreateTimeConditions_OlderOnly(t *testing.T) {
	d := 2 * time.Hour
	cond := createTimeConditions(&d, nil)
	if cond.OlderThan != 2*time.Hour {
		t.Errorf("OlderThan = %v", cond.OlderThan)
	}
	if cond.NewerThan != 0 {
		t.Errorf("NewerThan = %v, want 0", cond.NewerThan)
	}
}

func TestCreateTimeConditions_NewerOnly(t *testing.T) {
	d := 3 * time.Hour
	cond := createTimeConditions(nil, &d)
	if cond.OlderThan != 0 {
		t.Errorf("OlderThan = %v, want 0", cond.OlderThan)
	}
	if cond.NewerThan != 3*time.Hour {
		t.Errorf("NewerThan = %v", cond.NewerThan)
	}
}

func TestCreateTimeConditions_Both(t *testing.T) {
	older := 2 * time.Hour
	newer := 1 * time.Hour
	cond := createTimeConditions(&older, &newer)
	if cond.OlderThan != 2*time.Hour {
		t.Errorf("OlderThan = %v", cond.OlderThan)
	}
	if cond.NewerThan != 1*time.Hour {
		t.Errorf("NewerThan = %v", cond.NewerThan)
	}
}

func TestCreateTimeConditions_None(t *testing.T) {
	cond := createTimeConditions(nil, nil)
	if cond.OlderThan != 0 {
		t.Errorf("OlderThan = %v, want 0", cond.OlderThan)
	}
	if cond.NewerThan != 0 {
		t.Errorf("NewerThan = %v, want 0", cond.NewerThan)
	}
}

func TestEntryType(t *testing.T) {
	tests := []struct {
		dt   uint8
		want string
	}{
		{syscall.DT_DIR, "dir"},
		{syscall.DT_REG, "file"},
		{syscall.DT_LNK, "link"},
		{syscall.DT_SOCK, "socket"},
		{syscall.DT_CHR, "char"},
		{99, "unknown(99)"},
	}
	for _, tt := range tests {
		got := entryType(tt.dt)
		if got != tt.want {
			t.Errorf("entryType(%d) = %q, want %q", tt.dt, got, tt.want)
		}
	}
}

func TestIsExcluded(t *testing.T) {
	cfg := Config{Excludes: []string{"*.log", "**/node_modules"}}
	e2, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path     string
		excluded bool
	}{
		{"test.log", true},
		{"/some/path/node_modules", true},
		{"/some/path/node_modules/foo", false},
		{"test.txt", false},
		{"main.go", false},
	}
	for _, tt := range tests {
		if got := e2.isExcluded(tt.path); got != tt.excluded {
			t.Errorf("isExcluded(%q) = %v, want %v", tt.path, got, tt.excluded)
		}
	}
}

func TestIsNotIncluded(t *testing.T) {
	cfg := Config{Includes: []string{"*.go", "*.rs"}}
	e, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path        string
		notIncluded bool
	}{
		{"main.go", false},
		{"lib.rs", false},
		{"readme.md", true},
		{"script.py", true},
	}
	for _, tt := range tests {
		if got := e.isNotIncluded(tt.path); got != tt.notIncluded {
			t.Errorf("isNotIncluded(%q) = %v, want %v", tt.path, got, tt.notIncluded)
		}
	}
}
