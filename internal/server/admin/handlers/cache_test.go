package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/redis"
)

func TestCacheHandler_HandleClearCache(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)

	h := NewCacheHandler(client, nil)

	client.Client.Set(context.Background(), "test:1", "val1", time.Minute)
	client.Client.Set(context.Background(), "other:1", "val2", time.Minute)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/cache/clear?pattern=test:*", nil)
	rec := httptest.NewRecorder()

	h.HandleClearCache(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.InEpsilon(t, float64(1), resp["deleted"], 0.01)

	exists, _ := client.Client.Exists(req.Context(), "test:1").Result()
	assert.Equal(t, int64(0), exists)
	exists, _ = client.Client.Exists(req.Context(), "other:1").Result()
	assert.Equal(t, int64(1), exists)
}

func TestCacheHandler_HandleGetCacheStats(t *testing.T) {
	ctx := t.Context()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)

	h := NewCacheHandler(client, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/cache/stats", nil)
	rec := httptest.NewRecorder()

	h.HandleGetCacheStats(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp["info"])
}
