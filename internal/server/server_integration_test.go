//go:build integration

package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
)

func TestStart(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort: 8888,
	}
	srv := New(cfg, noopBroker{})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Make a request to verify server is running
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8888/healthz", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Stop the server
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Stop(ctx)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed && err.Error() != "listen: http: Server closed" {
			t.Fatalf("serve error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestDeclareStreamWithTimeout(t *testing.T) {
	b := broker.NewNatsBroker("nats://localhost:4222", "")
	if err := b.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer b.Close()

	cfg := config.ControlConfig{
		StreamTimeout: 5 * time.Second,
	}

	err := declareStreamWithTimeout(&cfg, b, "test_declare", "test_declare.>")
	if err != nil {
		t.Fatalf("declareStreamWithTimeout failed: %v", err)
	}
}

func TestStopError(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort: 8890,
	}
	srv := New(cfg, noopBroker{})

	// Create a listener and start serving without using Start()
	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", ":8890")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.server.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Stop with a very short timeout to trigger error path
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Stop should work even with short timeout (it'll just not complete gracefully)
	_ = srv.Stop(ctx)

	// Clean up
	_ = listener.Close()
	select {
	case <-errCh:
	case <-time.After(1 * time.Second):
	}
}

func TestRunWithConfig(t *testing.T) {
	cfg := &config.ControlConfig{
		HTTPPort:        8891,
		EgressID:        "test-egress",
		ResultTimeout:   5 * time.Second,
		AllowPrivateIPs: true,
		NATS: config.NATSConfig{
			URL:   "nats://localhost:4222",
			Token: "",
		},
		ShutdownTimeout: 2 * time.Second,
		StreamTimeout:   5 * time.Second,
	}

	// RunWithConfig starts the server, declares streams, and blocks on interrupt
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunWithConfig(cfg, "test-version")
	}()

	// Give it time to start
	time.Sleep(200 * time.Millisecond)

	// Verify the server is running
	resp, err := http.Get("http://localhost:8891/healthz")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Send SIGTERM to the current process to trigger shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		time.Sleep(10 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()

	// Wait for RunWithConfig to return
	select {
	case err := <-errCh:
		if err != nil {
			t.Logf("RunWithConfig returned error (expected): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunWithConfig did not return after signal")
	}
}

func TestRun(t *testing.T) {
	// Set environment variables for Run to use
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("NATS_TOKEN", "")
	t.Setenv("HTTP_PORT", "8892")
	t.Setenv("CONTROL_EGRESS_ID", "test-egress-run")
	t.Setenv("RESULT_TIMEOUT", "5s")
	t.Setenv("MAX_CONCURRENT_REQUESTS", "10")
	t.Setenv("MAX_BODY_SIZE", "10M")
	t.Setenv("ALLOW_PRIVATE_IPS", "true")
	t.Setenv("SHUTDOWN_TIMEOUT", "2s")
	t.Setenv("NATS_STREAM_TIMEOUT", "5s")

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run("test-version")
	}()

	// Give it time to start
	time.Sleep(200 * time.Millisecond)

	// Verify the server is running
	resp, err := http.Get("http://localhost:8892/healthz")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Send SIGTERM to trigger shutdown
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
