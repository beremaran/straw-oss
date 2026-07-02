package control

import (
	"context"
	"sync"

	"github.com/beremaran/straw/v2/internal/config"
)

// InMemorySnapshotStore is the P0 SnapshotStore implementation backing
// ConfigCache. A Postgres-backed implementation is future work once a
// database driver dependency is introduced for Control; until then this
// process-local store is the durable-ish backing used by admin handlers
// and tests. Unknown tenants read as an implicit version-0 empty
// snapshot rather than an error, so first writes for a brand-new tenant
// succeed without a separate "initialize snapshot" step.
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

func (s *InMemorySnapshotStore) CurrentTenantConfigVersion(_ context.Context, tenantID string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.versions[tenantID], nil
}

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
