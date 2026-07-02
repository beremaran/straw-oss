package control

import (
	"context"
	"sync"
	"time"
)

// AuditRecord mirrors the `config_audit_source` table. actor_id is always
// the API key ID in P0 (docs/planning/26). Secret fields must be redacted
// before a record is written; this task never writes plaintext key
// material or hashes into audit records.
// AuditRecord mirrors the config audit table.
type AuditRecord struct {
	ID           int64
	TenantID     string // empty for platform-scoped actions
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceID   string
	Action       string
	CreatedAt    time.Time
}

// AuditStore persists config audit records.
// AuditStore persists config audit records.
type AuditStore interface {
	Record(ctx context.Context, record AuditRecord) error
	ListTenant(ctx context.Context, tenantID string) ([]AuditRecord, error)
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

// recordAudit is a small helper used by admin handlers so every mutation
// site records actor, resource, and action consistently.
func recordAudit(ctx context.Context, store AuditStore, identity Identity, resourceType, resourceID, action string) {
	if store == nil {
		return
	}

	_ = store.Record(ctx, AuditRecord{
		TenantID:     identity.TenantID,
		ActorType:    "api_key",
		ActorID:      identity.APIKeyID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
	})
}
