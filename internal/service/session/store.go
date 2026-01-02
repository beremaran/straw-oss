package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	goredis "github.com/redis/go-redis/v9"
)

// Store defines the interface for session storage.
type Store interface {
	// Save creates or updates a session.
	Save(ctx context.Context, session *domain.Session, ttl time.Duration) error
	// Get retrieves a session by ID.
	Get(ctx context.Context, id string) (*domain.Session, error)
	// Delete removes a session.
	Delete(ctx context.Context, id string) error
	// Touch extends the session's expiration time.
	Touch(ctx context.Context, id string, ttl time.Duration) error
}

// RedisStore implements Store using Redis.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new Redis session store.
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{
		client: client,
	}
}

// Save implements Store.Save.
func (s *RedisStore) Save(ctx context.Context, session *domain.Session, ttl time.Duration) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := s.key(session.ID)
	if err := s.client.Client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to save session to redis: %w", err)
	}

	return nil
}

// Get implements Store.Get.
func (s *RedisStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	key := s.key(id)
	data, err := s.client.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, domain.ErrSessionExpired
		}
		// Since we cannot verify the exact error type without importing redis v9 directly (which we did in imports),
		// we should be careful. The infra package exposes *redis.Client, so we are coupled to it.
		// Let's assume we can rely on string check if we don't want to import redis here, but importing it is fine.
		// Actually I'll use a hack to avoid importing the driver just for the error if I can, but importing is cleaner.
		// I will rely on standard error conventions or specific helper if available.
		// Re-reading infra/client.go: it imports "github.com/redis/go-redis/v9".
		// I will assume I can't import "github.com/redis/go-redis/v9" unless I add it to go.mod, which it likely is.
		// For now, let's just return key not found error from domain.
		return nil, fmt.Errorf("failed to get session from redis: %w", err)
	}

	var session domain.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// Delete implements Store.Delete.
func (s *RedisStore) Delete(ctx context.Context, id string) error {
	key := s.key(id)
	if err := s.client.Client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete session from redis: %w", err)
	}
	return nil
}

// Touch implements Store.Touch.
func (s *RedisStore) Touch(ctx context.Context, id string, ttl time.Duration) error {
	key := s.key(id)
	if err := s.client.Client.Expire(ctx, key, ttl).Err(); err != nil {
		return fmt.Errorf("failed to extend session ttl: %w", err)
	}
	return nil
}

func (s *RedisStore) key(id string) string {
	return fmt.Sprintf("session:%s", id)
}
