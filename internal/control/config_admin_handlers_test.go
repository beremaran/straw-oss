package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	configTestPoolA = "pool_a"
)

func createRoutingRule(t *testing.T, ta *testAdmin, token, body string) (*httptest.ResponseRecorder, routingRuleResponse) {
	t.Helper()

	w := httptest.NewRecorder()
	ta.h.CreateRoutingRule(w, newAdminRequest(http.MethodPost, "/api/v1/config/routing-rules", token, body))

	var resp routingRuleResponse
	if w.Code == http.StatusOK {
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		if err != nil {
			t.Fatalf("unmarshal routing rule response: %v, body=%s", err, w.Body.String())
		}
	}

	return w, resp
}

func TestRoutingRuleCRUDAndRBAC(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_ra_admin", adminTestTenantA, RoleTenantAdmin)
	viewer := ta.seedTenantKey(t, "key_ra_viewer", adminTestTenantA, RoleViewer)

	// Viewer cannot create.
	w, _ := createRoutingRule(t, ta, viewer, `{"id":"route_crud_1","target_pool_id":"`+configTestPoolA+`"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer create status = %d, want 403", w.Code)
	}

	// tenant_admin creates with a client-supplied stable ID.
	w, created := createRoutingRule(t, ta, tenantAdmin, `{"id":"route_crud_1","priority":10,"target_pool_id":"`+configTestPoolA+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if created.ID != "route_crud_1" || created.ConfigVersion != 1 || !created.Enabled {
		t.Fatalf("created = %+v, want id=route_crud_1 version=1 enabled=true", created)
	}

	// Viewer can list/read.
	w = httptest.NewRecorder()
	ta.h.ListRoutingRules(w, newAdminRequest(http.MethodGet, "/api/v1/config/routing-rules", viewer, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", w.Code)
	}
	var listed []routingRuleResponse
	err := json.Unmarshal(w.Body.Bytes(), &listed)
	if err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "route_crud_1" {
		t.Fatalf("listed = %+v, want one route_crud_1", listed)
	}

	// Update with a stale expected_config_version conflicts and reports the
	// current version in details (docs/planning/26).
	w = httptest.NewRecorder()
	req := newAdminRequest(http.MethodPut, "/api/v1/config/routing-rules/route_crud_1", tenantAdmin,
		`{"priority":20,"target_pool_id":"`+configTestPoolA+`","expected_config_version":0}`)
	req.SetPathValue("id", "route_crud_1")
	ta.h.UpdateRoutingRule(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale update status = %d, want 409, body=%s", w.Code, w.Body.String())
	}
	var conflictResp ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &conflictResp)
	if err != nil {
		t.Fatalf("unmarshal conflict: %v", err)
	}
	if conflictResp.Details["current_config_version"] != "1" {
		t.Fatalf("conflict details = %+v, want current_config_version=1", conflictResp.Details)
	}

	// Update with the correct version succeeds.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPut, "/api/v1/config/routing-rules/route_crud_1", tenantAdmin,
		`{"priority":20,"target_pool_id":"`+configTestPoolA+`","expected_config_version":1}`)
	req.SetPathValue("id", "route_crud_1")
	ta.h.UpdateRoutingRule(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	// Delete soft-deletes: it drops from the list.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodDelete, "/api/v1/config/routing-rules/route_crud_1", tenantAdmin, "")
	req.SetPathValue("id", "route_crud_1")
	ta.h.DeleteRoutingRule(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", w.Code)
	}

	w = httptest.NewRecorder()
	ta.h.ListRoutingRules(w, newAdminRequest(http.MethodGet, "/api/v1/config/routing-rules", tenantAdmin, ""))
	var afterDelete []routingRuleResponse
	err = json.Unmarshal(w.Body.Bytes(), &afterDelete)
	if err != nil {
		t.Fatalf("unmarshal list after delete: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Fatalf("listed after delete = %+v, want none", afterDelete)
	}

	// Deleting again reports not found.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodDelete, "/api/v1/config/routing-rules/route_crud_1", tenantAdmin, "")
	req.SetPathValue("id", "route_crud_1")
	ta.h.DeleteRoutingRule(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", w.Code)
	}
}

func TestRoutingRulesTenantIsolation(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminA := ta.seedTenantKey(t, "key_iso_a", adminTestTenantA, RoleTenantAdmin)
	adminB := ta.seedTenantKey(t, "key_iso_b", adminTestTenantB, RoleTenantAdmin)

	_, created := createRoutingRule(t, ta, adminA, `{"id":"route_shared","target_pool_id":"`+configTestPoolA+`"}`)
	if created.ID != "route_shared" {
		t.Fatalf("create for tenant A failed: %+v", created)
	}

	// Tenant B's list must not see tenant A's rule.
	w := httptest.NewRecorder()
	ta.h.ListRoutingRules(w, newAdminRequest(http.MethodGet, "/api/v1/config/routing-rules", adminB, ""))
	var listed []routingRuleResponse
	err := json.Unmarshal(w.Body.Bytes(), &listed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("tenant B listed = %+v, want none (isolation)", listed)
	}
}

func TestRoutingRulesPaginationDefaults(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_page_admin", adminTestTenantA, RoleTenantAdmin)

	for i := range 3 {
		id := "route_" + string(rune('a'+i))
		_, created := createRoutingRule(t, ta, tenantAdmin, `{"id":"`+id+`","target_pool_id":"`+configTestPoolA+`"}`)
		if created.ID != id {
			t.Fatalf("create %s failed", id)
		}
	}

	w := httptest.NewRecorder()
	ta.h.ListRoutingRules(w, newAdminRequest(http.MethodGet, "/api/v1/config/routing-rules?limit=2&offset=0", tenantAdmin, ""))
	var page []routingRuleResponse
	err := json.Unmarshal(w.Body.Bytes(), &page)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("page len = %d, want 2 (limit applied)", len(page))
	}
}

func TestDenyRuleValidationAndRoleRestriction(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_deny_admin", adminTestTenantA, RoleTenantAdmin)
	operator := ta.seedTenantKey(t, "key_deny_operator", adminTestTenantA, RoleOperator)

	// Deny rules are tenant_admin-only writes (docs/planning/26): operator is
	// rejected outright.
	w := httptest.NewRecorder()
	ta.h.CreateDenyRule(w, newAdminRequest(http.MethodPost, "/api/v1/config/deny-rules", operator,
		`{"type":"host","value":"blocked.example","action":"deny"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("operator create status = %d, want 403", w.Code)
	}

	// Invalid CIDR is rejected as invalid_request.
	w = httptest.NewRecorder()
	ta.h.CreateDenyRule(w, newAdminRequest(http.MethodPost, "/api/v1/config/deny-rules", tenantAdmin,
		`{"type":"cidr","value":"not-a-cidr","action":"deny"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid cidr status = %d, want 400, body=%s", w.Code, w.Body.String())
	}

	// Valid host rule normalizes to lowercase and strips the trailing dot.
	w = httptest.NewRecorder()
	ta.h.CreateDenyRule(w, newAdminRequest(http.MethodPost, "/api/v1/config/deny-rules", tenantAdmin,
		`{"type":"host","value":"Blocked.Example.","action":"deny"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("valid host create status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var created denyRuleResponse
	err := json.Unmarshal(w.Body.Bytes(), &created)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.Value != "blocked.example" {
		t.Fatalf("normalized value = %q, want blocked.example", created.Value)
	}
}

func TestInjectionPolicySafetyRules(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_inj_admin", adminTestTenantA, RoleTenantAdmin)
	operator := ta.seedTenantKey(t, "key_inj_operator", adminTestTenantA, RoleOperator)

	// Host is always denied, for any role.
	w := httptest.NewRecorder()
	ta.h.CreateInjectionPolicy(w, newAdminRequest(http.MethodPost, "/api/v1/config/injection-policies", tenantAdmin,
		`{"operations":[{"op":"set","header_name":"Host","value_base64":"eA=="}]}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Host operation status = %d, want 400", w.Code)
	}

	// Operator cannot set Authorization (sensitive header).
	w = httptest.NewRecorder()
	ta.h.CreateInjectionPolicy(w, newAdminRequest(http.MethodPost, "/api/v1/config/injection-policies", operator,
		`{"operations":[{"op":"set","header_name":"Authorization","value_base64":"eA=="}]}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("operator Authorization status = %d, want 400", w.Code)
	}

	// tenant_admin may set Authorization.
	w = httptest.NewRecorder()
	ta.h.CreateInjectionPolicy(w, newAdminRequest(http.MethodPost, "/api/v1/config/injection-policies", tenantAdmin,
		`{"operations":[{"op":"set","header_name":"Authorization","value_base64":"eA=="}]}`))
	if w.Code != http.StatusOK {
		t.Fatalf("tenant_admin Authorization status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	// Operator may set a non-sensitive header.
	w = httptest.NewRecorder()
	ta.h.CreateInjectionPolicy(w, newAdminRequest(http.MethodPost, "/api/v1/config/injection-policies", operator,
		`{"operations":[{"op":"set","header_name":"X-Custom","value_base64":"eA=="}]}`))
	if w.Code != http.StatusOK {
		t.Fatalf("operator non-sensitive status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestFingerprintProfilesReadOnly(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	viewer := ta.seedTenantKey(t, "key_fp_viewer", adminTestTenantA, RoleViewer)

	w := httptest.NewRecorder()
	ta.h.ListFingerprintProfiles(w, newAdminRequest(http.MethodGet, "/api/v1/config/fingerprint-profiles", viewer, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", w.Code)
	}

	var profiles []fingerprintProfileResponse
	err := json.Unmarshal(w.Body.Bytes(), &profiles)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(profiles) == 0 {
		t.Fatal("profiles empty, want seeded built-ins")
	}

	found := false
	for _, p := range profiles {
		if p.Name == defaultFingerprintProfileName && p.ScopeType == fingerprintProfileScopeGlobal {
			found = true
		}
	}
	if !found {
		t.Fatalf("profiles = %+v, want built-in default global profile", profiles)
	}

	// No handler exists for a write path in P0: AdminHandlers exposes no
	// Create/Update/Delete fingerprint-profile method to route to.
}

func TestConfigWritesPublishInvalidation(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_pub_admin", adminTestTenantA, RoleTenantAdmin)

	published := &recordingInvalidationPublisher{}
	ta.h.ConfigCache = NewConfigCache(NewInMemorySnapshotStore(), published)

	_, created := createRoutingRule(t, ta, tenantAdmin, `{"id":"route_pub","target_pool_id":"`+configTestPoolA+`"}`)
	if created.ID != "route_pub" {
		t.Fatalf("create failed: %+v", created)
	}

	if !published.called {
		t.Fatal("PublishTenantInvalidation was not called after a routing rule write")
	}
	if published.tenantID != adminTestTenantA {
		t.Fatalf("published tenant = %q, want %q", published.tenantID, adminTestTenantA)
	}
}

type recordingInvalidationPublisher struct {
	called   bool
	tenantID string
	version  uint64
}

func (p *recordingInvalidationPublisher) PublishTenantInvalidation(_ context.Context, tenantID string, version uint64) error {
	p.called = true
	p.tenantID = tenantID
	p.version = version

	return nil
}
