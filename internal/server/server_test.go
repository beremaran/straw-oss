package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.NotNil(t, srv)
	assert.NotNil(t, srv.mux)
}

func TestServerUseMiddleware(t *testing.T) {
	cfg := config.ControlConfig{
		HTTPPort:              0,
		MaxBodySize:           defaultMaxBodySize,
		EgressID:              testEgressID,
		MaxConcurrentRequests: 1,
	}
	srv := New(cfg, noopBroker{})
	assert.NotNil(t, srv)
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
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Readyz", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/readyz", nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

type noopBroker struct{}

func (noopBroker) Publish(context.Context, string, []byte) error { return nil }

func (noopBroker) Subscribe(context.Context, string, broker.Handler, ...broker.SubscribeOption) error {
	return nil
}

func (noopBroker) ConsumeOnce(context.Context, string, time.Duration) ([]byte, error) {
	return nil, broker.ErrTimeout
}

func (noopBroker) DeclareStream(context.Context, string, ...string) error { return nil }

func (noopBroker) IsConnected() bool { return true }

func (noopBroker) Close() error { return nil }
