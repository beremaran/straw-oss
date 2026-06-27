package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/google/uuid"
)

type ApiKeyHandler struct {
	repo domain.ApiKeyRepository
}

func NewApiKeyHandler(repo domain.ApiKeyRepository) *ApiKeyHandler {
	return &ApiKeyHandler{repo: repo}
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
	if err := helper.ReadJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		helper.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	rawKey := hex.EncodeToString(keyBytes)

	hash := sha256.Sum256([]byte(rawKey))
	tokenHash := hex.EncodeToString(hash[:])

	apiKey := domain.NewApiKey(uuid.New().String(), tokenHash, req.Name, req.Scopes)
	apiKey.RateLimitOverride = req.RateLimitOverride

	if err := h.repo.Create(r.Context(), apiKey); err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}

	apiKeyResp := dto.FromApiKey(apiKey)
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

	if err := h.repo.Revoke(r.Context(), id); err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
