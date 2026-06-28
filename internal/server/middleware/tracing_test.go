package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw/internal/server/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingMiddleware(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("fail"))
	})

	handler := middleware.TracingMiddleware("test-service")(mux)

	t.Run("successful request creates span", func(t *testing.T) {
		exporter.Reset()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.NotEmpty(t, rec.Header().Get("Trace-Id"), "Response should contain Trace-Id header")

		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		span := spans[0]

		assert.Equal(t, "http.request", span.Name)
		assert.Equal(t, "test-service", span.InstrumentationScope.Name)

		attrs := make(map[string]string)
		for _, kv := range span.Attributes {
			attrs[string(kv.Key)] = kv.Value.String()
		}

		assert.Equal(t, "GET", attrs["http.method"])
		assert.Equal(t, "/test", attrs["http.route"])
		assert.Equal(t, "200", attrs["http.status_code"])
	})

	t.Run("propagates context from headers", func(t *testing.T) {
		exporter.Reset()

		ctx := context.Background()
		tracer := tp.Tracer("test-client")
		ctx, parentSpan := tracer.Start(ctx, "parent-request")

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)

		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		parentSpan.End()

		spans := exporter.GetSpans()
		require.Len(t, spans, 2)

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
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/error", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		spans := exporter.GetSpans()
		require.Len(t, spans, 1)
		span := spans[0]

		attrs := make(map[string]string)
		for _, kv := range span.Attributes {
			attrs[string(kv.Key)] = kv.Value.String()
		}
		assert.Equal(t, "500", attrs["http.status_code"])
		assert.Equal(t, codes.Error, span.Status.Code)
	})
}
