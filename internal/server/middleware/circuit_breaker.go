package middleware

import (
	"errors"
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/labstack/echo/v4"
)

// CircuitBreaker returns a middleware that stops requests if the circuit breaker is open.
// It also reports failures to the circuit breaker if the handler returns an error (or specific 5xx errors).
func CircuitBreaker(cb *circuitbreaker.CircuitBreaker) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !cb.Allow() {
				return echo.NewHTTPError(http.StatusServiceUnavailable, "Service Unavailable (Circuit Open)")
			}

			err := next(c)

			if err != nil {
				// Decide what counts as a failure.
				// Usually 5xx errors or connection errors.
				// Echo errors can be wrapped.
				var he *echo.HTTPError
				if errors.As(err, &he) {
					if he.Code >= 500 {
						cb.ReportFailure()
					} else {
						cb.ReportSuccess()
					}
				} else {
					// Unknown error, treat as failure? Or assume 500?
					// In Echo, returning a non-HTTPError usually results in a 500.
					cb.ReportFailure()
				}
				return err
			}

			// Success (2xx, 3xx, 4xx)
			// Note: 4xx is usually client error, so we shouldn't trip breaker.
			cb.ReportSuccess()
			return nil
		}
	}
}
