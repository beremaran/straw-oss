// Package router provides rule matching, caching, and fingerprint selection
// for the relay routing system.
package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/metrics"
)

const (
	// ActiveRulesKeyPrefix is the Redis key prefix for stored routing rules by version.
	ActiveRulesKeyPrefix = "router:rules:v"

	// RulesVersionKey is the Redis key for the current routing rules version counter.
	RulesVersionKey = "router:rules:version"

	// DefaultCacheTTL is the default time-to-live for cached routing rules.
	DefaultCacheTTL = 10 * time.Minute
)

// RuleCache caches routing rules in Redis with versioning support.
type RuleCache struct {
	client *redis.Client
	ttl    time.Duration
	tracer trace.Tracer
}

// NewRuleCache creates a new RuleCache with the given Redis client and TTL.
func NewRuleCache(client *redis.Client, ttl time.Duration) *RuleCache {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}

	return &RuleCache{
		client: client,
		ttl:    ttl,
		tracer: otel.Tracer("service/router/cache"),
	}
}

// GetRulesVersion retrieves the current rules version from cache.
func (c *RuleCache) GetRulesVersion(ctx context.Context) (int64, error) {
	ctx, span := c.tracer.Start(ctx, "cache.get", trace.WithAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("db.operation", "get"),
		attribute.String("db.redis.key", RulesVersionKey),
	))
	defer span.End()

	val, err := c.client.Get(ctx, RulesVersionKey).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}

		return 0, fmt.Errorf("failed to get rules version: %w", err)
	}

	return val, nil
}

// GetRulesByVersion retrieves cached rules for a specific version.
func (c *RuleCache) GetRulesByVersion(ctx context.Context, version int64) ([]domain.RoutingRule, error) {
	ctx, span := c.tracer.Start(ctx, "cache.get", trace.WithAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("db.operation", "get"),
		attribute.Int64("rule.version", version),
	))
	defer span.End()

	key := fmt.Sprintf("%s%d", ActiveRulesKeyPrefix, version)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			if metrics.CacheMisses != nil {
				metrics.CacheMisses.WithLabelValues("rules").Inc()
			}

			return nil, nil
		}

		return nil, fmt.Errorf("failed to get rules from cache (v%d): %w", version, err)
	}

	if metrics.CacheHits != nil {
		metrics.CacheHits.WithLabelValues("rules").Inc()
	}

	var rules []domain.RoutingRule

	err = json.Unmarshal(data, &rules)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal rules from cache (v%d): %w", version, err)
	}

	return rules, nil
}

// SetRulesByVersion stores rules in cache for a specific version.
func (c *RuleCache) SetRulesByVersion(ctx context.Context, version int64, rules []domain.RoutingRule) error {
	ctx, span := c.tracer.Start(ctx, "cache.set", trace.WithAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("db.operation", "set"),
		attribute.Int64("rule.version", version),
		attribute.Int("rule.count", len(rules)),
	))
	defer span.End()

	data, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("failed to marshal rules for cache: %w", err)
	}

	key := fmt.Sprintf("%s%d", ActiveRulesKeyPrefix, version)

	err = c.client.Set(ctx, key, data, c.ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set rules in cache (v%d): %w", version, err)
	}

	return nil
}

// IncrementRulesVersion atomically increments the rules version counter.
func (c *RuleCache) IncrementRulesVersion(ctx context.Context) (int64, error) {
	val, err := c.client.Incr(ctx, RulesVersionKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment rules version: %w", err)
	}

	return val, nil
}

// Invalidate removes cached rules for a specific version.
func (c *RuleCache) Invalidate(ctx context.Context, version int64) error {
	key := fmt.Sprintf("%s%d", ActiveRulesKeyPrefix, version)

	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to invalidate cache (v%d): %w", version, err)
	}

	return nil
}
