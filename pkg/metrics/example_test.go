package metrics_test

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/tigrawap/locar/pkg/metrics"
)

// ExampleRegisterMetrics shows the one-shot path: build a *metrics.Metrics
// and register it with a registry in a single call. A fresh
// prometheus.NewRegistry() is used here (and recommended for library
// consumers in general) so the example is isolated from any other metrics
// in the process.
//
// New pre-initializes all 6 known operations across all 3 counters, so the
// registry exposes 18 series from the moment registration succeeds,
// regardless of whether any operation has actually run yet.
func ExampleRegisterMetrics() {
	reg := prometheus.NewRegistry()

	m, err := metrics.RegisterMetrics(reg)
	if err != nil {
		fmt.Println("register error:", err)
		return
	}

	m.Op(metrics.OpOpenDir)
	m.Op(metrics.OpOpenDir)

	n, err := testutil.GatherAndCount(reg)
	if err != nil {
		fmt.Println("gather error:", err)
		return
	}
	fmt.Println(n)
	// Output:
	// 18
}

// ExampleMetrics_Register shows building a *metrics.Metrics with New and
// registering it separately, handling the registration error explicitly.
// This is useful when the *Metrics value is needed before the registry it
// will be registered with is available, or when the caller wants to react
// to a registration failure itself instead of using MustRegister.
func ExampleMetrics_Register() {
	m := metrics.New()

	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		fmt.Println("register error:", err)
		return
	}

	// Registering the same *Metrics again on the same registry fails: the
	// collectors are already there.
	err := m.Register(reg)
	fmt.Println(err != nil)
	// Output:
	// true
}
