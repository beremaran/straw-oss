package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestConcurrencyLimiter(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		e := echo.New()
		mw := ConcurrencyLimiter(10)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects requests over limit", func(t *testing.T) {
		e := echo.New()
		limit := 2
		mw := ConcurrencyLimiter(limit)

		// Use channels to control request flow
		startCh := make(chan struct{})
		holdCh := make(chan struct{})

		handler := mw(func(c echo.Context) error {
			startCh <- struct{}{} // Signal that request started
			<-holdCh              // Wait to be released
			return c.String(http.StatusOK, "OK")
		})

		// Start 'limit' number of requests that will hold
		var wg sync.WaitGroup
		for i := 0; i < limit; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				_ = handler(c)
			}()
			<-startCh // Wait for this request to start
		}

		// Now try one more request - it should be rejected
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler(c)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusServiceUnavailable, httpErr.Code)

		// Release the held requests
		close(holdCh)
		wg.Wait()
	})

	t.Run("uses default when zero limit", func(t *testing.T) {
		e := echo.New()
		mw := ConcurrencyLimiter(0) // Should default to 50

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("releases slot after request completes", func(t *testing.T) {
		e := echo.New()
		limit := 1
		mw := ConcurrencyLimiter(limit)

		handler := mw(func(c echo.Context) error {
			return c.String(http.StatusOK, "OK")
		})

		// First request should succeed
		req1 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec1 := httptest.NewRecorder()
		c1 := e.NewContext(req1, rec1)
		err := handler(c1)
		assert.NoError(t, err)

		// Second request should also succeed (slot was released)
		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec2 := httptest.NewRecorder()
		c2 := e.NewContext(req2, rec2)
		err = handler(c2)
		assert.NoError(t, err)
	})
}

func TestConcurrencyLimiterWithBlock(t *testing.T) {
	t.Run("blocks and waits for slot", func(t *testing.T) {
		e := echo.New()
		limit := 1
		mw := ConcurrencyLimiterWithBlock(limit)

		releaseCh := make(chan struct{})
		var callCount atomic.Int32

		handler := mw(func(c echo.Context) error {
			callCount.Add(1)
			if callCount.Load() == 1 {
				<-releaseCh // First request waits
			}
			return c.String(http.StatusOK, "OK")
		})

		// Start first request (will block on releaseCh)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			_ = handler(c)
		}()

		// Allow first request to acquire semaphore
		time.Sleep(10 * time.Millisecond)

		// Start second request (should block waiting for semaphore)
		secondDone := make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			_ = handler(c)
			close(secondDone)
		}()

		// Verify second request hasn't completed
		select {
		case <-secondDone:
			t.Fatal("second request should be blocked")
		case <-time.After(50 * time.Millisecond):
			// Expected - second request is blocked
		}

		// Release first request
		close(releaseCh)

		// Wait for second request to complete
		select {
		case <-secondDone:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("second request should have completed")
		}

		wg.Wait()
	})
}
