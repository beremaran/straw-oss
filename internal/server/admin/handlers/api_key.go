package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/google/uuid"
)

type ApiKeyHandler struct {
	repo      domain.ApiKeyRepository
	auditRepo domain.ManagementAuditRepository
}

func NewApiKeyHandler(repo domain.ApiKeyRepository, auditRepo domain.ManagementAuditRepository) *ApiKeyHandler {
	return &ApiKeyHandler{repo: repo, auditRepo: auditRepo}
}

func (h *ApiKeyHandler) HandleListApiKeys(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	keys, total, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list api keys")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListApiKeysResponse{
		Data:  dto.FromApiKeys(keys),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

func (h *ApiKeyHandler) HandleCreateApiKey(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateApiKeyRequest
	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.Name == "" {
		helper.WriteError(w, http.StatusBadRequest, "name is required")

		return
	}

	keyBytes := make([]byte, 32)
	_, err = rand.Read(keyBytes)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to generate key")

		return
	}
	rawKey := hex.EncodeToString(keyBytes)

	hash := sha256.Sum256([]byte(rawKey))
	tokenHash := hex.EncodeToString(hash[:])

	apiKey := domain.NewApiKey(uuid.New().String(), tokenHash, req.Name, req.Scopes)
	apiKey.RateLimitOverride = req.RateLimitOverride

	err = h.repo.Create(r.Context(), apiKey)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create api key")

		return
	}

	apiKeyResp := dto.FromApiKey(apiKey)
	
	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionCreate, "api_key", apiKey.ID, nil, apiKeyResp)
		_ = h.auditRepo.Create(r.Context(), event)
	}
	
	helper.WriteJSON(w, http.StatusCreated, dto.CreateApiKeyResponse{
		ApiKeyResponse: *apiKeyResp,
		RawKey:         rawKey,
	})
}

func (h *ApiKeyHandler) HandleRevokeApiKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	err := h.repo.Revoke(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to revoke api key")

		return
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionRevoke, "api_key", id, nil, nil)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	w.WriteHeader(http.StatusNoContent)
}
