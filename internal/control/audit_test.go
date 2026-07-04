package control

import (
	"context"
	"testing"
	"time"
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
		ResourceID:   "rule_1",
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
	if events[0].ResourceID != "rule_1" {
		t.Fatalf("resource_id = %q, want rule_1", events[0].ResourceID)
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

	recordAudit(context.Background(), store, Identity{TenantID: "ten_b", APIKeyID: "key_2"}, "worker", routingTestWorker1, "disable")

	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].ConfigType != "worker" || events[0].ResourceID != routingTestWorker1 || events[0].Action != "disable" {
		t.Fatalf("event = %+v, want config_type=worker resource_id=worker-1 action=disable", events[0])
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
