package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/beremaran/straw-oss/v2/internal/control"
)

// TestHealthzAlwaysOK proves /healthz reports liveness regardless of readiness.
func TestHealthzAlwaysOK(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	mux := newMetricsMux(ready, prometheus.NewRegistry())

	for _, r := range []bool{true, false} {
		ready.Store(r)

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("/healthz with ready=%v code = %d, want 200", r, rec.Code)
		}
	}
}

// TestReadyzReflectsReadiness proves /readyz is 200 when ready and flips to 503
// once drain clears the flag (docs/public/architecture.md).
func TestReadyzReflectsReadiness(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	mux := newMetricsMux(ready, prometheus.NewRegistry())

	ready.Store(true)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz ready code = %d, want 200", rec.Code)
	}

	ready.Store(false)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz draining code = %d, want 503", rec.Code)
	}
}

func TestReadyzReflectsSharedState(t *testing.T) {
	t.Parallel()
	ready := &atomic.Bool{}
	ready.Store(true)
	sharedReady := &atomic.Bool{}
	sharedReady.Store(false)
	mux := newMetricsMuxWithCheck(ready, prometheus.NewRegistry(), sharedReady.Load)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz during shared-state outage = %d", rec.Code)
	}
	sharedReady.Store(true)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz after shared-state recovery = %d", rec.Code)
	}
}

// TestMetricsServesRegisteredSeries proves /metrics on the metrics port
// (docs/public/architecture.md) returns 200 with the registered P0
// series exposed in Prometheus text format.
func TestMetricsServesRegisteredSeries(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	metrics := control.NewMetrics(reg)
	// straw_active_requests has no labels, so it is always present; verify
	// via a metric with no dynamic labels rather than one of the
	// unbounded request-label series, which (correctly, per Prometheus
	// client behavior) carry no series until first observed.
	metrics.IncActiveRequests()

	mux := newMetricsMux(&atomic.Bool{}, reg)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics code = %d, want 200", rec.Code)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}

	if !strings.Contains(string(body), "straw_active_requests 1") {
		t.Fatalf("/metrics body missing straw_active_requests 1:\n%s", body)
	}
}
