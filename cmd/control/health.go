package main

import (
	"net/http"
	"sync/atomic"
)

// newMetricsMux serves liveness and readiness probes on the metrics port
// (docs/planning/28, port 9090). /healthz reports process liveness and is 200
// for as long as the process runs. /readyz reports readiness and flips to 503
// once ready is cleared, which happens when shutdown drain begins
// (docs/planning/29 step 1: "marks readiness false"), so orchestrators and
// compose healthchecks stop routing to a draining Control.
func newMetricsMux(ready *atomic.Bool) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "draining", http.StatusServiceUnavailable)

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	return mux
}
