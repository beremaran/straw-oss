package control

import "github.com/beremaran/straw-oss/v2/internal/config"

// ConfigCache holds the immutable configuration for this Control process.
type ConfigCache struct {
	snapshot config.Snapshot
}

// NewConfigCache creates an immutable deployment configuration holder.
func NewConfigCache(snapshot config.Snapshot) *ConfigCache {
	return &ConfigCache{snapshot: snapshot.Clone()}
}

// Snapshot returns an isolated copy of the deployment configuration.
func (c *ConfigCache) Snapshot() config.Snapshot {
	return c.snapshot.Clone()
}
