package handlers

import (
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/server/dto"
	"github.com/labstack/echo/v4"
)

type CacheHandler struct {
	redisClient *redis.Client
}

func NewCacheHandler(redisClient *redis.Client) *CacheHandler {
	return &CacheHandler{redisClient: redisClient}
}

// HandleClearCache clears keys matching a pattern.
//
//	@Summary		Clear Cache
//	@Description	Clears Redis cache keys matching the specified pattern
//	@Tags			cache
//	@Produce		json
//	@Param			pattern	query		string	false	"Key pattern to match (default: *)"
//	@Success		200		{object}	dto.ClearCacheResponse	"Deletion result"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/cache/clear [post]
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

	return c.JSON(http.StatusOK, dto.ClearCacheResponse{
		Message: "cache cleared",
		Pattern: pattern,
		Deleted: count,
	})
}

// HandleGetCacheStats returns Redis memory stats.
//
//	@Summary		Get Cache Stats
//	@Description	Returns Redis server information and memory statistics
//	@Tags			cache
//	@Produce		json
//	@Success		200	{object}	dto.CacheStatsResponse	"Redis info"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/cache/stats [get]
func (h *CacheHandler) HandleGetCacheStats(c echo.Context) error {
	ctx := c.Request().Context()
	info, err := h.redisClient.Client.Info(ctx).Result()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get redis info"})
	}

	return c.JSON(http.StatusOK, dto.CacheStatsResponse{
		Info: info,
	})
}
