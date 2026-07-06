package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw/v2/internal/control"
)

const routeConfigTenants = "/api/v1/config/tenants"

// TestServeAdminRoutesCanonicalConfigPaths proves the identity and limits
// config endpoints are registered only under the canonical /api/v1/config
// base path (docs/planning/26), not the old bare root paths
// (docs/tasks/p0/36-canonical-config-base-path.md).
func TestServeAdminRoutesCanonicalConfigPaths(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	serveAdminRoutes(mux, &control.AdminHandlers{})

	canonical := []struct{ method, path string }{
		{http.MethodPost, routeConfigTenants},
		{http.MethodGet, routeConfigTenants},
		{http.MethodPost, "/api/v1/config/platform-api-keys"},
		{http.MethodGet, "/api/v1/config/platform-api-keys"},
		{http.MethodPost, "/api/v1/config/platform-api-keys/key_1/revoke"},
		{http.MethodPost, "/api/v1/config/api-keys"},
		{http.MethodGet, "/api/v1/config/api-keys"},
		{http.MethodPost, "/api/v1/config/api-keys/key_1/revoke"},
		{http.MethodPost, "/api/v1/config/worker-credentials"},
		{http.MethodGet, "/api/v1/config/worker-credentials"},
		{http.MethodPost, "/api/v1/config/worker-credentials/wcred_1/revoke"},
		{http.MethodGet, "/api/v1/config/quotas"},
		{http.MethodPut, "/api/v1/config/tenants/ten_1/quotas"},
		{http.MethodGet, "/api/v1/config/rate-limits"},
		{http.MethodPut, "/api/v1/config/rate-limits"},
		{http.MethodPost, "/api/v1/config/rollback"},
	}
	for _, tc := range canonical {
		req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil)
		if _, pattern := mux.Handler(req); pattern == "" {
			t.Errorf("%s %s: no route matched, want canonical route registered", tc.method, tc.path)
		}
	}

	bare := []struct{ method, path string }{
		{http.MethodPost, "/tenants"},
		{http.MethodGet, "/tenants"},
		{http.MethodPost, "/platform-api-keys"},
		{http.MethodGet, "/platform-api-keys"},
		{http.MethodPost, "/platform-api-keys/key_1/revoke"},
		{http.MethodPost, "/api-keys"},
		{http.MethodGet, "/api-keys"},
		{http.MethodPost, "/api-keys/key_1/revoke"},
		{http.MethodPost, "/worker-credentials"},
		{http.MethodGet, "/worker-credentials"},
		{http.MethodPost, "/worker-credentials/wcred_1/revoke"},
		{http.MethodGet, "/quotas"},
		{http.MethodPut, "/tenants/ten_1/quotas"},
		{http.MethodGet, "/rate-limits"},
		{http.MethodPut, "/rate-limits"},
		{http.MethodPost, "/rollback"},
	}
	for _, tc := range bare {
		req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404 for removed bare path", tc.method, tc.path, rec.Code)
		}
	}
}
