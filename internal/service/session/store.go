package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
	goredis "github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{
		client: client,
	}
}

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

func (s *RedisStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	key := s.key(id)
	data, err := s.client.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, domain.ErrSessionExpired
		}

		return nil, fmt.Errorf("failed to get session from redis: %w", err)
	}

	var session domain.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

func (s *RedisStore) Delete(ctx context.Context, id string) error {
	key := s.key(id)
	err := s.client.Client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete session from redis: %w", err)
	}

	return nil
}

func (s *RedisStore) Touch(ctx context.Context, id string, ttl time.Duration) error {
	key := s.key(id)
	cmd := s.client.Client.Expire(ctx, key, ttl)
	err := cmd.Err()
	if err != nil {
		return fmt.Errorf("failed to extend session ttl: %w", err)
	}
	if !cmd.Val() {
		return domain.ErrSessionExpired
	}

	return nil
}

func (s *RedisStore) key(id string) string {
	return fmt.Sprintf("session:%s", id)
}
