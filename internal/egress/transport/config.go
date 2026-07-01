// Package transport provides a TLS pooled HTTP transport with fingerprint-based
// host isolation and LRU eviction.
package transport

import "time"

const (
	defaultMaxPoolHosts     = 1000
	defaultIdleConnsPerHost = 10
	defaultIdleConnTimeout  = 90 * time.Second
	defaultEvictionInterval = 5 * time.Minute
)

// PoolConfig configures the pooled transport's connection pooling behavior.
type PoolConfig struct {
	MaxPoolHosts     int
	IdleConnsPerHost int
	IdleConnTimeout  time.Duration
	EvictionInterval time.Duration
}

// DefaultPoolConfig returns a PoolConfig with sensible defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxPoolHosts:     defaultMaxPoolHosts,
		IdleConnsPerHost: defaultIdleConnsPerHost,
		IdleConnTimeout:  defaultIdleConnTimeout,
		EvictionInterval: defaultEvictionInterval,
	}
}

// WithMaxPoolHosts sets the maximum number of host pools.
func (c PoolConfig) WithMaxPoolHosts(n int) PoolConfig {
	c.MaxPoolHosts = n

	return c
}

// WithIdleConnsPerHost sets the maximum idle connections per host.
func (c PoolConfig) WithIdleConnsPerHost(n int) PoolConfig {
	c.IdleConnsPerHost = n

	return c
}

// WithIdleConnTimeout sets the maximum idle connection timeout.
func (c PoolConfig) WithIdleConnTimeout(d time.Duration) PoolConfig {
	c.IdleConnTimeout = d

	return c
}

// WithEvictionInterval sets the interval between idle connection eviction runs.
func (c PoolConfig) WithEvictionInterval(d time.Duration) PoolConfig {
	c.EvictionInterval = d

	return c
}
