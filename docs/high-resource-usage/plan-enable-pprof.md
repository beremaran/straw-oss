# Implementation Plan: Enable pprof Profiling

## Problem Statement

There is no way to capture CPU and memory profiles from running services, making it impossible to identify performance bottlenecks with certainty.

## Proposed Changes

### [MODIFY] [registry.go](file:///home/beremaran/projects/straw-proxy/internal/observability/metrics/registry.go)

Add helper function to register pprof handlers:

```go
import "net/http/pprof"

// RegisterPprof adds pprof handlers to the given mux.
func RegisterPprof(mux *http.ServeMux) {
    mux.HandleFunc("/debug/pprof/", pprof.Index)
    mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
    mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
    mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
    mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
    mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
    mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
    mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
    mux.Handle("/debug/pprof/block", pprof.Handler("block"))
    mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
}
```

### [MODIFY] [main.go (endpoint)](file:///home/beremaran/projects/straw-proxy/cmd/endpoint/main.go)

Wire pprof into health handler:

```go
func setupHealthHandler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
    mux.Handle("/metrics", metrics.Handler())
    metrics.RegisterPprof(mux)  // Add this line
    return mux
}
```

### [MODIFY] [main.go (relay-server)](file:///home/beremaran/projects/straw-proxy/cmd/relay-server/main.go)

Wire pprof into metrics server:

```go
if cfg.Observability.MetricsEnabled {
    metrics.Init()
    mux := http.NewServeMux()
    mux.Handle("/metrics", metrics.Handler())
    metrics.RegisterPprof(mux)  // Add this line
    
    metricsSrv = &http.Server{
        Addr:    fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
        Handler: mux,
    }
    // ...
}
```

## Usage Examples

### Capture CPU Profile (30 seconds)

```bash
# Endpoint (port 8081 by default)
curl -o cpu.prof http://localhost:8081/debug/pprof/profile?seconds=30

# Analyze
go tool pprof -http=:8082 cpu.prof
```

### Capture Heap Profile

```bash
curl -o heap.prof http://localhost:8081/debug/pprof/heap

# Show top memory consumers
go tool pprof -top heap.prof
```

### Check Goroutine Count

```bash
curl http://localhost:8081/debug/pprof/goroutine?debug=1
```

### Look For Blocking

```bash
curl -o block.prof http://localhost:8081/debug/pprof/block
go tool pprof -http=:8082 block.prof
```

## Security Consideration

> [!WARNING]
> pprof endpoints expose internal state. In production, consider:
>
> - Binding metrics server to localhost only
> - Adding authentication middleware
> - Using a separate internal network

## Verification Plan

### Automated Tests

- Verify `/debug/pprof/` returns 200
- Verify profile can be captured and parsed

### Manual Verification

- Run load test
- Capture profiles and identify hotspots
