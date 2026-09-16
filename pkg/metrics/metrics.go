// Package metrics provides Prometheus counters for locar's filesystem
// operations. It is opt-in: a caller that does not care about metrics can
// simply leave the *Metrics value unset (nil), since every method on
// *Metrics is a safe no-op when the receiver is nil.
//
// See docs/metrics.md in the repository root for the full guide:
// registration options, exposing the counters over HTTP, sharing a
// *Metrics across scans, the metric/operation reference, and example
// PromQL.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Operation names used as the `operation` label value on all three metrics.
const (
	OpOpenDir         = "opendir"
	OpReadDir         = "readdir"
	OpStat            = "stat"
	OpLstat           = "lstat"
	OpDelete          = "delete"
	OpDeleteRecursive = "delete_recursive"
)

// allOps lists every known operation, used to pre-initialize label values
// in New() so each series exists (at zero) before the first operation runs.
var allOps = []string{OpOpenDir, OpReadDir, OpStat, OpLstat, OpDelete, OpDeleteRecursive}

// Metrics holds locar's file-operation counters.
//
// ops counts every attempted operation. errors and timeouts are subsets of
// ops (an operation that errors or times out is still counted in ops), not
// disjoint counters alongside it.
//
// A nil *Metrics is a valid no-op: every method on it is safe to call and
// does nothing. This lets callers that do not care about metrics leave the
// field unset.
type Metrics struct {
	ops      *prometheus.CounterVec
	errors   *prometheus.CounterVec
	timeouts *prometheus.CounterVec
}

// New builds a Metrics with all counter vecs created but not yet registered
// with any registry. All known operation label values are pre-initialized
// to zero so their series show up in a scrape before the first operation.
func New() *Metrics {
	m := &Metrics{
		ops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "locar_file_operations_total",
			Help: "Total number of filesystem operations attempted, by operation.",
		}, []string{"operation"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "locar_file_operation_errors_total",
			Help: "Total number of filesystem operations that failed, by operation.",
		}, []string{"operation"}),
		timeouts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "locar_file_operation_timeouts_total",
			Help: "Total number of filesystem operations that exceeded their deadline, by operation.",
		}, []string{"operation"}),
	}
	for _, op := range allOps {
		m.ops.WithLabelValues(op)
		m.errors.WithLabelValues(op)
		m.timeouts.WithLabelValues(op)
	}
	return m
}

// Register registers all of m's counter vecs with r. It is a no-op (and
// returns nil) when m is nil.
func (m *Metrics) Register(r prometheus.Registerer) error {
	if m == nil {
		return nil
	}
	for _, c := range []prometheus.Collector{m.ops, m.errors, m.timeouts} {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// MustRegister registers all of m's counter vecs with r, panicking on
// error. It is a no-op when m is nil.
func (m *Metrics) MustRegister(r prometheus.Registerer) {
	if m == nil {
		return
	}
	if err := m.Register(r); err != nil {
		panic(err)
	}
}

// RegisterMetrics is a convenience one-liner that builds a new Metrics and
// registers it with r.
func RegisterMetrics(r prometheus.Registerer) (*Metrics, error) {
	m := New()
	if err := m.Register(r); err != nil {
		return nil, err
	}
	return m, nil
}

// Op increments the attempted-operations counter for operation. No-op when
// m is nil.
func (m *Metrics) Op(operation string) {
	if m == nil {
		return
	}
	m.ops.WithLabelValues(operation).Inc()
}

// OpError increments the failed-operations counter for operation. No-op
// when m is nil.
func (m *Metrics) OpError(operation string) {
	if m == nil {
		return
	}
	m.errors.WithLabelValues(operation).Inc()
}

// OpTimeout increments the timed-out-operations counter for operation, and
// also counts it as an error (a timeout is a failure). No-op when m is nil.
func (m *Metrics) OpTimeout(operation string) {
	if m == nil {
		return
	}
	m.timeouts.WithLabelValues(operation).Inc()
	m.OpError(operation)
}

// Record is a convenience for the common "attempted an op, here's the
// result" case: it always increments ops, and additionally increments
// errors when err != nil. It does not classify timeouts; call OpTimeout
// explicitly at call sites that already know they hit a deadline.
func (m *Metrics) Record(operation string, err error) {
	if m == nil {
		return
	}
	m.Op(operation)
	if err != nil {
		m.OpError(operation)
	}
}
