package control

import (
	"context"
	"errors"
	"sync"
	"time"
)

// APIKeyStatus mirrors the `status` column of the `api_keys` table.
type APIKeyStatus string

const (
	// APIKeyStatusActive marks a live API key.
	APIKeyStatusActive APIKeyStatus = "active"
	// APIKeyStatusRevoked marks a revoked API key.
	APIKeyStatusRevoked APIKeyStatus = "revoked"
)

// APIKeyRecord mirrors the `api_keys` table defined in
// migrations/postgres/0001_init.sql. It never carries plaintext secret
// material — only the server-side hash and the visible lookup prefix.
type APIKeyRecord struct {
	ID            string
	ScopeType     ScopeType
	TenantID      string // empty for platform keys
	Role          Role
	Prefix        string
	SecretHash    string
	Status        APIKeyStatus
	CreatedAt     time.Time
	RevokedAt     *time.Time
	ConfigVersion uint64 // 0 for platform keys; tenant config version at creation for tenant keys
}

var (
	// ErrAPIKeyNotFound is returned when a key ID cannot be found.
	ErrAPIKeyNotFound = errors.New("api key not found")
	// ErrAPIKeyAlreadyExist is returned when inserting a duplicate key ID.
	ErrAPIKeyAlreadyExist = errors.New("api key already exists")
)

// APIKeyStore persists API key records. Implementations must never expose
// plaintext secrets.
type APIKeyStore interface {
	Create(ctx context.Context, record APIKeyRecord) error
	FindByPrefix(ctx context.Context, prefix string) ([]APIKeyRecord, error)
	Get(ctx context.Context, id string) (APIKeyRecord, error)
	Revoke(ctx context.Context, id string, revokedAt time.Time) (APIKeyRecord, error)
	ListPlatform(ctx context.Context) ([]APIKeyRecord, error)
	ListTenant(ctx context.Context, tenantID string) ([]APIKeyRecord, error)
	CountPlatformSystemAdmins(ctx context.Context) (int, error)
}

// InMemoryAPIKeyStore is a process-local APIKeyStore. It is the P0 store
// implementation; a Postgres-backed implementation is future work once a
// database driver dependency is introduced.
type InMemoryAPIKeyStore struct {
	mu      sync.RWMutex
	records map[string]APIKeyRecord
}

// NewInMemoryAPIKeyStore builds an empty store.
func NewInMemoryAPIKeyStore() *InMemoryAPIKeyStore {
	return &InMemoryAPIKeyStore{records: make(map[string]APIKeyRecord)}
}

// Create inserts an API key record.
func (s *InMemoryAPIKeyStore) Create(_ context.Context, record APIKeyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.records[record.ID]; exists {
		return ErrAPIKeyAlreadyExist
	}

	s.records[record.ID] = record

	return nil
}

// FindByPrefix returns active records matching a lookup prefix.
func (s *InMemoryAPIKeyStore) FindByPrefix(_ context.Context, prefix string) ([]APIKeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []APIKeyRecord

	for _, r := range s.records {
		if r.Prefix == prefix {
			out = append(out, r)
		}
	}

	return out, nil
}

// Get returns an API key record by ID.
func (s *InMemoryAPIKeyStore) Get(_ context.Context, id string) (APIKeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.records[id]
	if !ok {
		return APIKeyRecord{}, ErrAPIKeyNotFound
	}

	return r, nil
}

// Revoke marks an API key revoked.
func (s *InMemoryAPIKeyStore) Revoke(_ context.Context, id string, revokedAt time.Time) (APIKeyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[id]
	if !ok {
		return APIKeyRecord{}, ErrAPIKeyNotFound
	}

	r.Status = APIKeyStatusRevoked
	t := revokedAt
	r.RevokedAt = &t
	s.records[id] = r

	return r, nil
}

// ListPlatform returns platform-scoped API keys.
func (s *InMemoryAPIKeyStore) ListPlatform(_ context.Context) ([]APIKeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []APIKeyRecord

	for _, r := range s.records {
		if r.ScopeType == ScopePlatform {
			out = append(out, r)
		}
	}

	return out, nil
}

// ListTenant returns tenant-scoped API keys for a tenant.
func (s *InMemoryAPIKeyStore) ListTenant(_ context.Context, tenantID string) ([]APIKeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []APIKeyRecord

	for _, r := range s.records {
		if r.ScopeType == ScopeTenant && r.TenantID == tenantID {
			out = append(out, r)
		}
	}

	return out, nil
}

// CountPlatformSystemAdmins returns the number of active platform admins.
func (s *InMemoryAPIKeyStore) CountPlatformSystemAdmins(_ context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0

	for _, r := range s.records {
		if r.ScopeType == ScopePlatform && r.Role == RoleSystemAdmin && r.Status == APIKeyStatusActive {
			count++
		}
	}

	return count, nil
}
