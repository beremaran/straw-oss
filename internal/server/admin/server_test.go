package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestServer_HealthCheck(t *testing.T) {
	// Setup
	cfg := config.ServerConfig{AdminPort: 8081}
	// Passing nil DB since health check is a GET request and won't trigger audit log
	// And code handles nil db by skipping audit middleware
	s := New(cfg, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "OK", rec.Body.String())
}

func TestServer_AuthProtection(t *testing.T) {
	cfg := config.ServerConfig{
		AdminPort:   8081,
		AdminAPIKey: "admin-secret",
	}
	s := New(cfg, nil, nil, nil, nil)

	// Attempt to access a protected route without auth
	// We request a path under /admin
	req := httptest.NewRequest(http.MethodGet, "/admin/some-resource", nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// With valid Bearer token
	req = httptest.NewRequest(http.MethodGet, "/admin/some-resource", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)

	// Since the route doesn't exist, it should return 404.
	// But it passed the Auth Middleware!
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
