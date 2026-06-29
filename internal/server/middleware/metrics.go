package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/metrics"
)

// MetricsMiddleware records request metrics including count, duration, and routing information.
func MetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if metrics.ActiveSessions != nil {
				metrics.ActiveSessions.Inc()
				defer metrics.ActiveSessions.Dec()
			}

			start := time.Now()
			sw := NewStatusResponseWriter(w)
			next.ServeHTTP(sw, r)

			duration := time.Since(start).Seconds()

			status := strconv.Itoa(sw.Status)

			ruleID := "unknown"
			if rule, ok := r.Context().Value(ContextRoutingRuleKey{Value: RoutingRuleContextKey}).(*domain.RoutingRule); ok && rule != nil {
				ruleID = rule.ID
			}

			fingerprint := "unknown"

			if metrics.RequestsTotal != nil {
				metrics.RequestsTotal.WithLabelValues(status, ruleID, fingerprint).Inc()
			}

			if metrics.RequestDuration != nil {
				metrics.RequestDuration.WithLabelValues(ruleID, status).Observe(duration)
			}
		})
	}
}
