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

const (
	testUserDetailPath           = "/management/users/user-1"
	testAPIKeyDetailPath         = "/management/api-keys/key-1"
	testRuleDetailPath           = "/management/rules/rule-1"
	testEndpointDetailPath       = "/management/endpoints/endpoint-1"
	testCostMultiplierDetailPath = "/management/cost-multipliers/multiplier-1"
	testReportDetailPath         = "/management/reports/report-1"
	testAlertRuleDetailPath      = "/management/alerts/rules/rule-1"
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

func TestServer_ManagementRoutesRequireAuth(t *testing.T) {
	cfg := config.ServerConfig{
		ManagementPort:   8081,
		ManagementAPIKey: "management-secret",
	}
	s := New(cfg, nil, nil, nil, nil, nil)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/management/users"},
		{http.MethodPost, "/management/users"},
		{http.MethodGet, testUserDetailPath},
		{http.MethodPatch, testUserDetailPath},
		{http.MethodDelete, testUserDetailPath},
		{http.MethodGet, "/management/roles"},
		{http.MethodPost, "/management/roles"},
		{http.MethodPatch, "/management/roles/role-1"},
		{http.MethodDelete, "/management/roles/role-1"},
		{http.MethodGet, "/management/identity-providers"},
		{http.MethodPost, "/management/identity-providers"},
		{http.MethodPatch, "/management/identity-providers/provider-1"},
		{http.MethodDelete, "/management/identity-providers/provider-1"},
		{http.MethodGet, "/management/api-keys"},
		{http.MethodPost, "/management/api-keys"},
		{http.MethodGet, testAPIKeyDetailPath},
		{http.MethodPatch, testAPIKeyDetailPath},
		{http.MethodPost, "/management/api-keys/key-1/rotate"},
		{http.MethodPost, "/management/api-keys/key-1/reactivate"},
		{http.MethodPost, "/management/api-keys/key-1/revoke"},
		{http.MethodDelete, testAPIKeyDetailPath},
		{http.MethodGet, "/management/rules"},
		{http.MethodPost, "/management/rules"},
		{http.MethodGet, testRuleDetailPath},
		{http.MethodPut, testRuleDetailPath},
		{http.MethodDelete, testRuleDetailPath},
		{http.MethodGet, "/management/endpoints"},
		{http.MethodPost, "/management/endpoints"},
		{http.MethodGet, testEndpointDetailPath},
		{http.MethodPatch, testEndpointDetailPath},
		{http.MethodDelete, testEndpointDetailPath},
		{http.MethodPost, "/management/endpoints/endpoint-1/drain"},
		{http.MethodPost, "/management/endpoints/endpoint-1/undrain"},
		{http.MethodPost, "/management/endpoints/endpoint-1/restart"},
		{http.MethodGet, "/management/endpoints/endpoint-1/logs"},
		{http.MethodGet, "/management/endpoints/endpoint-1/logs/stream"},
		{http.MethodGet, "/management/endpoints/endpoint-1/commands"},
		{http.MethodGet, "/management/commands/command-1"},
		{http.MethodGet, "/management/fingerprints"},
		{http.MethodPost, "/management/fingerprints"},
		{http.MethodGet, "/management/fingerprints/preset-1"},
		{http.MethodDelete, "/management/fingerprints/preset-1"},
		{http.MethodPost, "/management/fingerprints/broadcast"},
		{http.MethodGet, "/management/usage/summary"},
		{http.MethodGet, "/management/billing/estimate"},
		{http.MethodGet, "/management/cost-multipliers"},
		{http.MethodPost, "/management/cost-multipliers"},
		{http.MethodGet, testCostMultiplierDetailPath},
		{http.MethodPut, testCostMultiplierDetailPath},
		{http.MethodDelete, testCostMultiplierDetailPath},
		{http.MethodGet, "/management/reports"},
		{http.MethodPost, "/management/reports"},
		{http.MethodGet, testReportDetailPath},
		{http.MethodPatch, testReportDetailPath},
		{http.MethodDelete, testReportDetailPath},
		{http.MethodPost, "/management/reports/report-1/run"},
		{http.MethodGet, "/management/reports/report-1/runs"},
		{http.MethodGet, "/management/report-runs/run-1"},
		{http.MethodGet, "/management/report-runs/run-1/download"},
		{http.MethodGet, "/management/report-schedules"},
		{http.MethodPost, "/management/report-schedules"},
		{http.MethodPatch, "/management/report-schedules/schedule-1"},
		{http.MethodDelete, "/management/report-schedules/schedule-1"},
		{http.MethodGet, "/management/notification-channels"},
		{http.MethodPost, "/management/notification-channels"},
		{http.MethodPatch, "/management/notification-channels/channel-1"},
		{http.MethodDelete, "/management/notification-channels/channel-1"},
		{http.MethodPost, "/management/notification-channels/channel-1/test"},
		{http.MethodGet, "/management/notification-preferences"},
		{http.MethodPatch, "/management/notification-preferences"},
		{http.MethodGet, "/management/alerts/rules"},
		{http.MethodPost, "/management/alerts/rules"},
		{http.MethodGet, testAlertRuleDetailPath},
		{http.MethodPatch, testAlertRuleDetailPath},
		{http.MethodDelete, testAlertRuleDetailPath},
		{http.MethodGet, "/management/alerts/events"},
		{http.MethodPost, "/management/alerts/events/event-1/ack"},
		{http.MethodPost, "/management/cache/clear"},
		{http.MethodGet, "/management/cache/stats"},
		{http.MethodGet, "/management/audit/events"},
		{http.MethodGet, "/management/audit/events/1"},
		{http.MethodGet, "/management/audit/requests"},
		{http.MethodGet, "/management/audit/export"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), route.method, route.path, nil)
			rec := httptest.NewRecorder()

			s.GetHandler().ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}
