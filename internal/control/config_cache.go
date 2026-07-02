package control

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/beremaran/straw/v2/internal/config"
)

var ErrVersionConflict = errors.New("config version conflict")

type SnapshotStore interface {
	CurrentTenantConfigVersion(ctx context.Context, tenantID string) (uint64, error)
	LoadTenantSnapshot(ctx context.Context, tenantID string, version uint64) (config.TenantSnapshot, error)
	SaveTenantSnapshot(ctx context.Context, snapshot config.TenantSnapshot, expectedVersion uint64) (config.TenantSnapshot, error)
}

type InvalidationPublisher interface {
	PublishTenantInvalidation(ctx context.Context, tenantID string, version uint64) error
}

type ConfigCache struct {
	store         SnapshotStore
	publisher     InvalidationPublisher
	mu            sync.RWMutex
	latest        map[string]config.TenantSnapshot
	latestVersion map[string]uint64
}

func NewConfigCache(store SnapshotStore, publisher InvalidationPublisher) *ConfigCache {
	return &ConfigCache{
		store:         store,
		publisher:     publisher,
		latest:        make(map[string]config.TenantSnapshot),
		latestVersion: make(map[string]uint64),
	}
}

func (c *ConfigCache) Snapshot(ctx context.Context, tenantID string) (config.TenantSnapshot, error) {
	if snapshot, ok := c.cachedSnapshot(tenantID); ok {
		return snapshot.Clone(), nil
	}

	version, err := c.store.CurrentTenantConfigVersion(ctx, tenantID)
	if err != nil {
		if snapshot, ok := c.cachedSnapshot(tenantID); ok {
			return snapshot.Clone(), nil
		}
		return config.TenantSnapshot{}, err
	}

	return c.loadAndStore(ctx, tenantID, version)
}

func (c *ConfigCache) SyncTenantVersion(ctx context.Context, tenantID string) (bool, error) {
	version, err := c.store.CurrentTenantConfigVersion(ctx, tenantID)
	if err != nil {
		if _, ok := c.cachedSnapshot(tenantID); ok {
			return false, nil
		}
		return false, err
	}

	c.mu.RLock()
	currentVersion := c.latestVersion[tenantID]
	c.mu.RUnlock()
	if version <= currentVersion {
		return false, nil
	}

	if _, err := c.loadAndStore(ctx, tenantID, version); err != nil {
		if _, ok := c.cachedSnapshot(tenantID); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *ConfigCache) Save(ctx context.Context, snapshot config.TenantSnapshot, expectedVersion uint64) (config.TenantSnapshot, error) {
	saved, err := c.store.SaveTenantSnapshot(ctx, snapshot.Clone(), expectedVersion)
	if err != nil {
		return config.TenantSnapshot{}, err
	}

	c.storeSnapshot(saved)
	if c.publisher != nil {
		_ = c.publisher.PublishTenantInvalidation(ctx, saved.TenantID, saved.ConfigVersion)
	}
	return saved.Clone(), nil
}

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
