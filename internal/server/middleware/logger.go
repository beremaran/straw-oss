package middleware

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// LoggerMiddleware returns a middleware that logs HTTP requests using slog.
func LoggerMiddleware() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogError:     true,
		LogRemoteIP:  true,
		LogLatency:   true,
		LogUserAgent: true,
		LogRequestID: true,
		HandleError:  true, // Forward error to next handler so it can be handled by global error handler
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			ctx := c.Request().Context()

			// Attributes to log
			attrs := []slog.Attr{
				slog.String("event", "http.request.complete"),
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.String("remote_ip", v.RemoteIP),
				slog.Duration("latency", v.Latency),
				slog.String("user_agent", v.UserAgent),
			}

			if v.RequestID != "" {
				attrs = append(attrs, slog.String("request_id", v.RequestID))
			}

			// We use LogAttrs to pass the context which allows TraceHandler to extract trace_id
			if v.Error != nil {
				attrs = append(attrs, slog.String("error", v.Error.Error()))
				// Log at Error level if status is 5xx or there is an error
				if v.Status >= 500 {
					slog.LogAttrs(ctx, slog.LevelError, "request completed with error", attrs...)
				} else {
					slog.LogAttrs(ctx, slog.LevelInfo, "request completed with client error", attrs...) // 4xx is usually info/warn
				}
			} else {
				slog.LogAttrs(ctx, slog.LevelInfo, "request completed", attrs...)
			}

			return nil
		},
	})
}
