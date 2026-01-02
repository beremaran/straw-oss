package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// InitTracerProvider initializes the global trace provider with an OTLP exporter.
// It returns a shutdown function that should be called when the service terminates.
// If OTEL_SDK_DISABLED is set to "true", tracing is disabled and a no-op shutdown is returned.
func InitTracerProvider(ctx context.Context, serviceName, serviceVersion string) (func(context.Context) error, error) {
	// Check if OpenTelemetry SDK is disabled via environment variable
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		// Return a no-op shutdown function when tracing is disabled
		return func(context.Context) error { return nil }, nil
	}
	// Initialize the OTLP/gRPC exporter.
	// We rely on standard environment variables for configuration:
	// - OTEL_EXPORTER_OTLP_ENDPOINT: target address (e.g., "localhost:4317")
	// - OTEL_EXPORTER_OTLP_INSECURE: "true" to disable TLS
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Create the resource describing this service.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create the trace provider with the exporter and resource.
	// We use a BatchSpanProcessor for efficiency in production.
	bsp := sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithBatchTimeout(5*time.Second),
		sdktrace.WithMaxExportBatchSize(512),
	)

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
	)

	// Set the global TraceProvider.
	otel.SetTracerProvider(tracerProvider)

	// Set the global Propagator to W3C Trace Context (standard for distributed tracing).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return a function to handle clean shutdown.
	shutdown := func(ctx context.Context) error {
		// Shutdown the exporter and flush any pending spans.
		if err := tracerProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown tracer provider: %w", err)
		}
		return nil
	}

	return shutdown, nil
}
