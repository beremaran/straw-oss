package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server/admin/middleware"
)

func TestServer_HealthCheck(t *testing.T) {
	cfg := config.ServerConfig{ManagementPort: 8081}
	s := New(cfg, nil, nil, nil, nil, nil)

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
	s := New(cfg, nil, nil, nil, nil, nil)

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

func TestManagementGlobalMiddleware_MissingPermission(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /management/protected", middleware.RequirePermission(middleware.PermissionAPIKeysRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	h := managementGlobalMiddleware(config.ServerConfig{}, nil)(mux)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/protected", nil)
	req = req.WithContext(middleware.ContextWithActor(req.Context(), middleware.Actor{
		Type:        middleware.ActorTypeUser,
		ID:          "user-1",
		Permissions: []string{middleware.PermissionUsageRead},
	}))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
