package control

import (
	"context"
	"errors"
	"sync"
	"time"
)

// TenantStatus mirrors the `status` column of the `tenants` table.
type TenantStatus string

const (
	// TenantStatusActive marks a live tenant record.
	TenantStatusActive TenantStatus = "active"
	// TenantStatusDeleted marks a deleted tenant record.
	TenantStatusDeleted TenantStatus = "deleted"
)

// Tenant is the minimal P0 tenant record needed to support platform key
// bootstrap and RBAC tests for this task. The full tenant config resource
// schema (rate_limit_ceiling, timeouts, metadata storage policy, ...) is
// defined in docs/planning/26-config-management-api-surface.md and is
// populated by later tasks; this task only needs enough of a tenant
// boundary to prove that tenant-scoped keys cannot create tenants and that
// system_admin can.
type Tenant struct {
	ID        string
	Name      string
	Status    TenantStatus
	CreatedAt time.Time
}

var (
	// ErrTenantNotFound is returned when a tenant ID cannot be found.
	ErrTenantNotFound = errors.New("tenant not found")
	// ErrTenantAlreadyExists is returned when inserting a duplicate tenant.
	ErrTenantAlreadyExists = errors.New("tenant already exists")
)

// TenantStore persists tenant boundary records.
type TenantStore interface {
	Create(ctx context.Context, tenant Tenant) error
	Get(ctx context.Context, id string) (Tenant, error)
}

// InMemoryTenantStore is the P0 store implementation.
type InMemoryTenantStore struct {
	mu      sync.RWMutex
	tenants map[string]Tenant
}

// NewInMemoryTenantStore builds an empty tenant store.
func NewInMemoryTenantStore() *InMemoryTenantStore {
	return &InMemoryTenantStore{tenants: make(map[string]Tenant)}
}

// Create inserts a tenant record.
func (s *InMemoryTenantStore) Create(_ context.Context, tenant Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tenants[tenant.ID]; exists {
		return ErrTenantAlreadyExists
	}

	s.tenants[tenant.ID] = tenant

	return nil
}

// Get fetches a tenant record by ID.
func (s *InMemoryTenantStore) Get(_ context.Context, id string) (Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tenants[id]
	if !ok {
		return Tenant{}, ErrTenantNotFound
	}

	return t, nil
}
