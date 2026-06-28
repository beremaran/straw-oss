package middleware

import (
	"net/http"

	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/beremaran/straw/internal/server/helper"
)

func CircuitBreaker(cb *circuitbreaker.CircuitBreaker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cb.Allow() {
				helper.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable (Circuit Open)")
				return
			}

			sw := NewStatusResponseWriter(w)
			next.ServeHTTP(sw, r)

			if sw.Status >= 500 {
				cb.ReportFailure()
			} else {
				cb.ReportSuccess()
			}
		})
	}
}
