package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

const mitmCATestTenant = "ten_mitm"

const (
	mitmCATestCertPEMField = "cert_pem"
	mitmCATestKeyPEMField  = "key_pem"
)

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

func TestMITMCAHandlerRotatesCAForTenantAdmin(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := newMITMCATestPEM(t)
	certFile := writeMITMCATestFile(t, "old-cert")
	keyFile := writeMITMCATestFile(t, "old-key")
	h, token := newTestMITMCAHandler(t, certFile, RoleTenantAdmin)
	h.KeyFile = keyFile
	h.Audit = NewInMemoryAuditStore()

	body := map[string]string{mitmCATestCertPEMField: string(certPEM), mitmCATestKeyPEMField: string(keyPEM)}
	req := newMITMCARotateRequest(t, token, body)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	getReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/mitm/ca.pem", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getResp := httptest.NewRecorder()
	h.ServeHTTP(getResp, getReq)
	if got := getResp.Body.String(); got != string(certPEM) {
		t.Fatalf("downloaded cert = %q, want rotated cert", got)
	}

	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if keyInfo.Size() != int64(len(keyPEM)) {
		t.Fatalf("key file size = %d, want %d", keyInfo.Size(), len(keyPEM))
	}
	if strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatal("response exposed private key material")
	}

	records, err := h.Audit.ListTenant(context.Background(), mitmCATestTenant)
	if err != nil {
		t.Fatalf("ListTenant() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("audit records = %d, want 1", len(records))
	}
	if strings.Contains(records[0].NewValueJSON, "PRIVATE KEY") {
		t.Fatal("audit row exposed private key material")
	}
}

func TestMITMCAHandlerRotateRejectsNonAdmins(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := newMITMCATestPEM(t)
	for _, role := range []Role{RoleRequester, RoleViewer, RoleOperator} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			h, token := newTestMITMCAHandler(t, writeMITMCATestFile(t, "old-cert"), role)
			h.KeyFile = writeMITMCATestFile(t, "old-key")
			req := newMITMCARotateRequest(t, token, map[string]string{mitmCATestCertPEMField: string(certPEM), mitmCATestKeyPEMField: string(keyPEM)})
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
			}
		})
	}
}

func TestMITMCAHandlerRotateRejectsPlatformKey(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("test-pepper")
	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	err = store.Create(context.Background(), APIKeyRecord{
		ID:         "key_mitm_ca_platform",
		ScopeType:  ScopePlatform,
		Role:       RoleSystemAdmin,
		Prefix:     generated.Prefix,
		SecretHash: HashAPIKeySecret(generated.Secret, pepper),
		Status:     APIKeyStatusActive,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("store.Create() error = %v", err)
	}

	certPEM, keyPEM := newMITMCATestPEM(t)
	h := &MITMCAHandler{
		Authenticator: NewAuthenticator(store, pepper),
		CertFile:      writeMITMCATestFile(t, "old-cert"),
		KeyFile:       writeMITMCATestFile(t, "old-key"),
	}
	req := newMITMCARotateRequest(t, generated.Secret, map[string]string{mitmCATestCertPEMField: string(certPEM), mitmCATestKeyPEMField: string(keyPEM)})
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestMITMCAHandlerRotateRejectsInvalidMaterial(t *testing.T) {
	t.Parallel()

	h, token := newTestMITMCAHandler(t, writeMITMCATestFile(t, "old-cert"), RoleTenantAdmin)
	h.KeyFile = writeMITMCATestFile(t, "old-key")
	req := newMITMCARotateRequest(t, token, map[string]string{mitmCATestCertPEMField: "not pem", mitmCATestKeyPEMField: "secret private-key"})
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private-key") {
		t.Fatal("error response exposed submitted key material")
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

func newMITMCARotateRequest(t *testing.T, token string, body map[string]string) *http.Request {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/mitm/ca", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)

	return req
}

func newMITMCATestPEM(t *testing.T) ([]byte, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Straw Test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}
