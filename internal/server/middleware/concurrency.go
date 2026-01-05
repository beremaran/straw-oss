// Package middleware provides HTTP middleware for the relay server.
package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// ConcurrencyLimiter creates middleware that limits concurrent in-flight requests.
// When the limit is reached, new requests are rejected with 503 Service Unavailable.
func ConcurrencyLimiter(maxConcurrent int) echo.MiddlewareFunc {
	if maxConcurrent <= 0 {
		maxConcurrent = 50 // Default to 50 if not specified
	}
	semaphore := make(chan struct{}, maxConcurrent)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try to acquire a slot without blocking
			select {
			case semaphore <- struct{}{}:
				// Got a slot, proceed with request
				defer func() { <-semaphore }()
				return next(c)
			default:
				// No slots available, reject the request
				return echo.NewHTTPError(http.StatusServiceUnavailable,
					"server at capacity, please retry")
			}
		}
	}
}

// ConcurrencyLimiterWithBlock creates middleware that limits concurrent requests,
// but blocks waiting for a slot rather than immediately rejecting.
// This is useful when you prefer to queue requests rather than reject them.
func ConcurrencyLimiterWithBlock(maxConcurrent int) echo.MiddlewareFunc {
	if maxConcurrent <= 0 {
		maxConcurrent = 50
	}
	semaphore := make(chan struct{}, maxConcurrent)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// Wait for a slot or context cancellation
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
				return next(c)
			case <-ctx.Done():
				return echo.NewHTTPError(http.StatusServiceUnavailable,
					"request cancelled while waiting for capacity")
			}
		}
	}
}
