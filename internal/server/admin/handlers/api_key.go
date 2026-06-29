package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/google/uuid"
)

type apiKeyInvalidator interface {
	InvalidateKeyByID(ctx context.Context, keyID string) error
}

var (
	errNilAPIKeyResponse      = errors.New("api key response is nil")
	errAPIKeyNameEmpty        = errors.New("name cannot be empty")
	errRateLimitOverrideRange = errors.New("rate_limit_override must be greater than or equal to 0")
	errGracePeriodInvalid     = errors.New("grace_period must be a valid duration")
	errGracePeriodNonPositive = errors.New("grace_period must be greater than zero")
)

type ApiKeyHandler struct {
	repo        domain.ApiKeyRepository
	tokenRepo   domain.ApiKeyTokenRepository
	auditRepo   domain.ManagementAuditRepository
	invalidator apiKeyInvalidator
}

func NewApiKeyHandler(repo domain.ApiKeyRepository, tokenRepo domain.ApiKeyTokenRepository, auditRepo domain.ManagementAuditRepository, invalidator apiKeyInvalidator) *ApiKeyHandler {
	return &ApiKeyHandler{
		repo:        repo,
		tokenRepo:   tokenRepo,
		auditRepo:   auditRepo,
		invalidator: invalidator,
	}
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

func (h *ApiKeyHandler) HandleGetApiKey(w http.ResponseWriter, r *http.Request) {
	key := h.loadAPIKey(w, r)
	if key == nil {
		return
	}

	detail, err := h.apiKeyDetail(r.Context(), key)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch api key tokens")

		return
	}

	helper.WriteJSON(w, http.StatusOK, detail)
}

func (h *ApiKeyHandler) HandleCreateApiKey(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateApiKeyRequest
	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		helper.WriteError(w, http.StatusBadRequest, "name is required")

		return
	}
	err = validateAPIKeyScopes(req.Scopes)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}
	err = validateRateLimitOverride(req.RateLimitOverride)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	rawKey, tokenHash, err := generateAPIKeySecret()
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to generate key")

		return
	}

	apiKey := domain.NewApiKey(uuid.New().String(), tokenHash, name, req.Scopes)
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

func (h *ApiKeyHandler) HandleUpdateApiKey(w http.ResponseWriter, r *http.Request) {
	key := h.loadAPIKey(w, r)
	if key == nil {
		return
	}

	var req dto.UpdateApiKeyRequest
	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	oldKey := *key
	err = applyAPIKeyUpdate(key, req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	err = h.repo.Update(r.Context(), key)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update api key")

		return
	}

	h.invalidateAPIKey(r.Context(), key.ID)

	detail, err := h.apiKeyDetail(r.Context(), key)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch updated api key")

		return
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionUpdate, "api_key", key.ID, dto.FromApiKey(&oldKey), detail)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, detail)
}

func (h *ApiKeyHandler) HandleRotateApiKey(w http.ResponseWriter, r *http.Request) {
	key := h.loadAPIKey(w, r)
	if key == nil {
		return
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		helper.WriteError(w, http.StatusBadRequest, "api key is expired")

		return
	}

	var req dto.RotateApiKeyRequest
	err := readOptionalJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	graceUntil, err := parseGracePeriod(req.GracePeriod)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	rawKey, tokenHash, err := generateAPIKeySecret()
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to generate key")

		return
	}

	token := domain.NewApiKeyToken(uuid.New().String(), key.ID, tokenHash)
	err = h.tokenRepo.Rotate(r.Context(), key.ID, token, graceUntil, req.RevokeExisting)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to rotate api key")

		return
	}

	h.invalidateAPIKey(r.Context(), key.ID)

	response := dto.RotateApiKeyResponse{
		APIKeyID:                 key.ID,
		RawKey:                   rawKey,
		TokenID:                  token.ID,
		PreviousTokensGraceUntil: graceUntil,
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionRotate, "api_key", key.ID, nil, map[string]interface{}{
			"api_key_id":                  key.ID,
			"token_id":                    token.ID,
			"previous_tokens_grace_until": graceUntil,
			"revoke_existing":             req.RevokeExisting,
		})
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, response)
}

func (h *ApiKeyHandler) HandleReactivateApiKey(w http.ResponseWriter, r *http.Request) {
	key := h.loadAPIKey(w, r)
	if key == nil {
		return
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		helper.WriteError(w, http.StatusBadRequest, "expired api key cannot be reactivated")

		return
	}

	if key.IsActive {
		detail, err := h.apiKeyDetail(r.Context(), key)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to fetch api key")

			return
		}

		helper.WriteJSON(w, http.StatusOK, detail)

		return
	}

	oldKey := *key
	key.IsActive = true
	err := h.repo.Update(r.Context(), key)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to reactivate api key")

		return
	}

	h.invalidateAPIKey(r.Context(), key.ID)

	detail, err := h.apiKeyDetail(r.Context(), key)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch reactivated api key")

		return
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionReactivate, "api_key", key.ID, dto.FromApiKey(&oldKey), detail)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, detail)
}

func (h *ApiKeyHandler) HandleRevokeApiKey(w http.ResponseWriter, r *http.Request) {
	key := h.loadAPIKey(w, r)
	if key == nil {
		return
	}

	err := h.repo.Revoke(r.Context(), key.ID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to revoke api key")

		return
	}

	h.invalidateAPIKey(r.Context(), key.ID)

	if h.auditRepo != nil {
		newValue := map[string]interface{}{"is_active": false}
		event := middleware.NewAuditEvent(r, domain.ActionRevoke, "api_key", key.ID, dto.FromApiKey(key), newValue)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ApiKeyHandler) apiKeyDetail(ctx context.Context, key *domain.ApiKey) (*dto.ApiKeyDetailResponse, error) {
	tokens, err := h.tokenRepo.ListByApiKeyID(ctx, key.ID)
	if err != nil {
		return nil, err
	}

	response := dto.FromApiKey(key)
	if response == nil {
		return nil, errNilAPIKeyResponse
	}

	return &dto.ApiKeyDetailResponse{
		ApiKeyResponse: *response,
		Tokens:         dto.FromApiKeyTokens(tokens),
	}, nil
}

func (h *ApiKeyHandler) invalidateAPIKey(ctx context.Context, keyID string) {
	if h.invalidator != nil {
		_ = h.invalidator.InvalidateKeyByID(ctx, keyID)
	}
}

func (h *ApiKeyHandler) loadAPIKey(w http.ResponseWriter, r *http.Request) *domain.ApiKey {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return nil
	}

	key, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch api key")

		return nil
	}
	if key == nil {
		helper.WriteError(w, http.StatusNotFound, "api key not found")

		return nil
	}

	return key
}

func applyAPIKeyUpdate(key *domain.ApiKey, req dto.UpdateApiKeyRequest) error {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return errAPIKeyNameEmpty
		}
		key.Name = name
	}

	if req.Scopes != nil {
		err := validateAPIKeyScopes(*req.Scopes)
		if err != nil {
			return err
		}
		key.Scopes = *req.Scopes
	}

	if req.RateLimitOverrideSet {
		err := validateRateLimitOverride(req.RateLimitOverride)
		if err != nil {
			return err
		}
		key.RateLimitOverride = req.RateLimitOverride
	}

	if req.ExpiresAtSet {
		key.ExpiresAt = req.ExpiresAt
	}

	if req.IsActive != nil {
		key.IsActive = *req.IsActive
	}

	return nil
}

func validateAPIKeyScopes(scopes []string) error {
	_, err := domain.StringsToTags(scopes)
	if err != nil {
		return fmt.Errorf("invalid scopes: %w", err)
	}

	return nil
}

func validateRateLimitOverride(rateLimit *int) error {
	if rateLimit != nil && *rateLimit < 0 {
		return errRateLimitOverrideRange
	}

	return nil
}

func parseGracePeriod(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return nil, errGracePeriodInvalid
	}
	if duration <= 0 {
		return nil, errGracePeriodNonPositive
	}

	graceUntil := time.Now().UTC().Add(duration)

	return &graceUntil, nil
}

func generateAPIKeySecret() (string, string, error) {
	keyBytes := make([]byte, 32)
	_, err := rand.Read(keyBytes)
	if err != nil {
		return "", "", err
	}

	rawKey := hex.EncodeToString(keyBytes)
	hash := sha256.Sum256([]byte(rawKey))
	tokenHash := hex.EncodeToString(hash[:])

	return rawKey, tokenHash, nil
}

func readOptionalJSON(r *http.Request, v interface{}) error {
	err := helper.ReadJSON(r, v)
	if errors.Is(err, io.EOF) {
		return nil
	}

	return err
}
