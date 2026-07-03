package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestHealthzAlwaysOK proves /healthz reports liveness regardless of readiness.
func TestHealthzAlwaysOK(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	mux := newMetricsMux(ready)

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
// once drain clears the flag (docs/planning/29).
func TestReadyzReflectsReadiness(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	mux := newMetricsMux(ready)

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
