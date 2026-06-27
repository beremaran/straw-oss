package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConcurrencyLimiter(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		mw := ConcurrencyLimiter(10)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects requests over limit", func(t *testing.T) {
		limit := 2
		mw := ConcurrencyLimiter(limit)

		startCh := make(chan struct{})
		holdCh := make(chan struct{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startCh <- struct{}{}
			<-holdCh
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))

		var wg sync.WaitGroup
		for i := 0; i < limit; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
			}()
			<-startCh
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

		close(holdCh)
		wg.Wait()
	})

	t.Run("uses default when zero limit", func(t *testing.T) {
		mw := ConcurrencyLimiter(0)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("releases slot after request completes", func(t *testing.T) {
		limit := 1
		mw := ConcurrencyLimiter(limit)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))

		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec1 := httptest.NewRecorder()
		handler.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)

		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec2 := httptest.NewRecorder()
		handler.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusOK, rec2.Code)
	})
}

func TestConcurrencyLimiterWithBlock(t *testing.T) {
	t.Run("blocks and waits for slot", func(t *testing.T) {
		limit := 1
		mw := ConcurrencyLimiterWithBlock(limit)

		releaseCh := make(chan struct{})
		var callCount atomic.Int32

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount.Add(1)
			if callCount.Load() == 1 {
				<-releaseCh
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}))

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}()

		time.Sleep(10 * time.Millisecond)

		secondDone := make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			close(secondDone)
		}()

		select {
		case <-secondDone:
			t.Fatal("second request should be blocked")
		case <-time.After(50 * time.Millisecond):

		}

		close(releaseCh)

		select {
		case <-secondDone:

		case <-time.After(1 * time.Second):
			t.Fatal("second request should have completed")
		}

		wg.Wait()
	})
}
