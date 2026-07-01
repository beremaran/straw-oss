package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/config"
)

const (
	defaultMaxBodySize = "10M"
	testEgressID       = "egress-1"
)

func TestNewServer(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort:              0,
		MaxBodySize:           defaultMaxBodySize,
		EgressID:              testEgressID,
		MaxConcurrentRequests: 1,
	}

	srv := New(cfg, noopBroker{})
	if srv == nil {
		t.Fatal("expected server")
	}
	if srv.mux == nil {
		t.Fatal("expected mux")
	}
}

func TestServerUseMiddleware(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort:              0,
		MaxBodySize:           defaultMaxBodySize,
		EgressID:              testEgressID,
		MaxConcurrentRequests: 1,
	}
	srv := New(cfg, noopBroker{})
	if srv == nil {
		t.Fatal("expected server")
	}
}

func TestServerHealthRoutes(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort:              0,
		MaxBodySize:           defaultMaxBodySize,
		EgressID:              testEgressID,
		MaxConcurrentRequests: 1,
	}
	srv := New(cfg, noopBroker{})

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", ":0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := listener.Addr().String()
	baseURL := "http://" + addr

	go func() {
		_ = srv.server.Serve(listener)
	}()
	defer func() { _ = srv.Stop(context.Background()) }()

	t.Run("Healthz", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/healthz", nil)
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
	})

	t.Run("Readyz", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/readyz", nil)
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
	})
}

type noopBroker struct{}

func (noopBroker) Publish(context.Context, string, []byte) error { return nil }

func (noopBroker) ConsumeOnce(context.Context, string, time.Duration) ([]byte, error) {
	return nil, nil
}
