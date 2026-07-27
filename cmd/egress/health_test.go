package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	internalegress "github.com/beremaran/straw-oss/internal/egress"
)

// TestEgressHealthzAlwaysOK proves /healthz reports liveness regardless of
// readiness.
func TestEgressHealthzAlwaysOK(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	mux := newHealthMux(ready, prometheus.NewRegistry())

	for _, r := range []bool{true, false} {
		ready.Store(r)

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("/healthz with ready=%v code = %d, want 200", r, rec.Code)
		}
	}
}

// TestEgressReadyzReflectsReadiness proves /readyz is 503 before readiness is
// set and flips to 200 once it is (docs/public/architecture.md, docs/public/architecture.md).
func TestEgressReadyzReflectsReadiness(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	mux := newHealthMux(ready, prometheus.NewRegistry())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz before ready code = %d, want 503", rec.Code)
	}

	ready.Store(true)
	rec = httptest.NewRecorder()
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

// TestEgressMetricsServedOnHealthPort proves the worker's single listener also
// serves the scrape endpoint (docs/public/operations.md) and that
// straw_egress_ready reports the same readiness /readyz does.
func TestEgressMetricsServedOnHealthPort(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	ready.Store(true)

	reg := prometheus.NewRegistry()
	metrics := internalegress.NewMetrics(reg)
	internalegress.RegisterReadinessCollector(reg, ready)
	metrics.ObserveRequest("", time.Second)

	mux := newHealthMux(ready, reg)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics code = %d, want 200", rec.Code)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}

	for _, want := range []string{`straw_egress_assignments_total{outcome="success"} 1`, "straw_egress_ready 1"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("/metrics body missing %q:\n%s", want, body)
		}
	}
}
