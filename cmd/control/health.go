package main

import (
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// newMetricsMux serves liveness/readiness probes and the Prometheus scrape
// endpoint on the metrics port (docs/public/architecture.md, port 9090). /healthz
// reports process liveness and is 200 for as long as the process runs.
// /readyz reports readiness and flips to 503 once ready is cleared, which
// happens when shutdown drain begins (docs/public/architecture.md step 1: "marks
// readiness false"), so orchestrators and compose healthchecks stop routing
// to a draining Control. /metrics serves the P0 series
// (docs/public/architecture.md) registered against reg.
func newMetricsMux(ready *atomic.Bool, reg *prometheus.Registry) *http.ServeMux {
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

	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	return mux
}
