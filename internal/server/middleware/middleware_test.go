package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBodyLimitRejectsLargeBody(t *testing.T) {
	handler := BodyLimit(3)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = r.Body.Read(make([]byte, 4))
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader("toolarge"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestRequestIDSetsResponseHeader(t *testing.T) {
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "req-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "req-1" {
		t.Fatalf("X-Request-ID = %q, want req-1", rec.Header().Get("X-Request-ID"))
	}
}

func TestBodyLimitAllowsSmallBody(t *testing.T) {
	handler := BodyLimit(100)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader("small"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestBodyLimitZeroAllowsAll(t *testing.T) {
	handler := BodyLimit(0)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader("any size"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequestIDGeneratesNewID(t *testing.T) {
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-ID")
	if got == "" {
		t.Fatal("X-Request-ID should be set")
	}
	if !strings.HasPrefix(got, "req_") {
		t.Fatalf("X-Request-ID = %q, want prefix 'req_'", got)
	}
}

func TestConcurrencyLimiterAllowsRequest(t *testing.T) {
	handler := ConcurrencyLimiter(2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestConcurrencyLimiterRejectsExcess(t *testing.T) {
	block := make(chan struct{})
	defer close(block)

	handler := ConcurrencyLimiter(1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-block // block forever to hold the slot
		w.WriteHeader(http.StatusOK)
	}))

	// First request acquires the slot and blocks
	req1 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	go handler.ServeHTTP(rec1, req1)

	// Give the first request time to acquire
	time.Sleep(50 * time.Millisecond)

	// Second request should be rejected (503)
	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec2.Code, http.StatusServiceUnavailable)
	}
}

func TestCORSHeaders(t *testing.T) {
	handler := CORS()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Allow-Origin = %q, want *", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
		t.Errorf("Allow-Methods = %q, want GET, POST, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	}
	if rec.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-Request-ID" {
		t.Errorf("Allow-Headers = %q, want Content-Type, X-Request-ID", rec.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestCORSPreflight(t *testing.T) {
	handler := CORS()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("OPTIONS handler should not call next")
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRecoverCatchesPanic(t *testing.T) {
	handler := Recover()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}
}

func TestRecoverPassesThrough(t *testing.T) {
	handler := Recover()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLoggerMiddleware(t *testing.T) {
	handler := LoggerMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/test-path", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWriteError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}
}
