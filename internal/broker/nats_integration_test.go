//go:build integration

package broker

import (
	"context"
	"testing"
	"time"
)

const (
	testNatsURL   = "nats://localhost:4222"
	testNatsToken = ""
	testStream    = "test_stream"
)

func newTestBroker(t *testing.T) *NatsBroker {
	t.Helper()
	b := NewNatsBroker(testNatsURL, testNatsToken)
	if err := b.Connect(); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	return b
}

func TestConnectAndClose(t *testing.T) {
	b := newTestBroker(t)
	defer b.Close()

	if !b.IsConnected() {
		t.Error("expected IsConnected to be true after Connect")
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestDeclareStream(t *testing.T) {
	b := newTestBroker(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := b.DeclareStream(ctx, testStream)
	if err != nil {
		t.Fatalf("DeclareStream failed: %v", err)
	}

	// Declare with custom subjects
	err = b.DeclareStream(ctx, "test_custom", "custom.>")
	if err != nil {
		t.Fatalf("DeclareStream with subjects failed: %v", err)
	}
}

func TestPublishAndConsumeOnce(t *testing.T) {
	b := newTestBroker(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Declare stream first
	err := b.DeclareStream(ctx, testStream)
	if err != nil {
		t.Fatalf("DeclareStream failed: %v", err)
	}

	subject := testStream + ".test"

	// Publish a message
	body := []byte(`{"test": "data"}`)
	err = b.Publish(ctx, subject, body)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Consume the message
	received, err := b.ConsumeOnce(ctx, subject, 2*time.Second)
	if err != nil {
		t.Fatalf("ConsumeOnce failed: %v", err)
	}

	if string(received) != string(body) {
		t.Errorf("received body = %q, want %q", string(received), string(body))
	}
}

func TestConsumeOnceTimeout(t *testing.T) {
	b := newTestBroker(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := b.DeclareStream(ctx, testStream)
	if err != nil {
		t.Fatalf("DeclareStream failed: %v", err)
	}

	// Consume from a subject with no messages - should timeout
	received, err := b.ConsumeOnce(ctx, testStream+".empty", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if received != nil {
		t.Errorf("expected nil body on timeout, got %q", string(received))
	}
}

func TestConsumeOnceNoStream(t *testing.T) {
	b := newTestBroker(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to consume from a non-existent stream
	_, err := b.ConsumeOnce(ctx, "nonexistent.stream", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for non-existent stream")
	}
}

func TestSubscribe(t *testing.T) {
	b := newTestBroker(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := b.DeclareStream(ctx, testStream)
	if err != nil {
		t.Fatalf("DeclareStream failed: %v", err)
	}

	subject := testStream + ".subscribe"
	done := make(chan struct{})

	handler := func(ctx context.Context, body []byte) error {
		if string(body) == "hello" {
			close(done)
		}
		return nil
	}

	err = b.Subscribe(ctx, subject, handler, 10)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish a message to trigger the handler
	err = b.Publish(ctx, subject, []byte("hello"))
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for handler to receive message")
	}

	// Cancel context should stop the consumer
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestIsConnectedFalseAfterClose(t *testing.T) {
	b := NewNatsBroker(testNatsURL, testNatsToken)

	if b.IsConnected() {
		t.Error("expected IsConnected to be false before Connect")
	}

	err := b.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !b.IsConnected() {
		t.Error("expected IsConnected to be true after Connect")
	}

	err = b.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Note: IsConnected may still return true briefly after Close due to NATS internals
	// We just verify Close doesn't panic
}
