package control

import (
	"context"
	"sync"

	"github.com/beremaran/straw/v2/internal/config"
)

// InMemorySnapshotStore is a test/local SnapshotStore implementation for
// ConfigCache. Runtime Control wires PostgresConfigStore. Unknown tenants read
// as an implicit version-0 empty snapshot rather than an error, so first writes
// for a brand-new tenant succeed without a separate initialize step.
type InMemorySnapshotStore struct {
	mu        sync.Mutex
	versions  map[string]uint64
	snapshots map[string]map[uint64]config.TenantSnapshot
}

// NewInMemorySnapshotStore builds an empty store.
func NewInMemorySnapshotStore() *InMemorySnapshotStore {
	return &InMemorySnapshotStore{
		versions:  make(map[string]uint64),
		snapshots: make(map[string]map[uint64]config.TenantSnapshot),
	}
}

// CurrentTenantConfigVersion returns the latest known version for a tenant.
func (s *InMemorySnapshotStore) CurrentTenantConfigVersion(_ context.Context, tenantID string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.versions[tenantID], nil
}

// LoadTenantSnapshot returns a stored tenant snapshot for a version.
func (s *InMemorySnapshotStore) LoadTenantSnapshot(_ context.Context, tenantID string, version uint64) (config.TenantSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if snap, ok := s.snapshots[tenantID][version]; ok {
		return snap.Clone(), nil
	}

	if version == 0 {
		return config.NewTenantSnapshot(tenantID, 0, nil), nil
	}

	return config.TenantSnapshot{}, ErrVersionConflict
}

// SaveTenantSnapshot stores a tenant snapshot with optimistic concurrency.
func (s *InMemorySnapshotStore) SaveTenantSnapshot(_ context.Context, snapshot config.TenantSnapshot, expectedVersion uint64) (config.TenantSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.versions[snapshot.TenantID]
	if current != expectedVersion {
		return config.TenantSnapshot{}, ErrVersionConflict
	}

	if snapshot.ConfigVersion <= current {
		return config.TenantSnapshot{}, ErrVersionConflict
	}

	if s.snapshots[snapshot.TenantID] == nil {
		s.snapshots[snapshot.TenantID] = make(map[uint64]config.TenantSnapshot)
	}

	s.snapshots[snapshot.TenantID][snapshot.ConfigVersion] = snapshot.Clone()
	s.versions[snapshot.TenantID] = snapshot.ConfigVersion

	return snapshot.Clone(), nil
}
