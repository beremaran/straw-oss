package server

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/beremaran/straw/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewServer(t *testing.T) {
	cfg := config.ServerConfig{
		HTTPPort:    0,
		MaxBodySize: "10M",
	}

	srv := New(cfg, nil, nil, nil, nil, nil, nil)
	assert.NotNil(t, srv)
	assert.NotNil(t, srv.mux)
}

func TestServerUseMiddleware(t *testing.T) {
	cfg := config.ServerConfig{
		HTTPPort:    0,
		MaxBodySize: "10M",
	}
	srv := New(cfg, nil, nil, nil, nil, nil, nil)
	assert.NotNil(t, srv)
}

func TestServerHealthRoutes(t *testing.T) {
	cfg := config.ServerConfig{
		HTTPPort:    0,
		MaxBodySize: "10M",
	}
	srv := New(cfg, nil, nil, nil, nil, nil, nil)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := listener.Addr().String()
	baseURL := "http://" + addr

	go func() {
		_ = srv.server.Serve(listener)
	}()
	defer srv.Stop(context.Background())

	t.Run("Healthz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/healthz")
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Readyz", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/readyz")
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
