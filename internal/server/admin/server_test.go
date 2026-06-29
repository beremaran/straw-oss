package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestServer_HealthCheck(t *testing.T) {
	cfg := config.ServerConfig{ManagementPort: 8081}
	s := New(cfg, nil, nil, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.GetHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "OK", rec.Body.String())
}

func TestServer_AuthProtection(t *testing.T) {
	cfg := config.ServerConfig{
		ManagementPort:   8081,
		ManagementAPIKey: "management-secret",
	}
	s := New(cfg, nil, nil, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/some-resource", nil)
	rec := httptest.NewRecorder()
	s.GetHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/some-resource", nil)
	req.Header.Set("Authorization", "Bearer management-secret")
	rec = httptest.NewRecorder()
	s.GetHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
