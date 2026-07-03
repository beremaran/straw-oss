package control

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const stickyOpTimeout = 250 * time.Millisecond

// StickyBackend is the sticky-session storage contract the Router depends
// on. Both the in-process StickyStore (tests, no-Redis dev) and
// RedisStickyStore (P0 durable-ephemeral backing) satisfy it.
type StickyBackend interface {
	Get(tenantID, sessionID string) (string, bool)
	Set(tenantID, sessionID, workerID string, ttl time.Duration)
	Refresh(tenantID, sessionID, workerID string, ttl time.Duration)
}

// RedisStickyStore stores sticky-session pins in Redis using the canonical
// key structure from docs/planning/10-routing-model.md:
// straw:sticky:<tenant_id>:<sticky_session_id>, TTL from the matched rule,
// refreshed on each use.
//
// Fail policy (docs/planning/20 "Sticky sessions: degrade according to
// route policy"): any Redis error degrades Get to "no pin found" rather
// than failing the request outright, and Set/Refresh best-effort no-op.
// The router's existing allow_sticky_fallback logic then decides whether to
// fail the request or select a fresh target, matching the documented "may
// fail sticky requests" behavior without special-casing Redis outages here.
type RedisStickyStore struct {
	client redis.Cmdable
}

// NewRedisStickyStore builds a RedisStickyStore over client.
func NewRedisStickyStore(client redis.Cmdable) *RedisStickyStore {
	return &RedisStickyStore{client: client}
}

// Get returns the pinned worker_id if present and unexpired. Any Redis
// error (including a missing key) reports "no pin found".
func (s *RedisStickyStore) Get(tenantID, sessionID string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), stickyOpTimeout)
	defer cancel()

	v, err := s.client.Get(ctx, stickyKey(tenantID, sessionID)).Result()
	if err != nil {
		return "", false
	}

	return v, true
}

// Set pins sessionID to workerID with the given TTL. Redis errors are
// swallowed: a failed pin degrades to "no sticky affinity" on the next Get,
// per the documented sticky fail policy.
func (s *RedisStickyStore) Set(tenantID, sessionID, workerID string, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), stickyOpTimeout)
	defer cancel()

	_ = s.client.Set(ctx, stickyKey(tenantID, sessionID), workerID, ttl).Err()
}

// Refresh extends the TTL of an existing pin on use.
func (s *RedisStickyStore) Refresh(tenantID, sessionID, workerID string, ttl time.Duration) {
	s.Set(tenantID, sessionID, workerID, ttl)
}
