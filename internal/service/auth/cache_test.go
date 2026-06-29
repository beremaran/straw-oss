package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
)

const (
	testID     = "test-id"
	testHash   = "test-hash"
	scopeRead  = "read"
	scopeWrite = "write"
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
	apiKey := &domain.APIKey{
		ID:        testID,
		TokenHash: testHash,
		Scopes:    []string{scopeRead},
	}

	got, err := cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.Nil(t, got)

	err = cache.SetKey(ctx, keyHash, apiKey)
	require.NoError(t, err)

	got, err = cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, apiKey.ID, got.ID)
	assert.Equal(t, apiKey.Scopes, got.Scopes)
}

func TestAuthCache_TTLExpiration(t *testing.T) {
	client, mr := newTestRedis(t)
	cache := NewAuthCache(client, 100*time.Millisecond)
	ctx := context.Background()

	keyHash := "ttl-hash"
	apiKey := &domain.APIKey{
		ID:        testID,
		TokenHash: testHash,
		Scopes:    []string{scopeRead},
	}

	err := cache.SetKey(ctx, keyHash, apiKey)
	require.NoError(t, err)

	got, err := cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.NotNil(t, got)

	mr.FastForward(150 * time.Millisecond)

	got, err = cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAuthCache_InvalidateKey(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	keyHash := "invalidate-hash"
	apiKey := &domain.APIKey{
		ID:        testID,
		TokenHash: testHash,
		Scopes:    []string{scopeRead},
	}

	err := cache.SetKey(ctx, keyHash, apiKey)
	require.NoError(t, err)

	got, err := cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.NotNil(t, got)

	err = cache.InvalidateKey(ctx, keyHash)
	require.NoError(t, err)

	got, err = cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAuthCache_InvalidateNonExistentKey(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	err := cache.InvalidateKey(ctx, "nonexistent-hash")
	require.NoError(t, err)
}

func TestAuthCache_OverwriteExistingKey(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	keyHash := "overwrite-hash"
	oldKey := &domain.APIKey{
		ID:        "old-id",
		TokenHash: "old-hash",
		Scopes:    []string{scopeRead},
	}
	newKey := &domain.APIKey{
		ID:        "new-id",
		TokenHash: "new-hash",
		Scopes:    []string{scopeWrite},
	}

	err := cache.SetKey(ctx, keyHash, oldKey)
	require.NoError(t, err)

	got, err := cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.Equal(t, "old-id", got.ID)

	err = cache.SetKey(ctx, keyHash, newKey)
	require.NoError(t, err)

	got, err = cache.GetKey(ctx, keyHash)
	require.NoError(t, err)
	assert.Equal(t, "new-id", got.ID)
	assert.Equal(t, []string{scopeWrite}, got.Scopes)
}

func TestAuthCache_MultipleKeys(t *testing.T) {
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	keys := map[string]*domain.APIKey{
		"hash1": {ID: "key1", TokenHash: "hash1", Scopes: []string{scopeRead}},
		"hash2": {ID: "key2", TokenHash: "hash2", Scopes: []string{scopeWrite}},
		"hash3": {ID: "key3", TokenHash: "hash3", Scopes: []string{"admin"}},
	}

	for hash, key := range keys {
		err := cache.SetKey(ctx, hash, key)
		require.NoError(t, err)
	}

	for hash, expectedKey := range keys {
		got, err := cache.GetKey(ctx, hash)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Equal(t, expectedKey.ID, got.ID)
		assert.Equal(t, expectedKey.Scopes, got.Scopes)
	}

	err := cache.InvalidateKey(ctx, "hash2")
	require.NoError(t, err)

	got, err := cache.GetKey(ctx, "hash2")
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = cache.GetKey(ctx, "hash1")
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, "key1", got.ID)
}

func TestAuthCache_GetKey_Error(t *testing.T) {
	client, mr := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	ctx := context.Background()

	mr.Close()

	_, err := cache.GetKey(ctx, "some-hash")
	require.Error(t, err)
}
