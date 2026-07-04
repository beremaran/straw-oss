package control

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// redisInvalidationChannelPrefix is the Redis pub/sub channel prefix from
// docs/planning/25: straw:config:invalidate:<tenant_id>. Pub/sub is an
// acceleration mechanism only, not a durable channel — ConfigCache.PollAllTenants
// is the durable Postgres-backed fallback for a missed message.
const redisInvalidationChannelPrefix = "straw:config:invalidate:"

func redisInvalidationChannel(tenantID string) string {
	return redisInvalidationChannelPrefix + tenantID
}

// RedisInvalidationPublisher publishes tenant config invalidation messages
// over Redis pub/sub after a committed config write.
type RedisInvalidationPublisher struct {
	client redis.Cmdable
}

// NewRedisInvalidationPublisher builds a publisher over client.
func NewRedisInvalidationPublisher(client redis.Cmdable) *RedisInvalidationPublisher {
	return &RedisInvalidationPublisher{client: client}
}

// PublishTenantInvalidation publishes the new version for tenantID. A
// publish failure (e.g. Redis outage) is returned to the caller but never
// blocks the config write that already committed to Postgres and applied
// locally to ConfigCache; the periodic Postgres poll recovers a lost
// publish.
func (p *RedisInvalidationPublisher) PublishTenantInvalidation(ctx context.Context, tenantID string, version uint64) error {
	err := p.client.Publish(ctx, redisInvalidationChannel(tenantID), strconv.FormatUint(version, 10)).Err()
	if err != nil {
		return fmt.Errorf("publish tenant invalidation: %w", err)
	}

	return nil
}

// RedisInvalidationSubscriber listens for Redis pub/sub invalidation
// messages and applies them to a ConfigCache. It requires a dedicated
// *redis.Client (pub/sub reserves the connection), separate from the
// Cmdable used for other Redis operations.
type RedisInvalidationSubscriber struct {
	client *redis.Client
	cache  *ConfigCache
}

// NewRedisInvalidationSubscriber builds a subscriber over client, applying
// received invalidations to cache.
func NewRedisInvalidationSubscriber(client *redis.Client, cache *ConfigCache) *RedisInvalidationSubscriber {
	return &RedisInvalidationSubscriber{client: client, cache: cache}
}

// Run subscribes to every tenant's invalidation channel and applies incoming
// messages to the cache until ctx is canceled. It returns nil on a clean
// shutdown (ctx canceled or the pub/sub channel closing) and an error only
// if the initial subscribe fails.
func (s *RedisInvalidationSubscriber) Run(ctx context.Context) error {
	pubsub := s.client.PSubscribe(ctx, redisInvalidationChannelPrefix+"*")
	defer func() { _ = pubsub.Close() }()

	_, err := pubsub.Receive(ctx)
	if err != nil {
		return fmt.Errorf("subscribe config invalidation: %w", err)
	}

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}

			s.applyMessage(msg)
		}
	}
}

func (s *RedisInvalidationSubscriber) applyMessage(msg *redis.Message) {
	tenantID := strings.TrimPrefix(msg.Channel, redisInvalidationChannelPrefix)
	if tenantID == "" {
		return
	}

	version, err := strconv.ParseUint(msg.Payload, 10, 64)
	if err != nil {
		slog.Warn("invalid config invalidation payload", "tenant_id", tenantID, "error", err)

		return
	}

	s.cache.ApplyInvalidation(tenantID, version)
}
