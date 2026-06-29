package handlers

import (
	"net/http"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

type CacheHandler struct {
	redisClient *redis.Client
	auditRepo   domain.ManagementAuditRepository
}

func NewCacheHandler(redisClient *redis.Client, auditRepo domain.ManagementAuditRepository) *CacheHandler {
	return &CacheHandler{redisClient: redisClient, auditRepo: auditRepo}
}

func (h *CacheHandler) HandleClearCache(w http.ResponseWriter, r *http.Request) {
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		pattern = "*"
	}

	ctx := r.Context()
	red := h.redisClient.Client

	iter := red.Scan(ctx, 0, pattern, 0).Iterator()
	count := 0
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 100 {
			err := red.Del(ctx, keys...).Err()
			if err != nil {
				helper.WriteError(w, http.StatusInternalServerError, "failed to delete keys")

				return
			}
			count += len(keys)
			keys = nil
		}
	}
	err := iter.Err()
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to scan keys")

		return
	}

	if len(keys) > 0 {
		err := red.Del(ctx, keys...).Err()
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to delete keys")

			return
		}
		count += len(keys)
	}

	resp := dto.ClearCacheResponse{
		Message: "cache cleared",
		Pattern: pattern,
		Deleted: count,
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionPurge, "cache", pattern, nil, resp)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, resp)
}

func (h *CacheHandler) HandleGetCacheStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	info, err := h.redisClient.Client.Info(ctx).Result()
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get redis info")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.CacheStatsResponse{
		Info: info,
	})
}
