package main

import (
	"net/http"
	"sync/atomic"
)

// newHealthMux serves liveness/readiness probes on the egress health port
// (docs/public/architecture.md: "P0 should prefer direct local /healthz and /readyz"
// for egress; docs/public/architecture.md port mapping). /healthz reports process
// liveness and is 200 for as long as the process runs. /readyz is 200 only
// after successful worker registration and flips to 503 once draining
// begins (docs/public/architecture.md "Worker Graceful Shutdown" step 1), mirroring
// cmd/control/health.go.
func newHealthMux(ready *atomic.Bool) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	return mux
}
