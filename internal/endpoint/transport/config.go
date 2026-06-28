package transport

import "time"

type PoolConfig struct {
	MaxPoolHosts int

	IdleConnsPerHost int

	IdleConnTimeout time.Duration

	EvictionInterval time.Duration
}

func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxPoolHosts:     1000,
		IdleConnsPerHost: 10,
		IdleConnTimeout:  90 * time.Second,
		EvictionInterval: 5 * time.Minute,
	}
}

func (c PoolConfig) WithMaxPoolHosts(n int) PoolConfig {
	c.MaxPoolHosts = n

	return c
}

func (c PoolConfig) WithIdleConnsPerHost(n int) PoolConfig {
	c.IdleConnsPerHost = n

	return c
}

func (c PoolConfig) WithIdleConnTimeout(d time.Duration) PoolConfig {
	c.IdleConnTimeout = d

	return c
}

func (c PoolConfig) WithEvictionInterval(d time.Duration) PoolConfig {
	c.EvictionInterval = d

	return c
}
