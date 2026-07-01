//go:build integration

package egress

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/protocol"
)

func TestWorkerStart(t *testing.T) {
	cfg := &config.EgressConfig{
		ID:               "test-egress-start",
		ConcurrencyLimit: 5,
		NATS: config.NATSConfig{
			URL:   "nats://localhost:4222",
			Token: "",
		},
	}

	// Create a custom executor that does nothing
	executor := &noopExecutor{}

	w := NewWorker(cfg, executor)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start the worker - it will block until context is done
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to stop the worker
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("Start returned error (expected): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestConnectWorkerBroker(t *testing.T) {
	cfg := &config.EgressConfig{
		NATS: config.NATSConfig{
			URL:   "nats://localhost:4222",
			Token: "",
		},
	}

	b, err := connectWorkerBroker(cfg)
	if err != nil {
		t.Fatalf("connectWorkerBroker failed: %v", err)
	}
	defer b.Close()

	if !b.IsConnected() {
		t.Fatal("expected broker to be connected")
	}
}

func TestSetupEgressExecutor(t *testing.T) {
	cfg := &config.EgressConfig{ID: "test"}

	// With nil executor, should create HTTP client
	executor, cleanup := setupEgressExecutor(cfg, nil)
	defer cleanup()

	if executor == nil {
		t.Fatal("expected executor to be non-nil")
	}

	// With custom executor, should return it
	custom := &noopExecutor{}
	executor2, cleanup2 := setupEgressExecutor(cfg, custom)
	defer cleanup2()

	if executor2 != custom {
		t.Fatal("expected custom executor to be returned")
	}
}

func TestWorkerRunWithConfig(t *testing.T) {
	cfg := &config.EgressConfig{
		ID:               "test-egress-run",
		ConcurrencyLimit: 5,
		NATS: config.NATSConfig{
			URL:   "nats://localhost:4222",
			Token: "",
		},
	}

	executor := &noopExecutor{}
	w := NewWorker(cfg, executor)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("Start returned error (expected): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

type noopExecutor struct{}

func (n *noopExecutor) Do(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	return &protocol.Response{
		RequestID:  req.ID,
		StatusCode: 200,
		Body:       []byte("noop"),
	}, nil
}

func TestConsumer_HandleMessageWithRealHandler(t *testing.T) {
	b := broker.NewNatsBroker("nats://localhost:4222", "")
	if err := b.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer b.Close()

	// Declare stream and subscribe
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := b.DeclareStream(ctx, "test_msg_handler", "test_msg_handler.>"); err != nil {
		t.Fatalf("declare stream: %v", err)
	}

	subject := "test_msg_handler.tasks"
	var receivedBody []byte
	var bodyMu sync.Mutex
	handler := func(ctx context.Context, body []byte) error {
		bodyMu.Lock()
		receivedBody = append([]byte(nil), body...)
		bodyMu.Unlock()
		return nil
	}

	if err := b.Subscribe(ctx, subject, handler, 10); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Publish a message
	body := []byte(`{"id":"test-1","method":"GET","url":"https://httpbin.org/get"}`)
	if err := b.Publish(ctx, subject, body); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Wait for handler to receive
	time.Sleep(200 * time.Millisecond)

	bodyMu.Lock()
	defer bodyMu.Unlock()
	if len(receivedBody) == 0 {
		t.Fatal("expected handler to receive body")
	}
}

func TestConsumer_HandleMessageFullFlow(t *testing.T) {
	b := broker.NewNatsBroker("nats://localhost:4222", "")
	if err := b.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Declare streams
	if err := b.DeclareStream(ctx, "test_flow", "test_flow.>"); err != nil {
		t.Fatalf("declare stream: %v", err)
	}
	if err := b.DeclareStream(ctx, "test_flow_results", "test_flow_results.>"); err != nil {
		t.Fatalf("declare stream: %v", err)
	}

	executor := &noopExecutor{}
	publisher := NewPublisher(b)

	c := NewConsumer(b, executor, "test-flow", 5, publisher.Publish)

	// Start the consumer
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Start(ctx)
	}()

	// Give consumer time to subscribe
	time.Sleep(100 * time.Millisecond)

	// Publish a task
	taskBody, err := protocol.MarshalRequest(&protocol.Request{
		ID:      "flow-req-1",
		Method:  "GET",
		URL:     "https://httpbin.org/get",
		ReplyTo: "test_flow_results.flow-req-1",
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}

	subject := "test_flow.tasks"
	if err := b.Publish(ctx, subject, taskBody); err != nil {
		t.Fatalf("publish task: %v", err)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Stop the consumer
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("Start returned error (expected): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestEgressRun(t *testing.T) {
	// Set environment variables for Run to use
	t.Setenv("EGRESS_ID", "test-egress-run-env")
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("NATS_TOKEN", "")
	t.Setenv("CONCURRENCY_LIMIT", "5")

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run()
	}()

	// Give it time to start
	time.Sleep(200 * time.Millisecond)

	// Cancel to stop
	// We can't easily send a signal to the goroutine, so we'll just verify it started
	// by checking that it doesn't return immediately

	// For now, just verify it started and doesn't error immediately
	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("Run returned error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		// Good - it's still running
	}

	// Send SIGTERM to stop
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		time.Sleep(10 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("Run returned error (expected): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after signal")
	}
}
