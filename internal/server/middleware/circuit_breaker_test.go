package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	cb := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 1,
		ResetTimeout:     100 * time.Millisecond,
	})

	mw := CircuitBreaker(cb)

	// Handler that fails
	failHandler := func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "fail")
	}

	h := mw(failHandler)

	// 1. First request fails -> Trip breaker
	err := h(c)
	assert.Error(t, err)
	assert.Equal(t, circuitbreaker.StateOpen, cb.State())

	// 2. Second request -> Circuit Open (503)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req, rec2)
	err = h(c2)
	assert.Error(t, err)
	he, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, he.Code)

	// 3. Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// 4. Third request -> Half-Open -> Success -> Closed
	successHandler := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}
	hSuccess := mw(successHandler)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req, rec3)

	err = hSuccess(c3)
	assert.NoError(t, err)
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())
}
