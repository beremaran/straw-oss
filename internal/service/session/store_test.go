package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/service/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisStore(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	t.Run("Save and Get", func(t *testing.T) {
		sess := domain.NewSession("123", "ep1", "rule1", []string{"tag1"})
		err := store.Save(ctx, sess, time.Minute)
		assert.NoError(t, err)

		retrieved, err := store.Get(ctx, "123")
		assert.NoError(t, err)
		assert.Equal(t, sess.ID, retrieved.ID)
		assert.Equal(t, sess.EndpointID, retrieved.EndpointID)
	})

	t.Run("Get Non-Existent", func(t *testing.T) {
		_, err := store.Get(ctx, "non-existent")
		assert.ErrorIs(t, err, domain.ErrSessionExpired)
	})

	t.Run("Delete", func(t *testing.T) {
		sess := domain.NewSession("456", "ep1", "rule1", []string{"tag1"})
		err := store.Save(ctx, sess, time.Minute)
		assert.NoError(t, err)

		err = store.Delete(ctx, "456")
		assert.NoError(t, err)

		_, err = store.Get(ctx, "456")
		assert.ErrorIs(t, err, domain.ErrSessionExpired)
	})

	t.Run("Touch", func(t *testing.T) {
		sess := domain.NewSession("789", "ep1", "rule1", []string{"tag1"})
		err := store.Save(ctx, sess, time.Second)
		assert.NoError(t, err)

		err = store.Touch(ctx, "789", time.Hour)
		assert.NoError(t, err)

		mr.FastForward(2 * time.Second)

		_, err = store.Get(ctx, "789")
		assert.NoError(t, err, "Session should still exist after touch")
	})
}

func TestRedisStore_SaveWithRedisError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	sess := domain.NewSession("123", "ep1", "rule1", []string{"tag1"})

	mr.Close()

	err = store.Save(ctx, sess, time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save session to redis")
}

func TestRedisStore_GetWithInvalidData(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	key := "session:invalid-json"
	err = client.Client.Set(ctx, key, []byte("invalid json"), time.Minute).Err()
	require.NoError(t, err)

	_, err = store.Get(ctx, "invalid-json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal session")
}

func TestRedisStore_GetWithRedisError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	mr.Close()

	_, err = store.Get(ctx, "123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get session from redis")
}

func TestRedisStore_DeleteWithRedisError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	mr.Close()

	err = store.Delete(ctx, "123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete session from redis")
}

func TestRedisStore_TouchWithRedisError(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	mr.Close()

	err = store.Touch(ctx, "123", time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to extend session ttl")
}

func TestRedisStore_TouchNonExistent(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	err = store.Touch(ctx, "non-existent", time.Minute)
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestRedisStore_SaveAllFields(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	sess := domain.NewSession("full-test", "ep1", "rule1", []string{"tag1", "tag2"})
	sess.MigrationCount = 2
	sess.RequestCount = 5

	err = store.Save(ctx, sess, time.Minute)
	assert.NoError(t, err)

	retrieved, err := store.Get(ctx, "full-test")
	assert.NoError(t, err)
	assert.Equal(t, sess.ID, retrieved.ID)
	assert.Equal(t, sess.EndpointID, retrieved.EndpointID)
	assert.Equal(t, sess.RuleID, retrieved.RuleID)
	assert.Equal(t, sess.Tags, retrieved.Tags)
	assert.Equal(t, sess.MigrationCount, retrieved.MigrationCount)
	assert.Equal(t, sess.RequestCount, retrieved.RequestCount)
	assert.False(t, retrieved.CreatedAt.IsZero())
	assert.False(t, retrieved.LastUsedAt.IsZero())
}

func TestRedisStore_SaveAndVerifyTTL(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	ctx := context.Background()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)

	sess := domain.NewSession("ttl-test", "ep1", "rule1", nil)
	ttl := 2 * time.Second

	err = store.Save(ctx, sess, ttl)
	assert.NoError(t, err)

	_, err = store.Get(ctx, "ttl-test")
	assert.NoError(t, err)

	mr.FastForward(ttl + time.Second)

	_, err = store.Get(ctx, "ttl-test")
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}
