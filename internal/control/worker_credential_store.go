package control

import (
	"context"
	"errors"
	"sync"
	"time"
)

// WorkerCredentialStatus mirrors the `status` column of `worker_credentials`.
type WorkerCredentialStatus string

const (
	WorkerCredentialStatusActive  WorkerCredentialStatus = "active"
	WorkerCredentialStatusRevoked WorkerCredentialStatus = "revoked"
)

// AllowedPool scopes a worker credential to one pool within one tenant, per
// docs/planning/06-identity-roles-and-tenant-isolation.md.
type AllowedPool struct {
	TenantID string `json:"tenant_id"`
	PoolID   string `json:"pool_id"`
}

// WorkerCredential mirrors the `worker_credentials` table. P0 creation
// forces TenantScope to a single tenant (the caller's) and rejects
// AllowedPools entries referencing any other tenant; multi-tenant
// credentials are a P1, system_admin-only operation.
type WorkerCredential struct {
	ID                     string
	Status                 WorkerCredentialStatus
	ExecutorType           string
	PublicKeyEd25519Base64 string
	TenantScope            []string
	AllowedPools           []AllowedPool
	CreatedAt              time.Time
	UpdatedAt              time.Time
	ConfigVersion          uint64
}

var ErrWorkerCredentialNotFound = errors.New("worker credential not found")

// WorkerCredentialStore persists worker credential records.
type WorkerCredentialStore interface {
	Create(ctx context.Context, record WorkerCredential) error
	Get(ctx context.Context, id string) (WorkerCredential, error)
	Revoke(ctx context.Context, id string, revokedAt time.Time) (WorkerCredential, error)
	ListTenant(ctx context.Context, tenantID string) ([]WorkerCredential, error)
}

// InMemoryWorkerCredentialStore is the P0 store implementation.
type InMemoryWorkerCredentialStore struct {
	mu      sync.RWMutex
	records map[string]WorkerCredential
}

func NewInMemoryWorkerCredentialStore() *InMemoryWorkerCredentialStore {
	return &InMemoryWorkerCredentialStore{records: make(map[string]WorkerCredential)}
}

func (s *InMemoryWorkerCredentialStore) Create(_ context.Context, record WorkerCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[record.ID] = record
	return nil
}

func (s *InMemoryWorkerCredentialStore) Get(_ context.Context, id string) (WorkerCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.records[id]
	if !ok {
		return WorkerCredential{}, ErrWorkerCredentialNotFound
	}
	return r, nil
}

func (s *InMemoryWorkerCredentialStore) Revoke(_ context.Context, id string, revokedAt time.Time) (WorkerCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[id]
	if !ok {
		return WorkerCredential{}, ErrWorkerCredentialNotFound
	}
	r.Status = WorkerCredentialStatusRevoked
	r.UpdatedAt = revokedAt
	s.records[id] = r
	return r, nil
}

func (s *InMemoryWorkerCredentialStore) ListTenant(_ context.Context, tenantID string) ([]WorkerCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []WorkerCredential
	for _, r := range s.records {
		for _, t := range r.TenantScope {
			if t == tenantID {
				out = append(out, r)
				break
			}
		}
	}
	return out, nil
}
