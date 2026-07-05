package control

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

// AuditRecord mirrors the `config_audit_source` table. actor_id is always
// the API key ID in P0 (docs/planning/26). Secret fields must be redacted
// before a record is written; this task never writes plaintext key
// material or hashes into audit records.
// AuditRecord mirrors the config audit table.
type AuditRecord struct {
	ID            int64
	TenantID      string // empty for platform-scoped actions
	ActorType     string
	ActorID       string
	ResourceType  string
	ResourceID    string
	Action        string
	CreatedAt     time.Time
	ConfigVersion uint64
	FieldPath     string
	OldValueJSON  string
	NewValueJSON  string
	SkipPostgres  bool
}

// AuditStore persists config audit records.
// AuditStore persists config audit records.
type AuditStore interface {
	Record(ctx context.Context, record AuditRecord) error
	ListTenant(ctx context.Context, tenantID string) ([]AuditRecord, error)
	// ListTenantPage returns a tenant's audit history ordered created_at
	// descending then id ascending, per the shared list contract
	// (docs/planning/26). Callers pass an already-clamped limit.
	ListTenantPage(ctx context.Context, tenantID string, limit, offset int) ([]AuditRecord, error)
}

// InMemoryAuditStore is the P0 store implementation.
// InMemoryAuditStore is the P0 audit store implementation.
type InMemoryAuditStore struct {
	mu      sync.RWMutex
	records []AuditRecord
	nextID  int64
}

// NewInMemoryAuditStore builds an empty in-memory audit store.
func NewInMemoryAuditStore() *InMemoryAuditStore {
	return &InMemoryAuditStore{}
}

// Record appends an audit record.
func (s *InMemoryAuditStore) Record(_ context.Context, record AuditRecord) error {
	if record.SkipPostgres {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++

	record.ID = s.nextID
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	s.records = append(s.records, record)

	return nil
}

// ListTenant returns audit records for a tenant.
func (s *InMemoryAuditStore) ListTenant(_ context.Context, tenantID string) ([]AuditRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []AuditRecord

	for _, r := range s.records {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}

	return out, nil
}

// ListTenantPage returns a paginated, tenant-scoped view of the audit log,
// sorted created_at descending then id ascending.
func (s *InMemoryAuditStore) ListTenantPage(_ context.Context, tenantID string, limit, offset int) ([]AuditRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []AuditRecord

	for _, r := range s.records {
		if r.TenantID == tenantID {
			matched = append(matched, r)
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}

		return matched[i].ID < matched[j].ID
	})

	if offset >= len(matched) {
		return []AuditRecord{}, nil
	}

	end := min(offset+limit, len(matched))

	return matched[offset:end], nil
}

// recordAudit is a small helper used by admin handlers so every mutation
// site records actor, resource, and action consistently.
func recordAudit(
	ctx context.Context,
	store AuditStore,
	identity Identity,
	resourceType, resourceID, action string,
	configVersion uint64,
	fieldPath string,
	oldVal, newVal any,
	skipPostgres bool,
) {
	if store == nil {
		return
	}

	oldJSON, _ := redactAndMarshal(oldVal)
	newJSON, _ := redactAndMarshal(newVal)

	_ = store.Record(ctx, AuditRecord{
		TenantID:      identity.TenantID,
		ActorType:     configActorTypeAPIKey,
		ActorID:       identity.APIKeyID,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Action:        action,
		ConfigVersion: configVersion,
		FieldPath:     fieldPath,
		OldValueJSON:  oldJSON,
		NewValueJSON:  newJSON,
		SkipPostgres:  skipPostgres,
	})
}

// redactAndMarshal converts the object to its redacted JSON representation.
// It classifies and redacts secret fields like value_base64 in injection policies.
func redactAndMarshal(v any) (string, error) {
	if v == nil {
		return "", nil
	}

	switch val := v.(type) {
	case config.InjectionPolicy:
		val = redactInjectionPolicy(val)
		v = val
	case *config.InjectionPolicy:
		if val != nil {
			clone := redactInjectionPolicy(*val)
			v = &clone
		}
	}

	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("json marshal: %w", err)
	}

	return string(b), nil
}

// auditStoreWithEvents wraps an AuditStore so every successful Record also
// mirrors into the config_audit_events ClickHouse sink (docs/tasks/p0/32),
// covering every recordAudit call site (tenant, API key, worker credential,
// routing/deny/injection/pool config, worker admin, request cancel) from one
// choke point. Old/new value JSON and config_version are populated only by
// the separate Postgres config_audit_source writer in
// postgres_config_store.go (insertConfigAudit), which already carries those
// fields redacted; rows mirrored from here leave them empty since
// AuditRecord does not carry them.
type auditStoreWithEvents struct {
	AuditStore
	events ConfigAuditRecorder
}

// NewAuditStoreWithEvents wraps store so every Record call also enqueues a
// config_audit_events row. If events is nil, store is returned unwrapped.
func NewAuditStoreWithEvents(store AuditStore, events ConfigAuditRecorder) AuditStore {
	if events == nil {
		return store
	}

	return &auditStoreWithEvents{AuditStore: store, events: events}
}

func (s *auditStoreWithEvents) Record(ctx context.Context, record AuditRecord) error {
	err := s.AuditStore.Record(ctx, record)
	if err != nil {
		return fmt.Errorf("record audit: %w", err)
	}

	s.events.Enqueue(ConfigAuditEvent{
		Timestamp:     time.Now().UTC(),
		TenantID:      record.TenantID,
		ActorType:     record.ActorType,
		ActorID:       record.ActorID,
		ConfigType:    record.ResourceType,
		ResourceID:    record.ResourceID,
		Action:        record.Action,
		ConfigVersion: record.ConfigVersion,
		FieldPath:     record.FieldPath,
		OldValueJSON:  record.OldValueJSON,
		NewValueJSON:  record.NewValueJSON,
	})

	return nil
}
