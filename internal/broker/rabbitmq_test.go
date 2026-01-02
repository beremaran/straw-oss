package broker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestRabbitMQBroker_Integration(t *testing.T) {
	// This test requires a running RabbitMQ instance.
	// Since we know the environment has dockerized rabbitmq on localhost:5672, we run it.

	opts := []Option{
		Addrs("amqp://guest:guest@localhost:5672/"),
		ReconnectWait(1 * time.Second),
	}
	broker := NewRabbitMQBroker(opts...)

	// Connect
	if err := broker.Connect(); err != nil {
		t.Fatalf("Failed to connect to broker: %v", err)
	}
	defer broker.Close()

	// Test unique queue name
	queueName := fmt.Sprintf("test_queue_%d", time.Now().UnixNano())

	// Channel to receive result
	received := make(chan string, 1)

	// Subscribe
	handler := func(ctx context.Context, body []byte) error {
		received <- string(body)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := broker.Subscribe(ctx, queueName, handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Give time for subscription to be active
	time.Sleep(100 * time.Millisecond)

	// Publish
	msg := "hello world"
	if err := broker.Publish(ctx, "", queueName, []byte(msg)); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	select {
	case body := <-received:
		if body != msg {
			t.Errorf("Expected message %q, got %q", msg, body)
		}
	case <-time.After(3 * time.Second):
		t.Error("Timed out waiting for message")
	}
}

func TestRabbitMQBroker_TracingIntegration(t *testing.T) {
	// This test verifies that context is propagated via AMQP headers.
	opts := []Option{
		Addrs("amqp://guest:guest@localhost:5672/"),
		ReconnectWait(1 * time.Second),
	}
	broker := NewRabbitMQBroker(opts...)

	if err := broker.Connect(); err != nil {
		t.Fatalf("Failed to connect to broker: %v", err)
	}
	defer broker.Close()

	queueName := fmt.Sprintf("test_tracing_queue_%d", time.Now().UnixNano())

	// Start a trace
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	tracer := tp.Tracer("test-tracer")
	ctx, span := tracer.Start(context.Background(), "test-root-span")
	defer span.End()

	traceID := span.SpanContext().TraceID()

	// Channel to receive result
	receivedTraceID := make(chan string, 1)

	// Subscribe
	handler := func(ctx context.Context, body []byte) error {
		span := trace.SpanFromContext(ctx)
		receivedTraceID <- span.SpanContext().TraceID().String()
		return nil
	}

	if err := broker.Subscribe(ctx, queueName, handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Publish with context
	if err := broker.Publish(ctx, "", queueName, []byte("trace-test")); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	select {
	case received := <-receivedTraceID:
		if received != traceID.String() {
			t.Errorf("Expected TraceID %s, got %s", traceID.String(), received)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}
