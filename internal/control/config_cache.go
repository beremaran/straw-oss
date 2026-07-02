package control

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/beremaran/straw/v2/internal/config"
)

// ErrVersionConflict is returned when a snapshot write races a newer version.
var ErrVersionConflict = errors.New("config version conflict")

// SnapshotStore reads and writes tenant snapshots and versions.
type SnapshotStore interface {
	CurrentTenantConfigVersion(ctx context.Context, tenantID string) (uint64, error)
	LoadTenantSnapshot(ctx context.Context, tenantID string, version uint64) (config.TenantSnapshot, error)
	SaveTenantSnapshot(ctx context.Context, snapshot config.TenantSnapshot, expectedVersion uint64) (config.TenantSnapshot, error)
}

// InvalidationPublisher broadcasts tenant snapshot invalidations.
type InvalidationPublisher interface {
	PublishTenantInvalidation(ctx context.Context, tenantID string, version uint64) error
}

// ConfigCache caches tenant snapshots in memory and keeps them in sync
// with the backing store.
type ConfigCache struct {
	store         SnapshotStore
	publisher     InvalidationPublisher
	mu            sync.RWMutex
	latest        map[string]config.TenantSnapshot
	latestVersion map[string]uint64
}

// NewConfigCache builds an empty config cache.
func NewConfigCache(store SnapshotStore, publisher InvalidationPublisher) *ConfigCache {
	return &ConfigCache{
		store:         store,
		publisher:     publisher,
		latest:        make(map[string]config.TenantSnapshot),
		latestVersion: make(map[string]uint64),
	}
}

func wrapSnapshotStoreErr(tenantID string, version uint64, op string, err error) error {
	return fmt.Errorf("%s tenant %s version %d: %w", op, tenantID, version, err)
}

// Snapshot returns the cached snapshot for a tenant, loading it on demand.
func (c *ConfigCache) Snapshot(ctx context.Context, tenantID string) (config.TenantSnapshot, error) {
	if snapshot, ok := c.cachedSnapshot(tenantID); ok {
		return snapshot.Clone(), nil
	}

	version, err := c.store.CurrentTenantConfigVersion(ctx, tenantID)
	if err != nil {
		if snapshot, ok := c.cachedSnapshot(tenantID); ok {
			return snapshot.Clone(), nil
		}

		return config.TenantSnapshot{}, wrapSnapshotStoreErr(tenantID, 0, "load config version for", err)
	}

	return c.loadAndStore(ctx, tenantID, version)
}

// SyncTenantVersion refreshes the cached snapshot when the store version advances.
func (c *ConfigCache) SyncTenantVersion(ctx context.Context, tenantID string) (bool, error) {
	version, err := c.store.CurrentTenantConfigVersion(ctx, tenantID)
	if err != nil {
		if _, ok := c.cachedSnapshot(tenantID); ok {
			return false, nil
		}

		return false, wrapSnapshotStoreErr(tenantID, 0, "sync config version for", err)
	}

	c.mu.RLock()
	currentVersion := c.latestVersion[tenantID]
	c.mu.RUnlock()

	if version <= currentVersion {
		return false, nil
	}

	_, err = c.loadAndStore(ctx, tenantID, version)
	if err != nil {
		if _, ok := c.cachedSnapshot(tenantID); ok {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// Save persists a snapshot and updates the cache on success.
func (c *ConfigCache) Save(ctx context.Context, snapshot config.TenantSnapshot, expectedVersion uint64) (config.TenantSnapshot, error) {
	saved, err := c.store.SaveTenantSnapshot(ctx, snapshot.Clone(), expectedVersion)
	if err != nil {
		return config.TenantSnapshot{}, wrapSnapshotStoreErr(snapshot.TenantID, expectedVersion, "save tenant snapshot for", err)
	}

	c.storeSnapshot(saved)

	if c.publisher != nil {
		_ = c.publisher.PublishTenantInvalidation(ctx, saved.TenantID, saved.ConfigVersion)
	}

	return saved.Clone(), nil
}

// ApplyInvalidation clears stale cached state for a tenant version.
func (c *ConfigCache) ApplyInvalidation(tenantID string, version uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if current, ok := c.latestVersion[tenantID]; ok && version <= current {
		return
	}

	c.latestVersion[tenantID] = version
	delete(c.latest, tenantID)
}

func (c *ConfigCache) cachedSnapshot(tenantID string) (config.TenantSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snapshot, ok := c.latest[tenantID]

	return snapshot, ok
}

func (c *ConfigCache) loadAndStore(ctx context.Context, tenantID string, version uint64) (config.TenantSnapshot, error) {
	snapshot, err := c.store.LoadTenantSnapshot(ctx, tenantID, version)
	if err != nil {
		return config.TenantSnapshot{}, fmt.Errorf("load tenant %s version %d: %w", tenantID, version, err)
	}

	c.storeSnapshot(snapshot)

	return snapshot.Clone(), nil
}

func (c *ConfigCache) storeSnapshot(snapshot config.TenantSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.latest[snapshot.TenantID] = snapshot.Clone()
	c.latestVersion[snapshot.TenantID] = snapshot.ConfigVersion
}
