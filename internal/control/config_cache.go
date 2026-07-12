package control

import (
	"sync/atomic"

	"github.com/beremaran/straw-oss/v2/internal/config"
)

// ConfigCache holds the immutable configuration for this Control process.
type ConfigCache struct {
	snapshot atomic.Pointer[config.Snapshot]
}

// NewConfigCache creates an immutable deployment configuration holder.
func NewConfigCache(snapshot config.Snapshot) *ConfigCache {
	cache := &ConfigCache{}
	cache.Replace(snapshot)

	return cache
}

// Snapshot returns an isolated copy of the deployment configuration.
func (c *ConfigCache) Snapshot() config.Snapshot {
	if c == nil || c.snapshot.Load() == nil {
		return config.Snapshot{}
	}

	return c.snapshot.Load().Clone()
}

// Replace atomically activates a validated immutable snapshot for new requests.
func (c *ConfigCache) Replace(snapshot config.Snapshot) {
	cloned := snapshot.Clone()
	c.snapshot.Store(&cloned)
}
