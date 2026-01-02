package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/server/middleware"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingMiddleware(t *testing.T) {
	// Setup OpenTelemetry with InMemoryExporter
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Setup Echo
	e := echo.New()
	e.Use(middleware.TracingMiddleware("test-service"))

	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/error", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "fail")
	})

	t.Run("successful request creates span", func(t *testing.T) {
		exporter.Reset()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.NotEmpty(t, rec.Header().Get("Trace-Id"), "Response should contain Trace-Id header")

		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		span := spans[0]

		assert.Equal(t, "http.request", span.Name)
		assert.Equal(t, "test-service", span.InstrumentationLibrary.Name)

		attrs := make(map[string]string)
		for _, kv := range span.Attributes {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}

		assert.Equal(t, "GET", attrs["http.method"])
		assert.Equal(t, "/test", attrs["http.route"])
		assert.Equal(t, "200", attrs["http.status_code"])
	})

	t.Run("propagates context from headers", func(t *testing.T) {
		exporter.Reset()

		// Create a parent span
		ctx := context.Background()
		tracer := tp.Tracer("test-client")
		ctx, parentSpan := tracer.Start(ctx, "parent-request")

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		// Inject parent context into headers
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		parentSpan.End()

		spans := exporter.GetSpans()
		require.Len(t, spans, 2) // parent + server span

		// Find server span
		// Find server span
		var serverSpan tracetest.SpanStub
		found := false
		for _, s := range spans {
			if s.Name == "http.request" {
				serverSpan = s
				found = true
				break
			}
		}
		require.True(t, found, "Server span not found")

		assert.Equal(t, parentSpan.SpanContext().TraceID(), serverSpan.SpanContext.TraceID(), "Trace IDs should match")
		assert.Equal(t, parentSpan.SpanContext().SpanID(), serverSpan.Parent.SpanID(), "Parent Span ID should match")
	})

	t.Run("records error status", func(t *testing.T) {
		exporter.Reset()
		req := httptest.NewRequest(http.MethodGet, "/error", nil)
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		span := spans[0]

		attrs := make(map[string]string)
		for _, kv := range span.Attributes {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}
		assert.Equal(t, "500", attrs["http.status_code"])
		// Check that status is set to Error
		assert.Equal(t, codes.Error, span.Status.Code)
	})
}
