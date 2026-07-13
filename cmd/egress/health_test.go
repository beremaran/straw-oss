package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestEgressHealthzAlwaysOK proves /healthz reports liveness regardless of
// readiness.
func TestEgressHealthzAlwaysOK(t *testing.T) {
	t.Parallel()

	ready := &atomic.Bool{}
	mux := newHealthMux(ready)

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
	mux := newHealthMux(ready)

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
