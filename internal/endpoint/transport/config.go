// Package transport provides connection pooling for the endpoint HTTP client.
// It manages per-host transport pools with LRU eviction for memory efficiency.
package transport

import "time"

// PoolConfig configures the connection pool behavior.
type PoolConfig struct {
	// MaxPoolHosts is the maximum number of distinct hosts to maintain pools for.
	// When exceeded, the least recently used host pool is evicted.
	// Default: 1000
	MaxPoolHosts int

	// IdleConnsPerHost is the number of idle keep-alive connections per host.
	// Default: 10
	IdleConnsPerHost int

	// IdleConnTimeout is the maximum duration an idle connection remains open.
	// Default: 90s
	IdleConnTimeout time.Duration

	// EvictionInterval is how often to check for and evict stale host pools.
	// Default: 5m
	EvictionInterval time.Duration
}

// DefaultPoolConfig returns a PoolConfig with sensible defaults
// matching the design document Section 2.3.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxPoolHosts:     1000,
		IdleConnsPerHost: 10,
		IdleConnTimeout:  90 * time.Second,
		EvictionInterval: 5 * time.Minute,
	}
}

// WithMaxPoolHosts returns a copy of the config with MaxPoolHosts set.
func (c PoolConfig) WithMaxPoolHosts(n int) PoolConfig {
	c.MaxPoolHosts = n
	return c
}

// WithIdleConnsPerHost returns a copy of the config with IdleConnsPerHost set.
func (c PoolConfig) WithIdleConnsPerHost(n int) PoolConfig {
	c.IdleConnsPerHost = n
	return c
}

// WithIdleConnTimeout returns a copy of the config with IdleConnTimeout set.
func (c PoolConfig) WithIdleConnTimeout(d time.Duration) PoolConfig {
	c.IdleConnTimeout = d
	return c
}

// WithEvictionInterval returns a copy of the config with EvictionInterval set.
func (c PoolConfig) WithEvictionInterval(d time.Duration) PoolConfig {
	c.EvictionInterval = d
	return c
}
