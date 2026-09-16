package metrics

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNilMetrics_SafeNoop(t *testing.T) {
	var m *Metrics

	cases := []struct {
		name string
		fn   func()
	}{
		{"Op", func() { m.Op(OpOpenDir) }},
		{"OpError", func() { m.OpError(OpOpenDir) }},
		{"OpTimeout", func() { m.OpTimeout(OpOpenDir) }},
		{"Record", func() { m.Record(OpOpenDir, errors.New("boom")) }},
		{"Register", func() {
			if err := m.Register(prometheus.NewRegistry()); err != nil {
				t.Errorf("nil Metrics.Register returned error: %v", err)
			}
		}},
		{"MustRegister", func() { m.MustRegister(prometheus.NewRegistry()) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("nil Metrics.%s panicked: %v", c.name, r)
				}
			}()
			c.fn()
		})
	}
}

func TestRegisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := RegisterMetrics(reg)
	if err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
	if m == nil {
		t.Fatal("RegisterMetrics returned nil Metrics with nil error")
	}

	// Registering a second Metrics into the same registry must fail,
	// since the collectors would collide.
	if _, err := RegisterMetrics(reg); err == nil {
		t.Error("expected error registering into an already-populated registry, got nil")
	}
}

func TestCountersIncrement(t *testing.T) {
	m := New()

	m.Op(OpOpenDir)
	m.Op(OpOpenDir)
	if got := testutil.ToFloat64(m.ops.WithLabelValues(OpOpenDir)); got != 2 {
		t.Errorf("ops[opendir] = %v, want 2", got)
	}

	m.OpError(OpReadDir)
	if got := testutil.ToFloat64(m.errors.WithLabelValues(OpReadDir)); got != 1 {
		t.Errorf("errors[readdir] = %v, want 1", got)
	}

	m.OpTimeout(OpReadDir)
	if got := testutil.ToFloat64(m.timeouts.WithLabelValues(OpReadDir)); got != 1 {
		t.Errorf("timeouts[readdir] = %v, want 1", got)
	}
	// A timeout must also count as an error.
	if got := testutil.ToFloat64(m.errors.WithLabelValues(OpReadDir)); got != 2 {
		t.Errorf("errors[readdir] after timeout = %v, want 2", got)
	}

	m.Record(OpStat, nil)
	if got := testutil.ToFloat64(m.ops.WithLabelValues(OpStat)); got != 1 {
		t.Errorf("ops[stat] = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.errors.WithLabelValues(OpStat)); got != 0 {
		t.Errorf("errors[stat] = %v, want 0", got)
	}

	m.Record(OpLstat, errors.New("fail"))
	if got := testutil.ToFloat64(m.ops.WithLabelValues(OpLstat)); got != 1 {
		t.Errorf("ops[lstat] = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.errors.WithLabelValues(OpLstat)); got != 1 {
		t.Errorf("errors[lstat] = %v, want 1", got)
	}

	m.Op(OpDelete)
	m.Op(OpDeleteRecursive)
	if got := testutil.ToFloat64(m.ops.WithLabelValues(OpDelete)); got != 1 {
		t.Errorf("ops[delete] = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.ops.WithLabelValues(OpDeleteRecursive)); got != 1 {
		t.Errorf("ops[delete_recursive] = %v, want 1", got)
	}
}

func TestNew_AllLabelsPresentAtZero(t *testing.T) {
	m := New()

	for _, op := range allOps {
		if got := testutil.ToFloat64(m.ops.WithLabelValues(op)); got != 0 {
			t.Errorf("ops[%s] = %v, want 0", op, got)
		}
		if got := testutil.ToFloat64(m.errors.WithLabelValues(op)); got != 0 {
			t.Errorf("errors[%s] = %v, want 0", op, got)
		}
		if got := testutil.ToFloat64(m.timeouts.WithLabelValues(op)); got != 0 {
			t.Errorf("timeouts[%s] = %v, want 0", op, got)
		}
	}

	// Every label value across all three vecs should be a collectible
	// series (count == len(allOps) per vec).
	if n := testutil.CollectAndCount(m.ops); n != len(allOps) {
		t.Errorf("ops series count = %d, want %d", n, len(allOps))
	}
	if n := testutil.CollectAndCount(m.errors); n != len(allOps) {
		t.Errorf("errors series count = %d, want %d", n, len(allOps))
	}
	if n := testutil.CollectAndCount(m.timeouts); n != len(allOps) {
		t.Errorf("timeouts series count = %d, want %d", n, len(allOps))
	}
}
