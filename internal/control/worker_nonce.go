package control

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const workerNonceOpTimeout = 250 * time.Millisecond

// WorkerNonceStore records registration nonces so a captured, signature-valid
// RegisterRequest cannot be replayed (docs/planning/27-security-controls.md
// "Worker Credential Signing"). Consume reports whether nonce was fresh
// (recorded now, first use) for credentialID; false means the nonce was
// already seen (replay). Nonces expire only by ttl and are never reused.
type WorkerNonceStore interface {
	Consume(ctx context.Context, credentialID string, nonce []byte, ttl time.Duration) (fresh bool, err error)
}

// RedisWorkerNonceStore is the Redis-backed WorkerNonceStore
// (docs/planning/21-state-and-storage.md). Each nonce is recorded with `SET
// NX` scoped by credential_id, so a nonce reused under a different
// credential is tracked independently and every key carries a TTL.
type RedisWorkerNonceStore struct {
	client redis.Cmdable
}

// NewRedisWorkerNonceStore builds a RedisWorkerNonceStore over client.
func NewRedisWorkerNonceStore(client redis.Cmdable) *RedisWorkerNonceStore {
	return &RedisWorkerNonceStore{client: client}
}

func workerNonceKey(credentialID string, nonce []byte) string {
	return "straw:workernonce:" + credentialID + ":" + base64.RawURLEncoding.EncodeToString(nonce)
}

// Consume attempts to atomically claim nonce for credentialID. A Redis error
// (including an outage) is returned to the caller, which applies the
// configured fail-open/fail-closed policy; Consume itself takes no stance on
// availability.
func (s *RedisWorkerNonceStore) Consume(ctx context.Context, credentialID string, nonce []byte, ttl time.Duration) (bool, error) {
	opCtx, cancel := context.WithTimeout(ctx, workerNonceOpTimeout)
	defer cancel()

	ok, err := s.client.SetNX(opCtx, workerNonceKey(credentialID, nonce), 1, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("consume worker nonce: %w", err)
	}

	return ok, nil
}
