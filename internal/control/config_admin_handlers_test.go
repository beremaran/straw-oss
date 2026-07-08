package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	configTestPoolA  = "pool_a"
	testCNAMESuffix  = "internal.svc.cluster.local"
	testPrivateRange = "172.16.0.0/12"
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

func TestRoutingRuleRejectsUnknownIngressType(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_route_ingress_admin", adminTestTenantA, RoleTenantAdmin)

	w, _ := createRoutingRule(t, ta, tenantAdmin, `{"id":"route_bad_ingress","target_pool_id":"`+configTestPoolA+`","match_conditions":{"ingress_type":"ftp"}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400, body=%s", w.Code, w.Body.String())
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

func createExecutorPool(t *testing.T, ta *testAdmin, token, body string) (*httptest.ResponseRecorder, executorPoolResponse) {
	t.Helper()

	w := httptest.NewRecorder()
	ta.h.CreateExecutorPool(w, newAdminRequest(http.MethodPost, "/api/v1/config/executor-pools", token, body))

	var resp executorPoolResponse
	if w.Code == http.StatusOK {
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		if err != nil {
			t.Fatalf("unmarshal executor pool response: %v, body=%s", err, w.Body.String())
		}
	}

	return w, resp
}

func TestExecutorPoolCRUDAndRBAC(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_pool_admin", adminTestTenantA, RoleTenantAdmin)
	operator := ta.seedTenantKey(t, "key_pool_operator", adminTestTenantA, RoleOperator)
	viewer := ta.seedTenantKey(t, "key_pool_viewer", adminTestTenantA, RoleViewer)

	// Operator cannot create (docs/planning/26: executor-pool writes are
	// tenant_admin-only).
	w, _ := createExecutorPool(t, ta, operator, `{"id":"pool_crud_1"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("operator create status = %d, want 403", w.Code)
	}

	// An invalid allowed_ip_types value is rejected.
	w, _ = createExecutorPool(t, ta, tenantAdmin, `{"id":"pool_crud_bad_iptype","allowed_ip_types":["not_a_type"]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid ip type create status = %d, want 400, body=%s", w.Code, w.Body.String())
	}

	// tenant_admin creates with a client-supplied stable ID and the P0
	// capability restriction fields (docs/planning/26, docs/tasks/p0/42).
	w, created := createExecutorPool(t, ta, tenantAdmin,
		`{"id":"pool_crud_1","allow_degraded_workers":true,`+
			`"allowed_ip_types":["`+ipTypeDatacenter+`"],"allowed_countries":["US"],"allowed_regions":["us-west-1"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if created.ID != "pool_crud_1" || created.ConfigVersion != 1 || !created.Enabled || !created.AllowDegradedWorkers {
		t.Fatalf("created = %+v, want id=pool_crud_1 version=1 enabled=true allow_degraded_workers=true", created)
	}
	if len(created.AllowedIPTypes) != 1 || created.AllowedIPTypes[0] != ipTypeDatacenter ||
		len(created.AllowedCountries) != 1 || created.AllowedCountries[0] != "US" ||
		len(created.AllowedRegions) != 1 || created.AllowedRegions[0] != "us-west-1" {
		t.Fatalf("created capability fields = %+v, want datacenter/US/us-west-1", created)
	}

	// Viewer and operator can list/read.
	w = httptest.NewRecorder()
	ta.h.ListExecutorPools(w, newAdminRequest(http.MethodGet, "/api/v1/config/executor-pools", viewer, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", w.Code)
	}
	var listed []executorPoolResponse
	err := json.Unmarshal(w.Body.Bytes(), &listed)
	if err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "pool_crud_1" {
		t.Fatalf("listed = %+v, want one pool_crud_1", listed)
	}

	// Update with a stale expected_config_version conflicts and reports the
	// current version in details.
	w = httptest.NewRecorder()
	req := newAdminRequest(http.MethodPut, "/api/v1/config/executor-pools/pool_crud_1", tenantAdmin,
		`{"expected_config_version":0}`)
	req.SetPathValue("id", "pool_crud_1")
	ta.h.UpdateExecutorPool(w, req)
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

	// Update with the correct version succeeds and can flip
	// allow_degraded_workers off.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPut, "/api/v1/config/executor-pools/pool_crud_1", tenantAdmin,
		`{"expected_config_version":1,"allow_degraded_workers":false}`)
	req.SetPathValue("id", "pool_crud_1")
	ta.h.UpdateExecutorPool(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var updated executorPoolResponse
	err = json.Unmarshal(w.Body.Bytes(), &updated)
	if err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if updated.AllowDegradedWorkers {
		t.Fatalf("updated = %+v, want allow_degraded_workers=false", updated)
	}

	// Delete soft-deletes: it drops from the list.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodDelete, "/api/v1/config/executor-pools/pool_crud_1", tenantAdmin, "")
	req.SetPathValue("id", "pool_crud_1")
	ta.h.DeleteExecutorPool(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", w.Code)
	}

	w = httptest.NewRecorder()
	ta.h.ListExecutorPools(w, newAdminRequest(http.MethodGet, "/api/v1/config/executor-pools", tenantAdmin, ""))
	var afterDelete []executorPoolResponse
	err = json.Unmarshal(w.Body.Bytes(), &afterDelete)
	if err != nil {
		t.Fatalf("unmarshal list after delete: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Fatalf("listed after delete = %+v, want none", afterDelete)
	}

	// Deleting again reports not found.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodDelete, "/api/v1/config/executor-pools/pool_crud_1", tenantAdmin, "")
	req.SetPathValue("id", "pool_crud_1")
	ta.h.DeleteExecutorPool(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", w.Code)
	}
}

func TestExecutorPoolsTenantIsolation(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminA := ta.seedTenantKey(t, "key_pool_iso_a", adminTestTenantA, RoleTenantAdmin)
	adminB := ta.seedTenantKey(t, "key_pool_iso_b", adminTestTenantB, RoleTenantAdmin)

	_, created := createExecutorPool(t, ta, adminA, `{"id":"pool_shared"}`)
	if created.ID != "pool_shared" {
		t.Fatalf("create for tenant A failed: %+v", created)
	}

	w := httptest.NewRecorder()
	ta.h.ListExecutorPools(w, newAdminRequest(http.MethodGet, "/api/v1/config/executor-pools", adminB, ""))
	var listed []executorPoolResponse
	err := json.Unmarshal(w.Body.Bytes(), &listed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("tenant B listed = %+v, want none (isolation)", listed)
	}
}

func TestExecutorPoolsPaginationDefaults(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_pool_page_admin", adminTestTenantA, RoleTenantAdmin)

	for i := range 3 {
		id := "pool_" + string(rune('a'+i))
		_, created := createExecutorPool(t, ta, tenantAdmin, `{"id":"`+id+`"}`)
		if created.ID != id {
			t.Fatalf("create %s failed", id)
		}
	}

	w := httptest.NewRecorder()
	ta.h.ListExecutorPools(w, newAdminRequest(http.MethodGet, "/api/v1/config/executor-pools?limit=2&offset=0", tenantAdmin, ""))
	var page []executorPoolResponse
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

// TestDenyRuleLeadingDotNormalization is docs/tasks/p0/48's fix: a
// host/host_suffix value written with a leading dot (".evil.example") must
// enforce exactly like the dotless form instead of being stored verbatim and
// silently never matching, and a value that normalizes to nothing (".") must
// be rejected rather than stored as an inert empty host.
func TestDenyRuleLeadingDotNormalization(t *testing.T) {
	t.Parallel()

	withDot, err := normalizeDenyRule(denyRuleTypeHostSuffix, ".evil.example", "")
	if err != nil {
		t.Fatalf("normalize leading-dot host_suffix: %v", err)
	}

	withoutDot, err := normalizeDenyRule(denyRuleTypeHostSuffix, "evil.example", "")
	if err != nil {
		t.Fatalf("normalize dotless host_suffix: %v", err)
	}

	if withDot.NormalizedHost != "evil.example" {
		t.Fatalf("normalized host = %q, want evil.example (leading dot stripped)", withDot.NormalizedHost)
	}

	if withDot.NormalizedHost != withoutDot.NormalizedHost {
		t.Fatalf("leading-dot form normalized to %q, dotless to %q, want identical", withDot.NormalizedHost, withoutDot.NormalizedHost)
	}

	for _, host := range []string{"evil.example", "x.evil.example"} {
		if !hostMatchesDenyRule(host, withDot) {
			t.Fatalf("host %q not matched by host_suffix rule created as .evil.example", host)
		}
	}

	hostRule, err := normalizeDenyRule(denyRuleTypeHost, ".evil.example", "")
	if err != nil {
		t.Fatalf("normalize leading-dot host: %v", err)
	}

	if !hostMatchesDenyRule("evil.example", hostRule) {
		t.Fatal("exact host not matched by host rule created as .evil.example")
	}

	// Dot-only values normalize to "" and would never match any host: reject.
	for _, ruleType := range []string{denyRuleTypeHost, denyRuleTypeHostSuffix, denyRuleTypeCNAMESuffix} {
		for _, value := range []string{".", "..", " . "} {
			_, err := normalizeDenyRule(ruleType, value, "")
			if !errors.Is(err, errInvalidDenyRuleValue) {
				t.Fatalf("normalizeDenyRule(%s, %q) err = %v, want errInvalidDenyRuleValue", ruleType, value, err)
			}
		}
	}
}

// TestDenyRuleTaxonomyCRUD is docs/tasks/p0/43's CRUD-round-trip-per-type
// coverage: every docs/planning/26 P0 type/action accepted with a reason, and
// the old narrower cname/ip/allow values rejected with a clear error.
func TestDenyRuleTaxonomyCRUD(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_deny_taxonomy", adminTestTenantA, RoleTenantAdmin)

	cases := []struct {
		name  string
		body  string
		value string
	}{
		{name: denyRuleTypeCIDR, body: `{"type":"cidr","value":"10.1.0.0/16","action":"deny","reason":"tenant private range"}`, value: "10.1.0.0/16"},
		{name: "host", body: `{"type":"host","value":"blocked.example.com","action":"deny","reason":"exact host"}`, value: "blocked.example.com"},
		{name: "host_suffix", body: `{"type":"host_suffix","value":"blocked.example.org","action":"deny","reason":"whole domain"}`, value: "blocked.example.org"},
		{name: "cname_suffix", body: `{"type":"cname_suffix","value":"` + testCNAMESuffix + `","action":"deny","reason":"internal cname"}`, value: testCNAMESuffix},
		{name: "metadata_ip", body: `{"type":"metadata_ip","value":"169.254.169.254/32","action":"deny","reason":"cloud metadata"}`, value: "169.254.169.254/32"},
		{name: "private_range", body: `{"type":"private_range","value":"` + testPrivateRange + `","action":"allow_override","reason":"tenant needs internal reach"}`, value: testPrivateRange},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ta.h.CreateDenyRule(w, newAdminRequest(http.MethodPost, "/api/v1/config/deny-rules", tenantAdmin, tc.body))

			if w.Code != http.StatusOK {
				t.Fatalf("create status = %d, want 200, body=%s", w.Code, w.Body.String())
			}

			var created denyRuleResponse

			err := json.Unmarshal(w.Body.Bytes(), &created)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if created.Value != tc.value {
				t.Fatalf("value = %q, want %q", created.Value, tc.value)
			}

			if created.Reason == "" {
				t.Fatal("reason round-trip is empty, want the submitted reason")
			}

			w = httptest.NewRecorder()
			ta.h.ListDenyRules(w, newAdminRequest(http.MethodGet, "/api/v1/config/deny-rules", tenantAdmin, ""))

			var page []denyRuleResponse

			err = json.Unmarshal(w.Body.Bytes(), &page)
			if err != nil {
				t.Fatalf("unmarshal list: %v", err)
			}

			found := false

			for _, r := range page {
				if r.ID == created.ID {
					found = true

					if r.Type != tc.name || r.Value != tc.value || r.Reason != created.Reason {
						t.Fatalf("listed rule = %+v, want type=%s value=%s reason=%s", r, tc.name, tc.value, created.Reason)
					}
				}
			}

			if !found {
				t.Fatalf("created rule %s not found in list", created.ID)
			}
		})
	}

	// The old narrower taxonomy values are rejected with a clear error.
	rejected := []string{
		`{"type":"cname","value":"internal.svc.cluster.local","action":"deny"}`,
		`{"type":"ip","value":"169.254.169.254","action":"deny"}`,
		`{"type":"host","value":"blocked.example.com","action":"allow"}`,
	}

	for _, body := range rejected {
		w := httptest.NewRecorder()
		ta.h.CreateDenyRule(w, newAdminRequest(http.MethodPost, "/api/v1/config/deny-rules", tenantAdmin, body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("old-taxonomy body %s status = %d, want 400, body=%s", body, w.Code, w.Body.String())
		}
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

	foundDefault := false
	foundFirefox120 := false
	foundSafari16 := false
	foundLegacyFirefox := false
	foundLegacySafari := false

	for _, p := range profiles {
		switch p.Name {
		case defaultFingerprintProfileName:
			if p.ScopeType == fingerprintProfileScopeGlobal {
				foundDefault = true
			}
		case "firefox_120":
			foundFirefox120 = true
		case "safari_16_0":
			foundSafari16 = true
		case "firefox_121":
			foundLegacyFirefox = true
		case "safari_17":
			foundLegacySafari = true
		}
	}
	if !foundDefault {
		t.Fatalf("profiles = %+v, want built-in default global profile", profiles)
	}
	if !foundFirefox120 {
		t.Fatalf("profiles = %+v, missing firefox_120", profiles)
	}
	if !foundSafari16 {
		t.Fatalf("profiles = %+v, missing safari_16_0", profiles)
	}
	if foundLegacyFirefox {
		t.Fatalf("profiles = %+v, should not contain legacy firefox_121", profiles)
	}
	if foundLegacySafari {
		t.Fatalf("profiles = %+v, should not contain legacy safari_17", profiles)
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

func TestRollbackConfigUsesTransactionalWriter(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	writes := &recordingConfigWrites{}
	ta.h.ConfigWrites = writes
	published := &recordingInvalidationPublisher{}
	ta.h.ConfigCache = NewConfigCache(NewInMemorySnapshotStore(), published)

	tenantAdmin := ta.seedTenantKey(t, "key_rollback_admin", adminTestTenantA, RoleTenantAdmin)
	viewer := ta.seedTenantKey(t, "key_rollback_viewer", adminTestTenantA, RoleViewer)

	w := httptest.NewRecorder()
	ta.h.RollbackConfig(w, newAdminRequest(http.MethodPost, "/api/v1/config/rollback", viewer,
		`{"expected_config_version":4,"target_config_version":2,"reason":"restore"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer rollback status = %d, want 403", w.Code)
	}

	w = httptest.NewRecorder()
	ta.h.RollbackConfig(w, newAdminRequest(http.MethodPost, "/api/v1/config/rollback", tenantAdmin,
		`{"expected_config_version":4,"target_config_version":2,"reason":"restore"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("rollback status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if !writes.rollbackCalled || writes.rollbackTenant != adminTestTenantA {
		t.Fatalf("rollback write = called:%v tenant:%q", writes.rollbackCalled, writes.rollbackTenant)
	}
	if writes.rollbackRequest.TargetConfigVersion != 2 || writes.rollbackActor.ActorID != "key_rollback_admin" {
		t.Fatalf("rollback request=%+v actor=%+v", writes.rollbackRequest, writes.rollbackActor)
	}
	if !published.called || published.tenantID != adminTestTenantA || published.version != 5 {
		t.Fatalf("published = %+v, want tenant %s version 5", published, adminTestTenantA)
	}
}

func TestRollbackConfigVersionConflict(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	ta.h.ConfigWrites = &recordingConfigWrites{rollbackErr: ErrVersionConflict}
	tenantAdmin := ta.seedTenantKey(t, "key_rollback_conflict", adminTestTenantA, RoleTenantAdmin)

	w := httptest.NewRecorder()
	ta.h.RollbackConfig(w, newAdminRequest(http.MethodPost, "/api/v1/config/rollback", tenantAdmin,
		`{"expected_config_version":4,"target_config_version":2,"reason":"restore"}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("rollback conflict status = %d, want 409, body=%s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal conflict response: %v", err)
	}
	if resp.Code != errorCodeConflict {
		t.Fatalf("rollback conflict code = %q, want %q", resp.Code, errorCodeConflict)
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

func TestListChangesRBACAndContent(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_changes_admin", adminTestTenantA, RoleTenantAdmin)
	viewer := ta.seedTenantKey(t, "key_changes_viewer", adminTestTenantA, RoleViewer)
	requester := ta.seedTenantKey(t, "key_changes_requester", adminTestTenantA, RoleRequester)

	// A routing rule write records an audit row for this tenant.
	_, created := createRoutingRule(t, ta, tenantAdmin, `{"id":"route_changes_1","target_pool_id":"`+configTestPoolA+`"}`)
	if created.ID != "route_changes_1" {
		t.Fatalf("create failed: %+v", created)
	}

	// requester lacks tenant_admin/operator/viewer and is forbidden.
	w := httptest.NewRecorder()
	ta.h.ListChanges(w, newAdminRequest(http.MethodGet, "/api/v1/config/changes", requester, ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("requester status = %d, want 403", w.Code)
	}

	// viewer is allowed and sees the recorded change with no secret material.
	w = httptest.NewRecorder()
	ta.h.ListChanges(w, newAdminRequest(http.MethodGet, "/api/v1/config/changes", viewer, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("viewer status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var changes []configChangeResponse

	err := json.Unmarshal(w.Body.Bytes(), &changes)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("changes = %+v, want 1 recorded change", changes)
	}

	if changes[0].ResourceType != resourceTypeRoutingRule || changes[0].ResourceID != "route_changes_1" || changes[0].Action != configActionUpsert {
		t.Fatalf("change = %+v, want routing_rule/route_changes_1/%s", changes[0], configActionUpsert)
	}

	if !strings.Contains(w.Body.String(), `"actor_id"`) || strings.Contains(strings.ToLower(w.Body.String()), "secret") {
		t.Fatalf("response body unexpectedly references secret material: %s", w.Body.String())
	}
}

func TestListChangesTenantIsolation(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminA := ta.seedTenantKey(t, "key_changes_iso_a", adminTestTenantA, RoleTenantAdmin)
	adminB := ta.seedTenantKey(t, "key_changes_iso_b", adminTestTenantB, RoleTenantAdmin)

	_, created := createRoutingRule(t, ta, adminA, `{"id":"route_changes_iso","target_pool_id":"`+configTestPoolA+`"}`)
	if created.ID != "route_changes_iso" {
		t.Fatalf("create for tenant A failed: %+v", created)
	}

	w := httptest.NewRecorder()
	ta.h.ListChanges(w, newAdminRequest(http.MethodGet, "/api/v1/config/changes", adminB, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var changes []configChangeResponse

	err := json.Unmarshal(w.Body.Bytes(), &changes)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(changes) != 0 {
		t.Fatalf("tenant B changes = %+v, want none (isolation)", changes)
	}
}

func TestListChangesPaginationDefaultsAndBounds(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdmin := ta.seedTenantKey(t, "key_changes_page_admin", adminTestTenantA, RoleTenantAdmin)

	for i := range 3 {
		id := "route_page_" + string(rune('a'+i))
		_, created := createRoutingRule(t, ta, tenantAdmin, `{"id":"`+id+`","target_pool_id":"`+configTestPoolA+`"}`)
		if created.ID != id {
			t.Fatalf("create %s failed", id)
		}
	}

	w := httptest.NewRecorder()
	ta.h.ListChanges(w, newAdminRequest(http.MethodGet, "/api/v1/config/changes?limit=2&offset=0", tenantAdmin, ""))

	var page []configChangeResponse

	err := json.Unmarshal(w.Body.Bytes(), &page)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(page) != 2 {
		t.Fatalf("page len = %d, want 2 (limit applied)", len(page))
	}

	// Requesting beyond bounds and with a limit over the max still respects
	// the shared contract (default 50 / max 200) instead of erroring.
	w = httptest.NewRecorder()
	ta.h.ListChanges(w, newAdminRequest(http.MethodGet, "/api/v1/config/changes?limit=1000&offset=0", tenantAdmin, ""))

	var all []configChangeResponse

	err = json.Unmarshal(w.Body.Bytes(), &all)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(all) != 3 {
		t.Fatalf("all = %+v, want 3 (over-max limit clamped, not rejected)", all)
	}

	w = httptest.NewRecorder()
	ta.h.ListChanges(w, newAdminRequest(http.MethodGet, "/api/v1/config/changes?offset=100", tenantAdmin, ""))

	var beyond []configChangeResponse

	err = json.Unmarshal(w.Body.Bytes(), &beyond)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(beyond) != 0 {
		t.Fatalf("beyond = %+v, want none", beyond)
	}
}
