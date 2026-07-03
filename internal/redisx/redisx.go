package redisx

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// errRedisURLEnvEmpty is returned by ResolveURL when the named environment
// variable is unset or empty.
var errRedisURLEnvEmpty = errors.New("redis: URL environment variable is empty")

// defaultRedisURLEnv is used when Config.URLEnv is unset, mirroring
// postgresx's STRAW_POSTGRES_DSN default pattern.
const defaultRedisURLEnv = "STRAW_REDIS_URL"

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

// ResolveURL reads the Redis connection URL from the environment variable
// named urlEnv, defaulting to STRAW_REDIS_URL when urlEnv is empty. Returns
// an error if the variable is unset or empty.
func ResolveURL(urlEnv string) (string, error) {
	if urlEnv == "" {
		urlEnv = defaultRedisURLEnv
	}

	url := os.Getenv(urlEnv)
	if url == "" {
		return "", fmt.Errorf("%w: %s", errRedisURLEnvEmpty, urlEnv)
	}

	return url, nil
}

// NewClientFromURL parses rawURL (a redis:// or rediss:// connection
// string) and builds a client, applying cfg's timeouts over whatever the URL
// specifies. It does not dial eagerly.
func NewClientFromURL(rawURL string, cfg Config) (*redis.Client, error) {
	cfg = cfg.withDefaults()

	opts, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}

	opts.DialTimeout = cfg.DialTimeout
	opts.ReadTimeout = cfg.ReadTimeout
	opts.WriteTimeout = cfg.WriteTimeout

	return redis.NewClient(opts), nil
}
