package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCancelRequestSystemAdminCancelsAnyRequest(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	ta.h.InFlight = NewInFlightRegistry()

	called := false
	ta.h.InFlight.Register(context.Background(), "req_1", adminTestTenantA, func() { called = true })

	token := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/requests/req_1/cancel", token, "")
	req.SetPathValue("request_id", "req_1")
	ta.h.CancelRequest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	if !called {
		t.Fatal("system_admin cancel did not invoke the cancel func")
	}
}

func TestCancelRequestTenantAdminAndOperatorCancelOwnTenant(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RoleTenantAdmin, RoleOperator} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			ta := newTestAdmin(t)
			ta.h.InFlight = NewInFlightRegistry()

			called := false
			ta.h.InFlight.Register(context.Background(), "req_1", adminTestTenantA, func() { called = true })

			token := ta.seedTenantKey(t, "key_"+string(role), adminTestTenantA, role)
			w := httptest.NewRecorder()
			req := newAdminRequest(http.MethodPost, "/api/v1/admin/requests/req_1/cancel", token, "")
			req.SetPathValue("request_id", "req_1")
			ta.h.CancelRequest(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
			}

			if !called {
				t.Fatalf("%s cancel did not invoke the cancel func", role)
			}
		})
	}
}

func TestCancelRequestForeignTenantInsufficientPermissionsNoDisclosure(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	ta.h.InFlight = NewInFlightRegistry()

	called := false
	ta.h.InFlight.Register(context.Background(), "req_1", adminTestTenantB, func() { called = true })

	token := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/requests/req_1/cancel", token, "")
	req.SetPathValue("request_id", "req_1")
	ta.h.CancelRequest(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", w.Code, w.Body.String())
	}

	var resp ErrorResponse

	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Code != adminTestInsufficientPerms {
		t.Fatalf("code = %q, want %q", resp.Code, adminTestInsufficientPerms)
	}

	if called {
		t.Fatal("foreign-tenant cancel invoked the cancel func")
	}
}

func TestCancelRequestUnknownRequestTenantScopeSameAsForeign(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	ta.h.InFlight = NewInFlightRegistry()

	token := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/requests/req_missing/cancel", token, "")
	req.SetPathValue("request_id", "req_missing")
	ta.h.CancelRequest(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (same as foreign-tenant), body=%s", w.Code, w.Body.String())
	}

	var resp ErrorResponse

	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Code != adminTestInsufficientPerms {
		t.Fatalf("code = %q, want %q", resp.Code, adminTestInsufficientPerms)
	}
}

func TestCancelRequestViewerForbidden(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	ta.h.InFlight = NewInFlightRegistry()
	ta.h.InFlight.Register(context.Background(), "req_1", adminTestTenantA, func() {})

	token := ta.seedTenantKey(t, "key_viewer", adminTestTenantA, RoleViewer)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/requests/req_1/cancel", token, "")
	req.SetPathValue("request_id", "req_1")
	ta.h.CancelRequest(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for viewer role, body=%s", w.Code, w.Body.String())
	}
}

func TestCancelRequestNoInFlightRegistryWiredIsControlInternalError(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)

	token := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/requests/req_1/cancel", token, "")
	req.SetPathValue("request_id", "req_1")
	ta.h.CancelRequest(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when InFlight unwired, body=%s", w.Code, w.Body.String())
	}
}
