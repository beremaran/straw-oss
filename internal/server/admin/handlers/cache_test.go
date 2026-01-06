package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestCacheHandler_HandleClearCache(t *testing.T) {
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(config.CoreConfig{RedisAddr: mr.Addr()}, nil)
	assert.NoError(t, err)

	h := NewCacheHandler(client)
	e := echo.New()

	// Seed data
	client.Client.Set(context.Background(), "test:1", "val1", time.Minute)
	client.Client.Set(context.Background(), "other:1", "val2", time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/admin/cache/clear?pattern=test:*", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.HandleClearCache(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["deleted"])

	// Verify
	exists, _ := client.Client.Exists(c.Request().Context(), "test:1").Result()
	assert.Equal(t, int64(0), exists)
	exists, _ = client.Client.Exists(c.Request().Context(), "other:1").Result()
	assert.Equal(t, int64(1), exists)
}

func TestCacheHandler_HandleGetCacheStats(t *testing.T) {
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(config.CoreConfig{RedisAddr: mr.Addr()}, nil)
	assert.NoError(t, err)

	h := NewCacheHandler(client)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/admin/cache/stats", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.HandleGetCacheStats(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp["info"])
}
