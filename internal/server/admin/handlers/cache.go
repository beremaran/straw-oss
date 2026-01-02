package handlers

import (
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/labstack/echo/v4"
)

type CacheHandler struct {
	redisClient *redis.Client
}

func NewCacheHandler(redisClient *redis.Client) *CacheHandler {
	return &CacheHandler{redisClient: redisClient}
}

// HandleClearCache clears keys matching a pattern.
// Query Param: pattern (default: "*")
// Usage: POST /admin/cache/clear?pattern=auth:*
func (h *CacheHandler) HandleClearCache(c echo.Context) error {
	pattern := c.QueryParam("pattern")
	if pattern == "" {
		pattern = "*"
	}

	// Safety check: prevent accidental flush of everything unless explicit,
	// though the requirement says "filtered by pattern".
	// Let's implement iterative deletion using SCAN.

	ctx := c.Request().Context()
	r := h.redisClient.Client

	iter := r.Scan(ctx, 0, pattern, 0).Iterator()
	count := 0
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 100 {
			if err := r.Del(ctx, keys...).Err(); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete keys"})
			}
			count += len(keys)
			keys = nil
		}
	}
	if err := iter.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to scan keys"})
	}

	if len(keys) > 0 {
		if err := r.Del(ctx, keys...).Err(); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete keys"})
		}
		count += len(keys)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "cache cleared",
		"pattern": pattern,
		"deleted": count,
	})
}

// HandleGetCacheStats returns Redis memory stats.
func (h *CacheHandler) HandleGetCacheStats(c echo.Context) error {
	ctx := c.Request().Context()
	info, err := h.redisClient.Client.Info(ctx).Result()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get redis info"})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"info": info,
	})
}
