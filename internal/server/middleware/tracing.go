package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/beremaran/straw/internal/observability/logging"
)

// TracingMiddleware creates OpenTelemetry spans for each request.
func TracingMiddleware(serviceName string) func(http.Handler) http.Handler {
	tracer := otel.Tracer(serviceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))

			route := r.Pattern
			if route == "" {
				route = r.URL.Path
			}

			opts := []trace.SpanStartOption{
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.client_ip", getRealIP(r)),
					attribute.String("http.user_agent", r.UserAgent()),
					attribute.String("http.host", r.Host),
					attribute.String("http.scheme", getScheme(r)),
					attribute.String("http.route", route),
				),
				trace.WithSpanKind(trace.SpanKindServer),
			}

			ctx, span := tracer.Start(ctx, "http.request", opts...)
			defer span.End()

			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = w.Header().Get("X-Request-ID")
			}

			if reqID != "" {
				ctx = context.WithValue(ctx, logging.RequestIDKey, reqID)
				span.SetAttributes(attribute.String("request_id", reqID))
			}

			r = r.WithContext(ctx)

			traceID := span.SpanContext().TraceID().String()
			r.Header.Set("Trace-Id", traceID)
			w.Header().Set("Trace-Id", traceID)
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

			sw := NewStatusResponseWriter(w)
			next.ServeHTTP(sw, r)

			span.SetAttributes(attribute.Int("http.status_code", sw.Status))

			if sw.Status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", sw.Status))
			}
		})
	}
}

func getRealIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if before, _, ok := strings.Cut(ip, ","); ok {
			return strings.TrimSpace(before)
		}

		return strings.TrimSpace(ip)
	}

	return r.RemoteAddr
}

func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}

	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}

	return "http"
}
