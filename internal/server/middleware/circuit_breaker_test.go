package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	cb := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 1,
		ResetTimeout:     100 * time.Millisecond,
	})

	mw := CircuitBreaker(cb)

	failHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("fail"))
	})

	h := mw(failHandler)

	h.ServeHTTP(rec, req)
	assert.Equal(t, circuitbreaker.StateOpen, cb.State())

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec2.Code)

	time.Sleep(150 * time.Millisecond)

	successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	hSuccess := mw(successHandler)
	rec3 := httptest.NewRecorder()

	hSuccess.ServeHTTP(rec3, req)
	assert.Equal(t, http.StatusOK, rec3.Code)
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())
}
