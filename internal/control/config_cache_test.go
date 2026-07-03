package control

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/beremaran/straw/v2/internal/config"
)

const (
	testTenantA = "tenant-a"
	testKeyA    = "key-a"
	testKeyB    = "key-b"
)

func TestConfigCacheSnapshotHit(t *testing.T) {
	t.Parallel()

	store := newFakeSnapshotStore()
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 1, []string{testKeyA}))
	store.setCurrentVersion(testTenantA, 1)
	cache := NewConfigCache(store, nil)

	snapshot, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.ConfigVersion != 1 {
		t.Fatalf("Snapshot() version = %d, want 1", snapshot.ConfigVersion)
	}
	if got := store.loadCalls; got != 1 {
		t.Fatalf("LoadTenantSnapshot calls = %d, want 1", got)
	}

	snapshot.RevokedAPIKeyIDs[0] = "mutated"

	again, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() second call error = %v", err)
	}
	if again.RevokedAPIKeyIDs[0] != testKeyA {
		t.Fatalf("cached snapshot mutated = %q, want %q", again.RevokedAPIKeyIDs[0], testKeyA)
	}
	if got := store.loadCalls; got != 1 {
		t.Fatalf("LoadTenantSnapshot calls after cache hit = %d, want 1", got)
	}
}

func TestConfigCacheSaveVersionConflict(t *testing.T) {
	t.Parallel()

	store := newFakeSnapshotStore()
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 1, nil))
	store.setCurrentVersion(testTenantA, 1)
	cache := NewConfigCache(store, nil)

	_, err := cache.Save(context.Background(), config.NewTenantSnapshot(testTenantA, 2, nil), 0)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("Save() error = %v, want %v", err, ErrVersionConflict)
	}
	if got := store.saveCalls; got != 1 {
		t.Fatalf("SaveTenantSnapshot calls = %d, want 1", got)
	}
}

func TestConfigCacheInvalidationLoadsNewVersion(t *testing.T) {
	t.Parallel()

	store := newFakeSnapshotStore()
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 1, []string{testKeyA}))
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 2, []string{testKeyA, testKeyB}))
	store.setCurrentVersion(testTenantA, 1)
	cache := NewConfigCache(store, nil)

	first, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if first.ConfigVersion != 1 {
		t.Fatalf("Snapshot() version = %d, want 1", first.ConfigVersion)
	}

	store.setCurrentVersion(testTenantA, 2)
	cache.ApplyInvalidation(testTenantA, 2)

	second, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() after invalidation error = %v", err)
	}
	if second.ConfigVersion != 2 {
		t.Fatalf("Snapshot() version = %d, want 2", second.ConfigVersion)
	}
	if got := store.loadCalls; got != 2 {
		t.Fatalf("LoadTenantSnapshot calls = %d, want 2", got)
	}
}

func TestConfigCacheMissedPubSubRecovery(t *testing.T) {
	t.Parallel()

	store := newFakeSnapshotStore()
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 1, nil))
	store.setCurrentVersion(testTenantA, 1)
	cache := NewConfigCache(store, nil)

	_, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 2, nil))
	store.setCurrentVersion(testTenantA, 2)

	updated, err := cache.SyncTenantVersion(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("SyncTenantVersion() error = %v", err)
	}
	if !updated {
		t.Fatalf("SyncTenantVersion() updated = false, want true")
	}

	snapshot, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() after sync error = %v", err)
	}
	if snapshot.ConfigVersion != 2 {
		t.Fatalf("Snapshot() version = %d, want 2", snapshot.ConfigVersion)
	}
}

func TestConfigCacheAPIKeyRevocationInvalidation(t *testing.T) {
	t.Parallel()

	store := newFakeSnapshotStore()
	store.seedSnapshot(config.NewTenantSnapshot("tenant-a", 1, []string{"key-a"}))
	store.setCurrentVersion("tenant-a", 1)
	publisher := &fakeInvalidationPublisher{}
	cache := NewConfigCache(store, publisher)

	saved, err := cache.Save(context.Background(), config.NewTenantSnapshot("tenant-a", 2, []string{"key-a", "key-b"}), 1)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.ConfigVersion != 2 {
		t.Fatalf("Save() version = %d, want 2", saved.ConfigVersion)
	}
	if len(saved.RevokedAPIKeyIDs) != 2 || saved.RevokedAPIKeyIDs[1] != "key-b" {
		t.Fatalf("Save() revoked keys = %v, want [key-a key-b]", saved.RevokedAPIKeyIDs)
	}
	if got := publisher.calls; got != 1 {
		t.Fatalf("PublishTenantInvalidation calls = %d, want 1", got)
	}
}

// TestConfigCachePollAllTenantsRecoversMissedInvalidation proves the
// periodic Postgres poll (docs/planning/25 "periodic Postgres version
// poll") refreshes every cached tenant whose durable version has advanced,
// without needing a pub/sub message.
func TestConfigCachePollAllTenantsRecoversMissedInvalidation(t *testing.T) {
	t.Parallel()

	store := newFakeSnapshotStore()
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 1, nil))
	store.setCurrentVersion(testTenantA, 1)
	store.seedSnapshot(config.NewTenantSnapshot("tenant-b", 1, nil))
	store.setCurrentVersion("tenant-b", 1)
	cache := NewConfigCache(store, nil)

	_, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() tenant-a error = %v", err)
	}

	_, err = cache.Snapshot(context.Background(), "tenant-b")
	if err != nil {
		t.Fatalf("Snapshot() tenant-b error = %v", err)
	}

	// tenant-a's durable version advances without a pub/sub message reaching
	// the cache; tenant-b is untouched.
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 2, []string{testKeyA}))
	store.setCurrentVersion(testTenantA, 2)

	cache.PollAllTenants(context.Background())

	snapshotA, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() tenant-a after poll error = %v", err)
	}

	if snapshotA.ConfigVersion != 2 {
		t.Fatalf("tenant-a Snapshot() version = %d, want 2", snapshotA.ConfigVersion)
	}

	if got := store.loadCalls; got != 3 {
		t.Fatalf("LoadTenantSnapshot calls = %d, want 3 (2 initial loads + 1 poll refresh)", got)
	}
}

type fakeSnapshotStore struct {
	mu        sync.Mutex
	versions  map[string]uint64
	snapshots map[string]map[uint64]config.TenantSnapshot
	loadCalls int
	saveCalls int
}

func newFakeSnapshotStore() *fakeSnapshotStore {
	return &fakeSnapshotStore{
		versions:  make(map[string]uint64),
		snapshots: make(map[string]map[uint64]config.TenantSnapshot),
	}
}

func (s *fakeSnapshotStore) CurrentTenantConfigVersion(_ context.Context, tenantID string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.versions[tenantID], nil
}

func (s *fakeSnapshotStore) LoadTenantSnapshot(_ context.Context, tenantID string, version uint64) (config.TenantSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loadCalls++
	snapshot, ok := s.snapshots[tenantID][version]
	if !ok {
		return config.TenantSnapshot{}, errors.New("snapshot not found")
	}

	return snapshot.Clone(), nil
}

func (s *fakeSnapshotStore) SaveTenantSnapshot(_ context.Context, snapshot config.TenantSnapshot, expectedVersion uint64) (config.TenantSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.saveCalls++
	current := s.versions[snapshot.TenantID]
	if current != expectedVersion {
		return config.TenantSnapshot{}, ErrVersionConflict
	}
	if snapshot.ConfigVersion <= current {
		return config.TenantSnapshot{}, errors.New("snapshot version must advance")
	}
	if s.snapshots[snapshot.TenantID] == nil {
		s.snapshots[snapshot.TenantID] = make(map[uint64]config.TenantSnapshot)
	}
	s.snapshots[snapshot.TenantID][snapshot.ConfigVersion] = snapshot.Clone()
	s.versions[snapshot.TenantID] = snapshot.ConfigVersion

	return snapshot.Clone(), nil
}

func (s *fakeSnapshotStore) seedSnapshot(snapshot config.TenantSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.snapshots[snapshot.TenantID] == nil {
		s.snapshots[snapshot.TenantID] = make(map[uint64]config.TenantSnapshot)
	}
	s.snapshots[snapshot.TenantID][snapshot.ConfigVersion] = snapshot.Clone()
}

func (s *fakeSnapshotStore) setCurrentVersion(tenantID string, version uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.versions[tenantID] = version
}

type fakeInvalidationPublisher struct {
	calls int
}

func (p *fakeInvalidationPublisher) PublishTenantInvalidation(context.Context, string, uint64) error {
	p.calls++

	return nil
}
