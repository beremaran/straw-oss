package server

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewServer(t *testing.T) {
	cfg := config.ServerConfig{
		HTTPPort:    0, // Random port
		MaxBodySize: "10M",
	}

	srv := New(cfg, nil, nil, nil, nil, nil, nil)
	assert.NotNil(t, srv)
	assert.NotNil(t, srv.echo)
}

func TestServerUseMiddleware(t *testing.T) {
	cfg := config.ServerConfig{
		HTTPPort: 0,
	}
	srv := New(cfg, nil, nil, nil, nil, nil, nil)
	// Check if middleware are registered by checking the routes or just ensuring no panic
	// Echo doesn't expose middleware list easily without reflection or private fields access in some versions
	// But init shouldn't panic
	assert.NotNil(t, srv)
}

func TestServerHealthRoutes(t *testing.T) {
	cfg := config.ServerConfig{
		HTTPPort: 0,
	}
	srv := New(cfg, nil, nil, nil, nil, nil, nil)

	// Pre-create listener to avoid race when reading Addr()
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	// Capture address before starting goroutine
	addr := listener.Addr().String()
	baseURL := "http://" + addr

	// Start server in background
	go func() {
		_ = srv.echo.Server.Serve(listener)
	}()
	defer srv.Stop(context.Background())

	t.Run("Healthz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/healthz")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Readyz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/readyz")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
