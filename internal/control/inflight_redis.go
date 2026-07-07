package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// redisInFlightKeyPrefix stores short-lived in-flight request ownership as
	// straw:inflight:<request_id> -> owning tenant_id. This is the
	// "short-lived in-flight request state" runtime tier from docs/planning/21;
	// the key carries a TTL so a crashed owner's record self-expires rather
	// than leaking (docs/planning/21 requires a TTL on every runtime-state key).
	redisInFlightKeyPrefix = "straw:inflight:"
	// redisRequestCancelChannel is the cross-instance cancel pub/sub channel. It
	// is an ephemeral, TTL-exempt runtime signal (docs/planning/21). Every
	// Control replica subscribes; only the replica that owns the request_id
	// locally acts on the message.
	redisRequestCancelChannel = "straw:request:cancel"

	// defaultInFlightRecordTTL bounds how long an ownership record survives an
	// owner crash. It must comfortably exceed the maximum request deadline;
	// Deregister removes the record on normal completion well before this.
	defaultInFlightRecordTTL = 10 * time.Minute
	// inFlightOpTimeout bounds each Redis operation so a slow backend cannot
	// stall the dispatch or admin-cancel path.
	inFlightOpTimeout = 2 * time.Second
)

func redisInFlightKey(requestID string) string {
	return redisInFlightKeyPrefix + requestID
}

// RedisInFlightCoordinator implements InFlightCrossInstance over the Redis
// runtime-state tier so an admin cancel reaches the Control instance that owns
// an in-flight request (docs/tasks/p1/23). It records ownership as a
// TTL-bounded key and signals cancels over a pub/sub channel; the owning
// instance applies the cancel to its local context via RedisRequestCancelSubscriber.
type RedisInFlightCoordinator struct {
	client redis.Cmdable
	ttl    time.Duration
}

// NewRedisInFlightCoordinator builds a coordinator over client. A non-positive
// ttl falls back to defaultInFlightRecordTTL.
func NewRedisInFlightCoordinator(client redis.Cmdable, ttl time.Duration) *RedisInFlightCoordinator {
	if ttl <= 0 {
		ttl = defaultInFlightRecordTTL
	}

	return &RedisInFlightCoordinator{client: client, ttl: ttl}
}

// Record advertises ownership of requestID by this instance. A failure is
// logged, not returned: it only disables cross-instance cancel for this one
// request (a sibling cancel would fall through to the not-found outcome), and
// must never block the dispatch that already began.
func (c *RedisInFlightCoordinator) Record(ctx context.Context, requestID, tenantID string) {
	// Detach from the request context: the ownership record must outlive the
	// request's own cancellation (Clear runs at teardown, when ctx is already
	// canceled), and the write must not be aborted by a client disconnect.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inFlightOpTimeout)
	defer cancel()

	err := c.client.Set(ctx, redisInFlightKey(requestID), tenantID, c.ttl).Err()
	if err != nil {
		slog.Warn("record in-flight ownership failed; cross-instance cancel unavailable for request",
			"request_id", requestID, "error", err)
	}
}

// Clear removes the ownership record on request completion.
func (c *RedisInFlightCoordinator) Clear(ctx context.Context, requestID string) {
	// Detached: Clear typically runs on a canceled request context (deferred at
	// teardown), yet the ownership record still needs to be removed promptly.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inFlightOpTimeout)
	defer cancel()

	err := c.client.Del(ctx, redisInFlightKey(requestID)).Err()
	if err != nil {
		slog.Warn("clear in-flight ownership failed", "request_id", requestID, "error", err)
	}
}

// Lookup returns the owning tenant_id advertised for requestID. A redis.Nil
// (no owner) returns ok=false with no log; any other error also returns
// ok=false (the cancel then collapses to the existing not-found outcome) and
// is logged.
func (c *RedisInFlightCoordinator) Lookup(ctx context.Context, requestID string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, inFlightOpTimeout)
	defer cancel()

	tenantID, err := c.client.Get(ctx, redisInFlightKey(requestID)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("lookup in-flight ownership failed", "request_id", requestID, "error", err)
		}

		return "", false
	}

	return tenantID, true
}

// SignalCancel publishes requestID on the cross-instance cancel channel for the
// owning instance to consume.
func (c *RedisInFlightCoordinator) SignalCancel(ctx context.Context, requestID string) error {
	ctx, cancel := context.WithTimeout(ctx, inFlightOpTimeout)
	defer cancel()

	err := c.client.Publish(ctx, redisRequestCancelChannel, requestID).Err()
	if err != nil {
		return fmt.Errorf("signal cross-instance cancel: %w", err)
	}

	return nil
}

// RedisRequestCancelSubscriber listens on the cross-instance cancel channel and
// applies each cancel to the local InFlightRegistry. A message for a request
// this instance does not own is ignored, so exactly one replica — the owner —
// tears the request down. It requires a dedicated *redis.Client (pub/sub
// reserves the connection).
type RedisRequestCancelSubscriber struct {
	client   *redis.Client
	registry *InFlightRegistry
}

// NewRedisRequestCancelSubscriber builds a subscriber applying received cancels
// to registry.
func NewRedisRequestCancelSubscriber(client *redis.Client, registry *InFlightRegistry) *RedisRequestCancelSubscriber {
	return &RedisRequestCancelSubscriber{client: client, registry: registry}
}

// Run subscribes to the cancel channel and applies incoming cancels until ctx
// is canceled. It returns nil on a clean shutdown (ctx canceled or the channel
// closing) and an error only if the initial subscribe fails.
func (s *RedisRequestCancelSubscriber) Run(ctx context.Context) error {
	pubsub := s.client.Subscribe(ctx, redisRequestCancelChannel)
	defer func() { _ = pubsub.Close() }()

	_, err := pubsub.Receive(ctx)
	if err != nil {
		return fmt.Errorf("subscribe request cancel: %w", err)
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

			s.registry.cancelLocal(msg.Payload)
		}
	}
}
