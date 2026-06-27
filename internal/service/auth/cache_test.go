package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() {
		mr.Close()
	})

	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})
	return &redis.Client{Client: rdb}, mr
}

func TestAuthCache_KeyOperations(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	keyHash := "some-hash"
	apiKey := &domain.ApiKey{
		ID:        "test-id",
		TokenHash: "test-hash",
		Scopes:    []string{"read"},
	}

	got, err := cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.Nil(t, got)

	err = cache.SetKey(ctx, keyHash, apiKey)
	assert.NoError(t, err)

	got, err = cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, apiKey.ID, got.ID)
	assert.Equal(t, apiKey.Scopes, got.Scopes)
}

func TestAuthCache_TTLExpiration(t *testing.T) {
	client, mr := newTestRedis(t)
	cache := NewAuthCache(client, 100*time.Millisecond)
	ctx := context.Background()

	keyHash := "ttl-hash"
	apiKey := &domain.ApiKey{
		ID:        "test-id",
		TokenHash: "test-hash",
		Scopes:    []string{"read"},
	}

	err := cache.SetKey(ctx, keyHash, apiKey)
	assert.NoError(t, err)

	got, err := cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.NotNil(t, got)

	mr.FastForward(150 * time.Millisecond)

	got, err = cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestAuthCache_InvalidateKey(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	keyHash := "invalidate-hash"
	apiKey := &domain.ApiKey{
		ID:        "test-id",
		TokenHash: "test-hash",
		Scopes:    []string{"read"},
	}

	err := cache.SetKey(ctx, keyHash, apiKey)
	assert.NoError(t, err)

	got, err := cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.NotNil(t, got)

	err = cache.InvalidateKey(ctx, keyHash)
	assert.NoError(t, err)

	got, err = cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestAuthCache_InvalidateNonExistentKey(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	err := cache.InvalidateKey(ctx, "nonexistent-hash")
	assert.NoError(t, err)
}

func TestAuthCache_OverwriteExistingKey(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	keyHash := "overwrite-hash"
	oldKey := &domain.ApiKey{
		ID:        "old-id",
		TokenHash: "old-hash",
		Scopes:    []string{"read"},
	}
	newKey := &domain.ApiKey{
		ID:        "new-id",
		TokenHash: "new-hash",
		Scopes:    []string{"write"},
	}

	err := cache.SetKey(ctx, keyHash, oldKey)
	assert.NoError(t, err)

	got, err := cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.Equal(t, "old-id", got.ID)

	err = cache.SetKey(ctx, keyHash, newKey)
	assert.NoError(t, err)

	got, err = cache.GetKey(ctx, keyHash)
	assert.NoError(t, err)
	assert.Equal(t, "new-id", got.ID)
	assert.Equal(t, []string{"write"}, got.Scopes)
}

func TestAuthCache_MultipleKeys(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	keys := map[string]*domain.ApiKey{
		"hash1": {ID: "key1", TokenHash: "hash1", Scopes: []string{"read"}},
		"hash2": {ID: "key2", TokenHash: "hash2", Scopes: []string{"write"}},
		"hash3": {ID: "key3", TokenHash: "hash3", Scopes: []string{"admin"}},
	}

	for hash, key := range keys {
		err := cache.SetKey(ctx, hash, key)
		assert.NoError(t, err)
	}

	for hash, expectedKey := range keys {
		got, err := cache.GetKey(ctx, hash)
		assert.NoError(t, err)
		assert.NotNil(t, got)
		assert.Equal(t, expectedKey.ID, got.ID)
		assert.Equal(t, expectedKey.Scopes, got.Scopes)
	}

	err := cache.InvalidateKey(ctx, "hash2")
	assert.NoError(t, err)

	got, err := cache.GetKey(ctx, "hash2")
	assert.NoError(t, err)
	assert.Nil(t, got)

	got, err = cache.GetKey(ctx, "hash1")
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, "key1", got.ID)
}

func TestAuthCache_GetKey_Error(t *testing.T) {
	client, mr := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	mr.Close()

	_, err := cache.GetKey(ctx, "some-hash")
	assert.Error(t, err)
}
