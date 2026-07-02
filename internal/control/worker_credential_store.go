package control

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// WorkerCredentialStatus mirrors the `status` column of `worker_credentials`.
type WorkerCredentialStatus string

const (
	// WorkerCredentialStatusActive marks a live worker credential.
	WorkerCredentialStatusActive WorkerCredentialStatus = "active"
	// WorkerCredentialStatusRevoked marks a revoked worker credential.
	WorkerCredentialStatusRevoked WorkerCredentialStatus = "revoked"
)

// AllowedPool scopes a worker credential to one pool within one tenant, per
// docs/planning/06-identity-roles-and-tenant-isolation.md.
type AllowedPool struct {
	TenantID string `json:"tenant_id"`
	PoolID   string `json:"pool_id"`
}

// WorkerCapabilities bounds the capabilities a worker may claim at
// registration. Each non-empty list is an allow-list: a registering worker
// may only claim values contained in it. An empty list means the credential
// places no restriction on that dimension (P0 permissive default; the
// tenant_admin create API does not yet author these, so they are set
// directly by platform tooling/tests). See
// docs/planning/06-identity-roles-and-tenant-isolation.md and the
// allowed_capabilities schema in docs/planning/26.
type WorkerCapabilities struct {
	Tags                  []string `json:"tags"`
	Countries             []string `json:"countries"`
	Regions               []string `json:"regions"`
	IPTypes               []string `json:"ip_types"`
	SupportedIngressModes []string `json:"supported_ingress_modes"`
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
	AllowedCapabilities    WorkerCapabilities
	CreatedAt              time.Time
	UpdatedAt              time.Time
	ConfigVersion          uint64
}

// ErrWorkerCredentialNotFound is returned when a worker credential ID is missing.
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

// NewInMemoryWorkerCredentialStore builds an empty worker credential store.
func NewInMemoryWorkerCredentialStore() *InMemoryWorkerCredentialStore {
	return &InMemoryWorkerCredentialStore{records: make(map[string]WorkerCredential)}
}

// Create inserts a worker credential record.
func (s *InMemoryWorkerCredentialStore) Create(_ context.Context, record WorkerCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[record.ID] = record

	return nil
}

// Get fetches a worker credential by ID.
func (s *InMemoryWorkerCredentialStore) Get(_ context.Context, id string) (WorkerCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.records[id]
	if !ok {
		return WorkerCredential{}, ErrWorkerCredentialNotFound
	}

	return r, nil
}

// Revoke marks a worker credential revoked.
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

// ListTenant returns the credentials scoped to a tenant.
func (s *InMemoryWorkerCredentialStore) ListTenant(_ context.Context, tenantID string) ([]WorkerCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []WorkerCredential

	for _, r := range s.records {
		if slices.Contains(r.TenantScope, tenantID) {
			out = append(out, r)
		}
	}

	return out, nil
}
