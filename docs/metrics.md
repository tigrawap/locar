# Prometheus metrics for `pkg/metrics`

This is the full guide for wiring Prometheus counters into a program that
embeds locar as a library via `pkg/engine`. It expands on the quick-start
snippet in the [readme](../readme.md).

## Overview

Metrics are entirely opt-in: `engine.Config.Metrics` is a `*metrics.Metrics`
that defaults to `nil`, and every method on it is a safe no-op when the
receiver is `nil`. Set it and you get three Prometheus counters — attempted
file operations, failed operations, and timed-out operations — broken down
by an `operation` label (`opendir`, `readdir`, `stat`, `lstat`, `delete`,
`delete_recursive`). Leave it unset and locar's own CLI behavior (and
performance) is unchanged.

## Quick start

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/tigrawap/locar/pkg/engine"
	"github.com/tigrawap/locar/pkg/metrics"
)

func main() {
	m, err := metrics.RegisterMetrics(prometheus.DefaultRegisterer)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":9090", nil))
	}()

	ctx := context.Background()
	eng, err := engine.New(ctx, engine.Config{Metrics: m})
	if err != nil {
		log.Fatal(err)
	}
	eng.AddDir("/data")
	eng.Start()
	eng.Wait()
}
```

That's it: build a `*metrics.Metrics`, register it, put it in
`engine.Config`, run a scan. The counters accumulate on the registry you
chose, ready to be scraped.

## Registering the counters

`pkg/metrics` gives you four ways to get counters onto a
`prometheus.Registerer`. Pick based on how much control you need over the
registry and error handling.

**`RegisterMetrics(r)` — the one-shot.** Builds a new `*metrics.Metrics` and
registers it in one call. Use this when you don't need to do anything with
the `*Metrics` between building and registering it.

```go
m, err := metrics.RegisterMetrics(reg)
if err != nil {
	log.Fatal(err)
}
```

**`New()` + `Register(r)` — when you want the value first.** Use this when
you need the `*Metrics` before you have a registry (e.g. to pass into
several `engine.Config`s that get built before you decide where to expose
metrics), or want to handle a registration error yourself instead of
`log.Fatal`-ing.

```go
m := metrics.New()
// ... do other setup, decide on a registry ...
if err := m.Register(reg); err != nil {
	// handle, retry, or surface the error your own way
	return err
}
```

**`MustRegister(r)` — for init-time wiring.** Panics instead of returning an
error, which is the usual shape for package-level or `func init()`
registration where a registration failure means the program is
misconfigured and should not start.

```go
var appMetrics = metrics.New()

func init() {
	appMetrics.MustRegister(prometheus.DefaultRegisterer)
}
```

**Default registry vs. a custom one.** All three functions above take any
`prometheus.Registerer`, so you can point them at the global
`prometheus.DefaultRegisterer` or at a registry you own:

```go
// Global default registry — shared with anything else in the process
// that also uses prometheus.DefaultRegisterer (e.g. client_golang's own
// process/go collectors, if you've registered those too).
m, err := metrics.RegisterMetrics(prometheus.DefaultRegisterer)

// A registry scoped to your own use of locar.
reg := prometheus.NewRegistry()
m, err := metrics.RegisterMetrics(reg)
```

Prefer a custom `prometheus.NewRegistry()` as the default for a library
consumer: it avoids global mutable state, keeps locar's series separate from
whatever else the host process registers, and makes tests trivially
isolated — each test gets its own registry instead of fighting over the
package-level default (this is exactly why `pkg/metrics`'s own tests use
`prometheus.NewRegistry()`). Use `prometheus.DefaultRegisterer` only when you
specifically want locar's metrics folded into a `/metrics` endpoint that
already scrapes the default registry.

**Double registration.** A `prometheus.Registerer` rejects a second
collector with the same fully-qualified name plus label set. That means:

- Calling `Register` (directly, via `RegisterMetrics`, or via `MustRegister`)
  twice with the *same* `*metrics.Metrics` on the *same* registry fails on
  the second call — the collectors are already registered.
- Building two separate `*metrics.Metrics` (two `New()` calls) and
  registering both into the *same* registry also fails, since both define
  `locar_file_operations_total{operation=...}` and friends.

Either case returns a `prometheus.AlreadyRegisteredError`. The fix is
structural, not a special error-handling path: build exactly one
`*metrics.Metrics` at startup, register it once, and share that single
value across every `engine.Config` / scan that wants metrics. Don't call
`metrics.New()` (or `RegisterMetrics`) per scan.

## Exposing the metrics

Once registered, expose the registry over HTTP with `promhttp` so Prometheus
can scrape it. The handler depends on which registry you used above.

Default registry:

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

http.Handle("/metrics", promhttp.Handler())
log.Fatal(http.ListenAndServe(":9090", nil))
```

Custom registry:

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

reg := prometheus.NewRegistry()
m, err := metrics.RegisterMetrics(reg)
if err != nil {
	log.Fatal(err)
}

http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
log.Fatal(http.ListenAndServe(":9090", nil))
```

locar itself never starts an HTTP listener or exposes an endpoint — that's
entirely the embedding program's responsibility, as shown above.

## Sharing one `*Metrics` across scans

`*metrics.Metrics` is safe to hand to multiple `engine.Config`s, whether the
resulting `*engine.Explorer`s run one after another or concurrently:
Prometheus counters (`prometheus.CounterVec` and the vectors it returns) are
goroutine-safe, and locar's own scan already increments them from many
worker goroutines per `Explorer`.

Keep in mind the counters are cumulative and process-wide, not per-scan: if
you run three scans back to back against the same `*metrics.Metrics`, the
counters after scan 3 reflect the sum of all three. If you need per-scan
numbers, either:

- record the counter values before and after each scan and diff them
  (`testutil.ToFloat64` is handy for this in tests; in production, read the
  scrape or use the underlying `prometheus.Counter`'s value), or
- give each scan its own `*metrics.Metrics` registered to its own
  `prometheus.NewRegistry()`, and drop that registry when the scan ends.

## Metric reference

All three metrics are `Counter`s (via `CounterVec`) with a single label,
`operation`.

| Metric | Labels | Meaning |
|---|---|---|
| `locar_file_operations_total` | `operation` | Every attempted operation of that kind, regardless of outcome. |
| `locar_file_operation_errors_total` | `operation` | Operations of that kind that failed, including timeouts. |
| `locar_file_operation_timeouts_total` | `operation` | Operations of that kind that exceeded their `--timeout` deadline. |

`New()` pre-registers all six operation values below at zero on all three
vectors, so every series exists from the first scrape rather than appearing
only after the first occurrence.

| Operation | Fires | Only when |
|---|---|---|
| `opendir` | Once per directory, when it is opened. | Always (every directory locar visits). |
| `readdir` | Once per `syscall.ReadDirent` **batch** call inside a directory read — **not** once per directory entry. A directory with thousands of entries read in one batch still counts as one `readdir` op. This is the easiest one to misread; don't use it as an entry count. | Always, per directory. |
| `stat` | Once per file, after fetching file times via `platform.GetFileTimes`. | Only when a time filter is active: `--atime-older`/`--atime-newer`, `--mtime-older`/`--mtime-newer`, or `--ctime-older`/`--ctime-newer` (`AtimeOlderThan`/`AtimeNewerThan`/etc. in `engine.Config`). |
| `lstat` | Once per result, via `os.Lstat`. | Only when sizes are requested: `--with-size` (`Config.WithSizes`). |
| `delete` | Once per found entry removed via `os.Remove` (a file, or an empty directory). | Only when `--delete` (`Config.Delete`) is set. |
| `delete_recursive` | Once per found entry removed via `os.RemoveAll` (a file, or a directory with all its contents). | Only when `--delete-all` (`Config.DeleteAll`) is set. |

## Interpreting the counters

`errors` and `timeouts` are **subsets** of `ops`, not counters alongside it:
an operation that fails is still counted once in `ops` and once in `errors`.
A *timeout* increments both `timeouts` and `errors` — `OpTimeout` calls
`OpError` internally — so `errors` already includes every timeout; you don't
need to add the two together.

Useful PromQL:

```promql
# Error rate by operation.
rate(locar_file_operation_errors_total[5m]) / rate(locar_file_operations_total[5m])
```

```promql
# readdir throughput (batch calls/sec, not entries/sec — see the reference table above).
rate(locar_file_operations_total{operation="readdir"}[5m])
```

```promql
# Any timeouts in the last 15 minutes — a hung-mount signal.
increase(locar_file_operation_timeouts_total[15m]) > 0
```

```promql
# Delete volume over the last hour.
increase(locar_file_operations_total{operation=~"delete|delete_recursive"}[1h])
```

Timeouts deserve their own alert rather than being folded into a generic
error-rate threshold: per the `--timeout` CLI help text ("Timeout for
readdir operations. Error will be reported, but os thread will be kept
hanging"), a timed-out `readdir` doesn't cancel the underlying syscall — the
OS thread stays blocked on it. Sustained timeouts are therefore a sign of a
wedged mount leaking blocked threads, not just slow-but-working I/O, and are
worth paging on independently of the overall error rate.

## Recording your own operations

`Op`, `OpError`, `OpTimeout`, and `Record` are exported, so code that wraps
locar (and does its own filesystem work alongside it) can fold that work
into the same counters using its own operation label value:

```go
m.Op("my_custom_walk")
// ... do the work ...
if err != nil {
	m.OpError("my_custom_walk")
}

// or, equivalently, in one call:
m.Record("my_custom_walk", err)
```

Keep the set of operation label values you introduce small and fixed —
Prometheus counter cardinality is driven by distinct label values, so
derive the operation name from your code path, never from per-file or
per-request data (a filename, an error message, a request ID, and so on).

## Disabling metrics

`nil` is a fully supported value for `engine.Config.Metrics` — it's what
locar's own CLI does today, since `internal/cli.Options.EngineConfig()`
never sets the field and there is no `--metrics` flag. Every method on a
nil `*metrics.Metrics` is a no-op, so leaving it unset costs nothing beyond
the (already-present) function calls at each operation site. locar never
starts its own metrics HTTP listener; exposing `/metrics` is always the
embedding program's job, as shown above.
