package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/broker"
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

func (noopBroker) CoreRequest(context.Context, string, []byte) ([]byte, error) { return nil, nil }

func (noopBroker) CorePublish(context.Context, string, []byte) error { return nil }

func (noopBroker) CoreSubscribe(context.Context, string, broker.Handler) (broker.Subscription, error) {
	return noopSubscription{}, nil
}

type noopSubscription struct{}

func (noopSubscription) Unsubscribe() error { return nil }

func TestServerAddress(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort: 8080,
	}
	srv := New(cfg, noopBroker{})
	addr := srv.Address()
	if addr != ":8080" {
		t.Fatalf("address = %q, want :8080", addr)
	}
}

func TestParseBytesSize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{"empty", "", 0},
		{"number only", "1024", 1024},
		{"1K", "1K", 1024},
		{"10K", "10K", 10240},
		{"1M", "1M", 1048576},
		{"10M", "10M", 10485760},
		{"1G", "1G", 1073741824},
		{"10G", "10G", 10737418240},
		{"lowercase k", "1k", 1024},
		{"lowercase m", "1m", 1048576},
		{"lowercase g", "1g", 1073741824},
		{"with B suffix", "100B", 100},
		{"with spaces", "  10M  ", 10485760},
		{"zero", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBytesSize(tt.input)
			if got != tt.want {
				t.Errorf("parseBytesSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	healthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "OK" {
		t.Fatalf("body = %q, want OK", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", rec.Header().Get("Content-Type"))
	}
}

func TestStop(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort: 0,
	}
	srv := New(cfg, noopBroker{})

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", ":0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	go func() {
		_ = srv.server.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = srv.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestApplyMiddlewares(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := applyMiddlewares(next,
		func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Custom", "test")
				h.ServeHTTP(w, r)
			})
		},
	)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Custom") != "test" {
		t.Errorf("X-Custom = %q, want test", rec.Header().Get("X-Custom"))
	}
}
