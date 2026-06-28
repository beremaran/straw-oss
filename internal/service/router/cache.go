package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/metrics"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	ActiveRulesKeyPrefix = "router:rules:v"

	RulesVersionKey = "router:rules:version"

	DefaultCacheTTL = 10 * time.Minute
)

type RuleCache struct {
	client *redis.Client
	ttl    time.Duration
	tracer trace.Tracer
}

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
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rules from cache (v%d): %w", version, err)
	}

	return rules, nil
}

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
	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("failed to set rules in cache (v%d): %w", version, err)
	}

	return nil
}

func (c *RuleCache) IncrementRulesVersion(ctx context.Context) (int64, error) {
	return c.client.Incr(ctx, RulesVersionKey).Result()
}

func (c *RuleCache) Invalidate(ctx context.Context, version int64) error {
	key := fmt.Sprintf("%s%d", ActiveRulesKeyPrefix, version)

	return c.client.Del(ctx, key).Err()
}
