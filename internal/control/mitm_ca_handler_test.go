package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

const mitmCATestTenant = "ten_mitm"

func TestMITMCAHandlerServesPublicCertificateOnly(t *testing.T) {
	t.Parallel()

	certFile := writeMITMCATestFile(t, "public-ca")
	keyFile := writeMITMCATestFile(t, "private-key")
	h, token := newTestMITMCAHandler(t, certFile, RoleRequester)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/mitm/ca.pem", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get(headerCanonicalContentType); got != mediaTypePEM {
		t.Fatalf("Content-Type = %q, want %q", got, mediaTypePEM)
	}
	if got := w.Body.String(); got != "public-ca" {
		t.Fatalf("body = %q, want public cert", got)
	}
	if strings.Contains(w.Body.String(), "private-key") {
		t.Fatal("response exposed private key material")
	}

	_, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("private key fixture disappeared: %v", err)
	}
}

func TestMITMCAHandlerRejectsUnauthorizedRoles(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		role Role
		want int
	}{
		{name: "viewer", role: RoleViewer, want: http.StatusForbidden},
		{name: "operator", role: RoleOperator, want: http.StatusForbidden},
		{name: "tenant_admin", role: RoleTenantAdmin, want: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, token := newTestMITMCAHandler(t, writeMITMCATestFile(t, "public-ca"), tt.role)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/mitm/ca.pem", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestMITMCAHandlerRequiresTenantAuth(t *testing.T) {
	t.Parallel()

	h, _ := newTestMITMCAHandler(t, writeMITMCATestFile(t, "public-ca"), RoleRequester)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/mitm/ca.pem", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestMITMCAHandlerRejectsInactiveTenant(t *testing.T) {
	t.Parallel()

	tenants := NewInMemoryTenantStore()
	err := tenants.Create(context.Background(), Tenant{ID: mitmCATestTenant, Status: TenantStatusSuspended})
	if err != nil {
		t.Fatalf("tenants.Create() error = %v", err)
	}

	h, token := newTestMITMCAHandler(t, writeMITMCATestFile(t, "public-ca"), RoleRequester)
	h.Authenticator.SetTenantStore(tenants)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/mitm/ca.pem", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestMITMCAHandlerRequiresMITMAllowedRoute(t *testing.T) {
	t.Parallel()

	h, token := newTestMITMCAHandler(t, writeMITMCATestFile(t, "public-ca"), RoleRequester)
	h.ConfigCache = newMITMCATestConfigCache(t, config.RoutingRule{
		ID:           "route_rest",
		Enabled:      true,
		TargetPoolID: routingTestPool1,
		Match:        config.MatchConditions{IngressType: IngressTypeREST},
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/mitm/ca.pem", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func newTestMITMCAHandler(t *testing.T, certFile string, role Role) (*MITMCAHandler, string) {
	t.Helper()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("test-pepper")
	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}

	err = store.Create(context.Background(), APIKeyRecord{
		ID:         "key_mitm_ca_" + string(role),
		ScopeType:  ScopeTenant,
		TenantID:   mitmCATestTenant,
		Role:       role,
		Prefix:     generated.Prefix,
		SecretHash: HashAPIKeySecret(generated.Secret, pepper),
		Status:     APIKeyStatusActive,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("store.Create() error = %v", err)
	}

	return &MITMCAHandler{
		Authenticator: NewAuthenticator(store, pepper),
		ConfigCache:   newMITMCATestConfigCache(t, config.RoutingRule{ID: "route_mitm", Enabled: true, TargetPoolID: routingTestPool1, Match: config.MatchConditions{IngressType: IngressTypeMITM}}),
		CertFile:      certFile,
	}, generated.Secret
}

func newMITMCATestConfigCache(t *testing.T, rule config.RoutingRule) *ConfigCache {
	t.Helper()

	store := NewInMemorySnapshotStore()
	_, err := store.SaveTenantSnapshot(context.Background(), config.TenantSnapshot{
		TenantID:      mitmCATestTenant,
		ConfigVersion: 1,
		RoutingRules:  []config.RoutingRule{rule},
	}, 0)
	if err != nil {
		t.Fatalf("SaveTenantSnapshot() error = %v", err)
	}

	return NewConfigCache(store, nil)
}

func writeMITMCATestFile(t *testing.T, contents string) string {
	t.Helper()

	path := t.TempDir() + "/ca.pem"
	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	return path
}
