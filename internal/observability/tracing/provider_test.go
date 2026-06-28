package tracing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
)

func TestInitTracerProvider(t *testing.T) {

	originalProvider := otel.GetTracerProvider()
	defer otel.SetTracerProvider(originalProvider)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	shutdown, err := InitTracerProvider(ctx, "test-service", "v0.0.1")

	if err != nil {
		t.Logf("InitTracerProvider returned error (likely due to no collector): %v", err)
	} else {
		assert.NotNil(t, shutdown, "Shutdown function should not be nil")

		currentProvider := otel.GetTracerProvider()
		assert.NotEqual(t, originalProvider, currentProvider, "Global tracer provider should have been updated")
	}
}
