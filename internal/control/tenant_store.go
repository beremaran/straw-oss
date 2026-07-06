package control

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// TenantStatus mirrors the `status` column of the `tenants` table.
type TenantStatus string

const (
	// TenantStatusActive marks a live tenant record.
	TenantStatusActive TenantStatus = "active"
	// TenantStatusSuspended marks a tenant whose keys are rejected but whose
	// record and config are retained.
	TenantStatusSuspended TenantStatus = "suspended"
	// TenantStatusDeleted marks a soft-deleted tenant record.
	TenantStatusDeleted TenantStatus = "deleted"
)

const (
	defaultTenantDefaultTimeoutMs = 60000
	defaultTenantMaxTimeoutMs     = 300000
	defaultMetadataQueryStorage   = MetadataStorageDrop
	defaultMetadataPathStorage    = MetadataStorageHash
)

// MetadataStoragePolicy controls how URL query/path metadata is stored.
type MetadataStoragePolicy string

const (
	// MetadataStorageDrop omits the URL component from stored metadata.
	MetadataStorageDrop MetadataStoragePolicy = "drop"
	// MetadataStorageHash stores a stable hash for correlation.
	MetadataStorageHash MetadataStoragePolicy = "hash"
	// MetadataStorageStore stores the URL component as-is.
	MetadataStorageStore MetadataStoragePolicy = "store"
)

// Tenant is the P0 tenant resource
// (docs/planning/26-config-management-api-surface.md Tenant schema, P0
// subset: name, status, timeout/storage policy, rate_limit_ceiling,
// config_version).
type Tenant struct {
	ID                   string
	Name                 string
	Status               TenantStatus
	DefaultTimeoutMs     uint64
	MaxTimeoutMs         uint64
	MetadataQueryStorage MetadataStoragePolicy
	MetadataPathStorage  MetadataStoragePolicy
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
	// RateLimitCeiling bounds tenant-managed rate-limit values
	// (docs/planning/26); nil means unbounded. Settable only by
	// system_admin.
	RateLimitCeiling *RateLimitCeiling
	// ConfigVersion is this tenant record's own optimistic-concurrency
	// version, checked against expected_config_version on PUT
	// (docs/planning/26 "Shared Config API Contract"). It is distinct from
	// the tenant_config_versions snapshot version used for cache
	// invalidation.
	ConfigVersion uint64
}

func normalizeTenant(t Tenant) Tenant {
	if t.DefaultTimeoutMs == 0 {
		t.DefaultTimeoutMs = defaultTenantDefaultTimeoutMs
	}

	if t.MaxTimeoutMs == 0 {
		t.MaxTimeoutMs = defaultTenantMaxTimeoutMs
	}

	if t.MetadataQueryStorage == "" {
		t.MetadataQueryStorage = defaultMetadataQueryStorage
	}

	if t.MetadataPathStorage == "" {
		t.MetadataPathStorage = defaultMetadataPathStorage
	}

	return t
}

func validateTenantPolicy(t Tenant) error {
	if t.DefaultTimeoutMs < minRequestTimeoutMs || t.MaxTimeoutMs < minRequestTimeoutMs || t.DefaultTimeoutMs > t.MaxTimeoutMs {
		return errInvalidTenantTimeouts
	}

	if !validMetadataStorage(t.MetadataQueryStorage) || !validMetadataStorage(t.MetadataPathStorage) {
		return errInvalidTenantMetadata
	}

	return nil
}

func validMetadataStorage(v MetadataStoragePolicy) bool {
	switch v {
	case MetadataStorageDrop, MetadataStorageHash, MetadataStorageStore:
		return true
	default:
		return false
	}
}

var (
	// ErrTenantNotFound is returned when a tenant ID cannot be found, or is
	// already soft-deleted for writes that require a live tenant.
	ErrTenantNotFound = errors.New("tenant not found")
	// ErrTenantAlreadyExists is returned when inserting a duplicate tenant.
	ErrTenantAlreadyExists = errors.New("tenant already exists")
	// ErrTenantVersionConflict is returned when a tenant update's
	// expected_config_version does not match the tenant's current
	// config_version.
	ErrTenantVersionConflict = errors.New("tenant config version conflict")
	errInvalidTenantTimeouts = errors.New("invalid tenant timeout bounds")
	errInvalidTenantMetadata = errors.New("invalid tenant metadata storage policy")
)

// TenantStore persists tenant boundary records.
type TenantStore interface {
	Create(ctx context.Context, tenant Tenant) error
	Get(ctx context.Context, id string) (Tenant, error)
	// List returns live and soft-deleted tenants, newest first, per the
	// shared config-list pagination contract.
	List(ctx context.Context, limit, offset int) ([]Tenant, error)
	// Update replaces the tenant's name, status, and rate_limit_ceiling
	// under optimistic concurrency. Returns ErrTenantNotFound for a
	// missing or already-deleted tenant, ErrTenantVersionConflict on a
	// version mismatch.
	Update(ctx context.Context, tenant Tenant, expectedVersion uint64) (Tenant, error)
	// SoftDelete marks a tenant deleted. Returns ErrTenantNotFound if the
	// tenant is missing or already deleted.
	SoftDelete(ctx context.Context, id string) (Tenant, error)
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

	tenant = normalizeTenant(tenant)

	err := validateTenantPolicy(tenant)
	if err != nil {
		return err
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

// List returns tenants ordered by CreatedAt descending, then ID ascending.
func (s *InMemoryTenantStore) List(_ context.Context, limit, offset int) ([]Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		out = append(out, t)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}

		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	if offset >= len(out) {
		return []Tenant{}, nil
	}

	end := min(offset+limit, len(out))

	return out[offset:end], nil
}

// Update replaces name/status/rate_limit_ceiling under optimistic concurrency.
func (s *InMemoryTenantStore) Update(_ context.Context, tenant Tenant, expectedVersion uint64) (Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.tenants[tenant.ID]
	if !ok || current.Status == TenantStatusDeleted {
		return Tenant{}, ErrTenantNotFound
	}

	if current.ConfigVersion != expectedVersion {
		return Tenant{}, ErrTenantVersionConflict
	}

	updated := current
	tenant = normalizeTenant(tenant)

	err := validateTenantPolicy(tenant)
	if err != nil {
		return Tenant{}, err
	}

	updated.Name = tenant.Name
	updated.Status = tenant.Status
	updated.DefaultTimeoutMs = tenant.DefaultTimeoutMs
	updated.MaxTimeoutMs = tenant.MaxTimeoutMs
	updated.MetadataQueryStorage = tenant.MetadataQueryStorage
	updated.MetadataPathStorage = tenant.MetadataPathStorage
	updated.RateLimitCeiling = tenant.RateLimitCeiling
	updated.ConfigVersion = current.ConfigVersion + 1
	updated.UpdatedAt = time.Now().UTC()

	s.tenants[tenant.ID] = updated

	return updated, nil
}

// SoftDelete marks a tenant deleted.
func (s *InMemoryTenantStore) SoftDelete(_ context.Context, id string) (Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.tenants[id]
	if !ok || current.Status == TenantStatusDeleted {
		return Tenant{}, ErrTenantNotFound
	}

	now := time.Now().UTC()
	current.Status = TenantStatusDeleted
	current.DeletedAt = &now
	current.UpdatedAt = now
	current.ConfigVersion++
	s.tenants[id] = current

	return current, nil
}
