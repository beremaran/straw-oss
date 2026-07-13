package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminHandlerRequiresDedicatedBearerAndIfMatch(t *testing.T) {
	t.Parallel()
	admin := newTestAdmin(t).admin
	auth, err := NewAdminAuthenticator("admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewAdminHandler(admin, auth).Register(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/config", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized code = %d", unauthorized.Code)
	}

	current, _ := admin.Current()
	raw, err := json.Marshal(current.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	missingPrecondition := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/admin/config", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer admin-secret")
	mux.ServeHTTP(missingPrecondition, req)
	if missingPrecondition.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match code = %d", missingPrecondition.Code)
	}
}

func TestDashboardContainsAllAdministrativeActions(t *testing.T) {
	t.Parallel()
	admin := newTestAdmin(t).admin
	auth, err := NewAdminAuthenticator("secret")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewAdminHandler(admin, auth).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/", nil))
	for _, action := range []string{"saveConfig", "drain", "undrain", "disable", "enable", "rollback", "cancelReq"} {
		if !bytes.Contains(rec.Body.Bytes(), []byte(action)) {
			t.Fatalf("dashboard missing action %q", action)
		}
	}
}

func TestAdminPutInvalidSnapshotReturns422AndDoesNotActivate(t *testing.T) {
	t.Parallel()
	fixture := newTestAdmin(t)
	auth, err := NewAdminAuthenticator("admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewAdminHandler(fixture.admin, auth).Register(mux)
	current, _ := fixture.admin.Current()
	invalid := current.Snapshot.Clone()
	invalid.RoutingRules[0].AllowStickyFallback = true
	raw, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/admin/config", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("If-Match", "1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("PUT code = %d, body = %s; want 422", rec.Code, rec.Body.String())
	}
	got, _ := fixture.admin.Current()
	if got.Revision != current.Revision || fixture.admin.cache.Snapshot().ConfigVersion != current.Snapshot.ConfigVersion {
		t.Fatal("invalid PUT changed durable or active configuration")
	}
}
