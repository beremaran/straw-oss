package session_test

import (
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

func TestService_CreateSession(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, err := svc.CreateSession(ctx, "ep1", "rule1", []string{"tag1"})
	assert.NoError(t, err)
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "ep1", sess.EndpointID)
}

func TestService_GetSession(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, err := svc.CreateSession(ctx, "ep1", "rule1", nil)
	require.NoError(t, err)

	got, err := svc.GetSession(ctx, sess.ID)
	assert.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)

	sess.LastUsedAt = time.Now().Add(-20 * time.Minute)
	err = store.Save(ctx, sess, time.Minute)
	require.NoError(t, err)

	_, err = svc.GetSession(ctx, sess.ID)
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_MigrateSession(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	updated, err := svc.MigrateSession(ctx, sess.ID, "ep2")
	assert.NoError(t, err)
	assert.Equal(t, "ep2", updated.EndpointID)
	assert.Equal(t, 1, updated.MigrationCount)

	updated, err = svc.MigrateSession(ctx, sess.ID, "ep3")
	assert.NoError(t, err)
	assert.Equal(t, 2, updated.MigrationCount)

	updated, err = svc.MigrateSession(ctx, sess.ID, "ep4")
	assert.NoError(t, err)
	assert.Equal(t, 3, updated.MigrationCount)

	_, err = svc.MigrateSession(ctx, sess.ID, "ep5")
	assert.ErrorIs(t, err, domain.ErrSessionMigrationLimit)
}

func TestService_CreateSession_StoreError(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	mr.Close()

	_, err = svc.CreateSession(ctx, "ep1", "rule1", []string{"tag1"})
	assert.Error(t, err)
}

func TestService_GetSession_StoreError(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	mr.Close()

	_, err = svc.GetSession(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestService_GetSession_NonExistent(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	_, err = svc.GetSession(ctx, "nonexistent")
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_TouchSession(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	err = svc.TouchSession(ctx, sess.ID)
	assert.NoError(t, err)

	err = svc.TouchSession(ctx, "nonexistent")
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_TouchSession_StoreError(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	mr.Close()

	err = svc.TouchSession(ctx, sess.ID)
	assert.Error(t, err)
}

func TestService_EndSession(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	err = svc.EndSession(ctx, sess.ID)
	assert.NoError(t, err)

	_, err = svc.GetSession(ctx, sess.ID)
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_EndSession_StoreError(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	mr.Close()

	err = svc.EndSession(ctx, sess.ID)
	assert.Error(t, err)
}

func TestService_MigrateSession_NonExistent(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	_, err = svc.MigrateSession(ctx, "nonexistent", "ep2")
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_MigrateSession_StoreSaveError(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	mr.Close()

	_, err = svc.MigrateSession(ctx, sess.ID, "ep2")
	assert.Error(t, err)
}

func TestService_CreateSession_WithTags(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	tags := []string{"tag1", "tag2", "tag3"}
	sess, err := svc.CreateSession(ctx, "ep1", "rule1", tags)
	assert.NoError(t, err)
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "ep1", sess.EndpointID)
	assert.Equal(t, "rule1", sess.RuleID)
	assert.Equal(t, tags, sess.Tags)
	assert.Equal(t, 0, sess.MigrationCount)
	assert.Equal(t, 0, sess.RequestCount)
	assert.False(t, sess.CreatedAt.IsZero())
	assert.False(t, sess.LastUsedAt.IsZero())
}
