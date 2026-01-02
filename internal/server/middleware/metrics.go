package middleware

import (
	"strconv"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/server/metrics"
	"github.com/labstack/echo/v4"
)

// MetricsMiddleware tracks request duration, count, and active sessions.
func MetricsMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if metrics.ActiveSessions != nil {
				metrics.ActiveSessions.Inc()
				defer metrics.ActiveSessions.Dec()
			}

			start := time.Now()
			err := next(c)
			duration := time.Since(start).Seconds()

			status := strconv.Itoa(c.Response().Status)

			// Extract rule ID if available
			ruleID := "unknown"
			if rule, ok := c.Get(ContextKeyRoutingRule).(*domain.RoutingRule); ok && rule != nil {
				ruleID = rule.ID
			}

			// Extract fingerprint if available (requires request filter or updated context)
			// Assuming it might be in context or just "unknown" for now if not explicitly set
			fingerprint := "unknown"
			// Placeholder for fingerprint extraction logic if it becomes available in context

			if metrics.RequestsTotal != nil {
				metrics.RequestsTotal.WithLabelValues(status, ruleID, fingerprint).Inc()
			}

			if metrics.RequestDuration != nil {
				metrics.RequestDuration.WithLabelValues(ruleID, status).Observe(duration)
			}

			return err
		}
	}
}
