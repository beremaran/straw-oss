package middleware

import (
	"net/http"

	"github.com/beremaran/straw/internal/server/helper"
)

// ConcurrencyLimiter limits the number of concurrent requests, rejecting excess immediately.
func ConcurrencyLimiter(maxConcurrent int) func(http.Handler) http.Handler {
	if maxConcurrent <= 0 {
		maxConcurrent = 50
	}

	semaphore := make(chan struct{}, maxConcurrent)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()

				next.ServeHTTP(w, r)
			default:
				helper.WriteError(w, http.StatusServiceUnavailable, "server at capacity, please retry")
			}
		})
	}
}

// ConcurrencyLimiterWithBlock limits concurrent requests, blocking excess until capacity is available.
func ConcurrencyLimiterWithBlock(maxConcurrent int) func(http.Handler) http.Handler {
	if maxConcurrent <= 0 {
		maxConcurrent = 50
	}

	semaphore := make(chan struct{}, maxConcurrent)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()

				next.ServeHTTP(w, r)
			case <-ctx.Done():
				helper.WriteError(w, http.StatusServiceUnavailable, "request cancelled while waiting for capacity")
			}
		})
	}
}
