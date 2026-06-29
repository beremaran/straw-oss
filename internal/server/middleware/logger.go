package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"time"
)

// LoggerMiddleware logs request details including method, URI, status, and latency.
func LoggerMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			sw := NewStatusResponseWriter(w)
			next.ServeHTTP(sw, r)

			latency := time.Since(start)
			ctx := r.Context()

			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = sw.Header().Get("X-Request-ID")
			}

			remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				remoteIP = r.RemoteAddr
			}

			attrs := []slog.Attr{
				slog.String("event", "http.request.complete"),
				slog.String("method", r.Method),
				slog.String("uri", r.URL.RequestURI()),
				slog.Int("status", sw.Status),
				slog.String("remote_ip", remoteIP),
				slog.Duration("latency", latency),
				slog.String("user_agent", r.UserAgent()),
			}

			if reqID != "" {
				attrs = append(attrs, slog.String("request_id", reqID))
			}

			if sw.Status >= http.StatusInternalServerError {
				slog.LogAttrs(ctx, slog.LevelError, "request completed with error", attrs...)
			} else {
				slog.LogAttrs(ctx, slog.LevelInfo, "request completed", attrs...)
			}
		})
	}
}
