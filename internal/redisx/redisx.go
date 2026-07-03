package redisx

import (
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultDialTimeout  = 2 * time.Second
	defaultReadTimeout  = 500 * time.Millisecond
	defaultWriteTimeout = 500 * time.Millisecond
)

// Config configures a Redis client connection.
type Config struct {
	Addr         string
	Password     string
	DB           int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (c Config) withDefaults() Config {
	if c.DialTimeout == 0 {
		c.DialTimeout = defaultDialTimeout
	}

	if c.ReadTimeout == 0 {
		c.ReadTimeout = defaultReadTimeout
	}

	if c.WriteTimeout == 0 {
		c.WriteTimeout = defaultWriteTimeout
	}

	return c
}

// NewClient builds a Redis client for cfg. It does not dial eagerly; callers
// that need to fail fast on an unreachable Redis should Ping the returned
// client before relying on it.
func NewClient(cfg Config) *redis.Client {
	cfg = cfg.withDefaults()

	return redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})
}
