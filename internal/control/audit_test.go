package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

const (
	auditTestEnabledKey = "enabled"
	auditTestRuleID     = "rule_1"
)

func TestAuditStoreWithEventsMirrorsRecordToConfigAuditEvents(t *testing.T) {
	t.Parallel()

	inner := NewInMemoryAuditStore()
	recorder := &captureConfigAuditRecorder{}
	store := NewAuditStoreWithEvents(inner, recorder)

	err := store.Record(context.Background(), AuditRecord{
		TenantID:     adminTestTenantA,
		ActorType:    configActorTypeAPIKey,
		ActorID:      authTestKey1,
		ResourceType: resourceTypeRoutingRule,
		ResourceID:   auditTestRuleID,
		Action:       configActionUpsert,
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	// The wrapped store's own behavior (Postgres/InMemory) must be
	// unaffected: recordAudit's callers still read back through ListTenant.
	records, err := inner.ListTenant(context.Background(), adminTestTenantA)
	if err != nil {
		t.Fatalf("ListTenant() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("inner records len = %d, want 1", len(records))
	}

	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("config audit events len = %d, want 1", len(events))
	}
	if events[0].TenantID != adminTestTenantA {
		t.Fatalf("tenant_id = %q, want ten_a", events[0].TenantID)
	}
	if events[0].ActorID != authTestKey1 {
		t.Fatalf("actor_id = %q, want key_1", events[0].ActorID)
	}
	if events[0].ConfigType != resourceTypeRoutingRule {
		t.Fatalf("config_type = %q, want %q", events[0].ConfigType, resourceTypeRoutingRule)
	}
	if events[0].ResourceID != auditTestRuleID {
		t.Fatalf("resource_id = %q, want %s", events[0].ResourceID, auditTestRuleID)
	}
	if events[0].Action != configActionUpsert {
		t.Fatalf("action = %q, want %q", events[0].Action, configActionUpsert)
	}
}

func TestNewAuditStoreWithEventsNilRecorderReturnsUnwrapped(t *testing.T) {
	t.Parallel()

	inner := NewInMemoryAuditStore()

	store := NewAuditStoreWithEvents(inner, nil)
	if store != AuditStore(inner) {
		t.Fatal("NewAuditStoreWithEvents(store, nil) should return store unwrapped")
	}
}

func TestRecordAuditMirrorsToConfigAuditEvents(t *testing.T) {
	t.Parallel()

	recorder := &captureConfigAuditRecorder{}
	store := NewAuditStoreWithEvents(NewInMemoryAuditStore(), recorder)

	recordAudit(context.Background(), store, Identity{TenantID: "ten_b", APIKeyID: "key_2"}, "worker", routingTestWorker1, "disable", 0, auditFieldPathAll, nil, nil, false)

	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].ConfigType != "worker" || events[0].ResourceID != routingTestWorker1 || events[0].Action != "disable" {
		t.Fatalf("event = %+v, want config_type=worker resource_id=worker-1 action=disable", events[0])
	}
}

func TestRecordAuditEnrichmentAndRedaction(t *testing.T) {
	t.Parallel()

	recorder := &captureConfigAuditRecorder{}
	inner := NewInMemoryAuditStore()
	store := NewAuditStoreWithEvents(inner, recorder)

	// An injection policy with sensitive header operation.
	policy := config.InjectionPolicy{
		ID:      "inject_1",
		Enabled: true,
		Operations: []config.InjectionOperation{
			{
				Op:          "set",
				HeaderName:  testAuthorizationHeader,
				ValueBase64: "c2VjcmV0X2tleV8xMjM0NQ==", // secret value
			},
		},
	}

	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "injection_policy", "inject_1", "update", 7, "operations", nil, policy, false)

	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.ConfigVersion != 7 {
		t.Fatalf("config_version = %d, want 7", ev.ConfigVersion)
	}
	if ev.FieldPath != "operations" {
		t.Fatalf("field_path = %q, want operations", ev.FieldPath)
	}
	if ev.OldValueJSON != "" {
		t.Fatalf("old_value_json = %q, want empty", ev.OldValueJSON)
	}
	// The secret ValueBase64 must be redacted!
	if ev.NewValueJSON == "" {
		t.Fatal("new_value_json is empty")
	}
	if !strings.Contains(ev.NewValueJSON, `"[redacted]"`) {
		t.Fatalf("newValueJSON = %s, expected redacted ValueBase64", ev.NewValueJSON)
	}
	if strings.Contains(ev.NewValueJSON, "c2VjcmV0X2tleV8xMjM0NQ==") {
		t.Fatalf("newValueJSON contains plaintext secret")
	}
}

func TestRecordAuditRedactsAPIKeySecretHash(t *testing.T) {
	t.Parallel()

	const secretHash = "peppered-secret-hash-should-never-leak"

	recorder := &captureConfigAuditRecorder{}
	inner := NewInMemoryAuditStore()
	store := NewAuditStoreWithEvents(inner, recorder)

	created := APIKeyRecord{
		ID:         "key_1",
		ScopeType:  ScopePlatform,
		Role:       RoleSystemAdmin,
		Prefix:     "sk_pfx",
		SecretHash: secretHash,
		Status:     APIKeyStatusActive,
	}
	revoked := created
	revoked.Status = APIKeyStatusRevoked

	// create: new value is the record with SecretHash populated.
	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "platform_api_key", "key_1", "create", 1, auditFieldPathAll, nil, created, false)
	// revoke: both old and new values carry SecretHash (pointer form).
	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "platform_api_key", "key_1", "revoke", 2, auditFieldPathAll, &created, &revoked, false)

	events := recorder.all()
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}

	for _, ev := range events {
		if strings.Contains(ev.NewValueJSON, secretHash) {
			t.Fatalf("new_value_json leaks secret hash: %s", ev.NewValueJSON)
		}
		if strings.Contains(ev.OldValueJSON, secretHash) {
			t.Fatalf("old_value_json leaks secret hash: %s", ev.OldValueJSON)
		}
	}

	// The Postgres-bound records must also be redacted.
	records, err := inner.ListTenant(context.Background(), adminTestTenantA)
	if err != nil {
		t.Fatalf("ListTenant() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}
	for _, rec := range records {
		if strings.Contains(rec.NewValueJSON, secretHash) || strings.Contains(rec.OldValueJSON, secretHash) {
			t.Fatalf("audit record leaks secret hash: old=%s new=%s", rec.OldValueJSON, rec.NewValueJSON)
		}
	}
}

func TestRecordAuditFieldPathDerivedOnUpdate(t *testing.T) {
	t.Parallel()

	recorder := &captureConfigAuditRecorder{}
	inner := NewInMemoryAuditStore()
	store := NewAuditStoreWithEvents(inner, recorder)

	oldRule := config.RoutingRule{
		ID:       auditTestRuleID,
		Enabled:  true,
		Priority: 10,
		Match:    config.MatchConditions{TargetHost: "*.old.example", IngressType: IngressTypeREST},
	}
	newRule := config.RoutingRule{
		ID:       auditTestRuleID,
		Enabled:  true,
		Priority: 20,
		Match:    config.MatchConditions{TargetHost: "*.example.com", IngressType: IngressTypeREST},
	}

	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "routing_rule", auditTestRuleID, "update", 7, auditFieldPathAll, oldRule, newRule, false)

	const wantPath = "match_conditions.target_host,priority"

	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].FieldPath != wantPath {
		t.Fatalf("event field_path = %q, want %q", events[0].FieldPath, wantPath)
	}

	records, err := inner.ListTenant(context.Background(), adminTestTenantA)
	if err != nil {
		t.Fatalf("ListTenant() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].FieldPath != wantPath {
		t.Fatalf("record field_path = %q, want %q", records[0].FieldPath, wantPath)
	}
}

func TestRecordAuditFieldPathSkipsMetadataChurn(t *testing.T) {
	t.Parallel()

	recorder := &captureConfigAuditRecorder{}
	store := NewAuditStoreWithEvents(NewInMemoryAuditStore(), recorder)

	oldTenant := Tenant{ID: adminTestTenantA, Name: "old", UpdatedAt: time.Unix(1, 0).UTC(), ConfigVersion: 4}
	newTenant := oldTenant
	newTenant.Name = "new"
	newTenant.UpdatedAt = time.Unix(2, 0).UTC()
	newTenant.ConfigVersion = 5

	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "tenant", adminTestTenantA, "update", 5, auditFieldPathAll, oldTenant, newTenant, false)

	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].FieldPath != "name" {
		t.Fatalf("event field_path = %q, want name", events[0].FieldPath)
	}
}

func TestRecordAuditFieldPathSentinelWhenNotDiffable(t *testing.T) {
	t.Parallel()

	recorder := &captureConfigAuditRecorder{}
	store := NewAuditStoreWithEvents(NewInMemoryAuditStore(), recorder)

	rule := map[string]any{auditTestEnabledKey: true}

	// create: no old value, so the whole-object sentinel is kept.
	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "routing_rule", auditTestRuleID, "create", 1, auditFieldPathAll, nil, rule, false)
	// no-op update: nothing differs, so the sentinel is kept.
	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "routing_rule", auditTestRuleID, "update", 2, auditFieldPathAll, rule, rule, false)

	events := recorder.all()
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	for i, ev := range events {
		if ev.FieldPath != auditFieldPathAll {
			t.Fatalf("events[%d].field_path = %q, want %q", i, ev.FieldPath, auditFieldPathAll)
		}
	}
}

func TestRecordAuditSkipPostgres(t *testing.T) {
	t.Parallel()

	recorder := &captureConfigAuditRecorder{}
	inner := NewInMemoryAuditStore()
	store := NewAuditStoreWithEvents(inner, recorder)

	recordAudit(context.Background(), store, Identity{TenantID: adminTestTenantA, APIKeyID: authTestKey1}, "routing_rule", auditTestRuleID, "upsert", 3, auditFieldPathAll, nil, nil, true)

	// It must NOT write to inner store (Postgres double-write prevention).
	records, err := inner.ListTenant(context.Background(), adminTestTenantA)
	if err != nil {
		t.Fatalf("ListTenant() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("inner records len = %d, want 0 (SkipPostgres is true)", len(records))
	}

	// But ClickHouse event must still be written!
	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].ConfigVersion != 3 {
		t.Fatalf("config_version = %d, want 3", events[0].ConfigVersion)
	}
}

func TestConfigAuditEventWriterOutageDoesNotBlockRecord(t *testing.T) {
	t.Parallel()

	writer := NewConfigAuditEventWriter(failingConfigAuditSink{}, 10, 10, time.Hour)
	t.Cleanup(writer.Close)

	store := NewAuditStoreWithEvents(NewInMemoryAuditStore(), writer)

	err := store.Record(context.Background(), AuditRecord{TenantID: adminTestTenantA, ResourceType: "tenant", ResourceID: adminTestTenantA, Action: "create"})
	if err != nil {
		t.Fatalf("Record() error = %v, want nil (ClickHouse outage must not fail the audit write)", err)
	}

	flushErr := writer.Flush(context.Background())
	if flushErr == nil {
		t.Fatal("Flush() error = nil, want outage error")
	}
}

type captureConfigAuditRecorder struct {
	events []ConfigAuditEvent
}

func (r *captureConfigAuditRecorder) Enqueue(event ConfigAuditEvent) {
	r.events = append(r.events, event)
}

func (r *captureConfigAuditRecorder) all() []ConfigAuditEvent {
	return append([]ConfigAuditEvent(nil), r.events...)
}

type failingConfigAuditSink struct{}

func (failingConfigAuditSink) WriteConfigAuditEvents(context.Context, []ConfigAuditEvent) error {
	return errClickHouseInsertFailed
}
