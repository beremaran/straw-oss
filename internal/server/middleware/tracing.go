package middleware

import (
	"context"
	"fmt"

	"github.com/kwilabs/straw-proxy-server/internal/observability/logging"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TracingMiddleware returns a middleware that adds OpenTelemetry tracing to requests.
func TracingMiddleware(serviceName string) echo.MiddlewareFunc {
	tracer := otel.Tracer(serviceName)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			ctx := req.Context()

			// 1. Extract trace context from incoming headers
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(req.Header))

			// 2. Create the span
			// We use "http.request" as the span name as requested.
			opts := []trace.SpanStartOption{
				trace.WithAttributes(
					attribute.String("http.method", req.Method),
					attribute.String("http.url", req.URL.String()),
					attribute.String("http.client_ip", c.RealIP()),
					attribute.String("http.user_agent", req.UserAgent()),
					attribute.String("http.host", req.Host),
					attribute.String("http.scheme", c.Scheme()),
					attribute.String("http.route", c.Path()),
				),
				trace.WithSpanKind(trace.SpanKindServer),
			}

			ctx, span := tracer.Start(ctx, "http.request", opts...)
			defer span.End()

			// Extract RequestID and put in context for logging
			reqID := c.Response().Header().Get(echo.HeaderXRequestID)
			if reqID != "" {
				ctx = context.WithValue(ctx, logging.RequestIDKey, reqID)
				span.SetAttributes(attribute.String("request_id", reqID))
			}

			// 3. Inject trace context into the request context for downstream use
			c.SetRequest(req.WithContext(ctx))

			// 4. Add trace_id and request_id to context for logging/debugging
			// Note: Echo's RequestID middleware should run before this to ensure Request ID is present.
			traceID := span.SpanContext().TraceID().String()
			c.Set("trace_id", traceID)

			// 5. Propagate trace context in response headers
			// This allows clients to correlate their requests with server traces.
			c.Response().Header().Set("Trace-Id", traceID)
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(c.Response().Header()))

			// 6. Execute next handler
			err := next(c)

			// 7. Record status and errors
			status := c.Response().Status
			span.SetAttributes(attribute.Int("http.status_code", status))

			if err != nil {
				span.RecordError(err)
				// If it's an echo HTTPError, the status might be different, but Echo usually handles writing it.
				// We rely on c.Response().Status being accurate after the handler chain returns (or before).
				// However, if next() returns an error, Echo's ErrorHandler is called *after* middleware returns in some cases,
				// or *wraps* it. Standard middleware pattern is to handle error, then return it.
				// We can try to extract status from error if it is an HTTPError.
				if httpErr, ok := err.(*echo.HTTPError); ok {
					span.SetAttributes(attribute.Int("http.status_code", httpErr.Code))
					if httpErr.Code >= 500 {
						span.SetStatus(codes.Error, httpErr.Error())
					}
				} else {
					// Generic error, usually 500
					span.SetStatus(codes.Error, err.Error())
				}
			} else {
				if status >= 500 {
					span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
				}
			}

			return err
		}
	}
}
