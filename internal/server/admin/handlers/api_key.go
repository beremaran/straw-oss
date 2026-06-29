// Package handlers provides HTTP request handlers for the admin management API.
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

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
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

const apiKeyByteLength = 32

// APIKeyHandler manages API key CRUD operations.
type APIKeyHandler struct {
	repo        domain.APIKeyRepository
	tokenRepo   domain.APIKeyTokenRepository
	auditRepo   domain.ManagementAuditRepository
	invalidator apiKeyInvalidator
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(repo domain.APIKeyRepository, tokenRepo domain.APIKeyTokenRepository, auditRepo domain.ManagementAuditRepository, invalidator apiKeyInvalidator) *APIKeyHandler {
	return &APIKeyHandler{
		repo:        repo,
		tokenRepo:   tokenRepo,
		auditRepo:   auditRepo,
		invalidator: invalidator,
	}
}

// HandleListAPIKeys lists all API keys.
func (h *APIKeyHandler) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
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

	helper.WriteJSON(w, http.StatusOK, dto.ListAPIKeysResponse{
		Data:  dto.FromAPIKeys(keys),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleGetAPIKey retrieves a single API key with its tokens.
func (h *APIKeyHandler) HandleGetAPIKey(w http.ResponseWriter, r *http.Request) {
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

// HandleCreateAPIKey creates a new API key.
func (h *APIKeyHandler) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateAPIKeyRequest

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

	apiKey := domain.NewAPIKey(uuid.New().String(), tokenHash, name, req.Scopes)
	apiKey.RateLimitOverride = req.RateLimitOverride

	err = h.repo.Create(r.Context(), apiKey)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create api key")

		return
	}

	apiKeyResp := dto.FromAPIKey(apiKey)
	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionCreate, "api_key", apiKey.ID, nil, apiKeyResp)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusCreated, dto.CreateAPIKeyResponse{
		APIKeyResponse: *apiKeyResp,
		RawKey:         rawKey,
	})
}

// HandleUpdateAPIKey updates an existing API key.
func (h *APIKeyHandler) HandleUpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	key := h.loadAPIKey(w, r)
	if key == nil {
		return
	}

	var req dto.UpdateAPIKeyRequest

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
		event := middleware.NewAuditEvent(r, domain.ActionUpdate, "api_key", key.ID, dto.FromAPIKey(&oldKey), detail)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, detail)
}

// HandleRotateAPIKey rotates an API key's secret.
func (h *APIKeyHandler) HandleRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	key := h.loadAPIKey(w, r)
	if key == nil {
		return
	}

	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		helper.WriteError(w, http.StatusBadRequest, "api key is expired")

		return
	}

	var req dto.RotateAPIKeyRequest

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

	token := domain.NewAPIKeyToken(uuid.New().String(), key.ID, tokenHash)

	err = h.tokenRepo.Rotate(r.Context(), key.ID, token, graceUntil, req.RevokeExisting)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to rotate api key")

		return
	}

	h.invalidateAPIKey(r.Context(), key.ID)

	response := dto.RotateAPIKeyResponse{
		APIKeyID:                 key.ID,
		RawKey:                   rawKey,
		TokenID:                  token.ID,
		PreviousTokensGraceUntil: graceUntil,
	}

	h.logRotateAudit(r, key, token, graceUntil, req.RevokeExisting)

	helper.WriteJSON(w, http.StatusOK, response)
}

// HandleReactivateAPIKey reactivates a revoked or expired API key.
func (h *APIKeyHandler) HandleReactivateAPIKey(w http.ResponseWriter, r *http.Request) {
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
		event := middleware.NewAuditEvent(r, domain.ActionReactivate, "api_key", key.ID, dto.FromAPIKey(&oldKey), detail)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, detail)
}

// HandleRevokeAPIKey revokes an API key.
func (h *APIKeyHandler) HandleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
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
		newValue := map[string]any{"is_active": false}
		event := middleware.NewAuditEvent(r, domain.ActionRevoke, "api_key", key.ID, dto.FromAPIKey(key), newValue)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *APIKeyHandler) apiKeyDetail(ctx context.Context, key *domain.APIKey) (*dto.APIKeyDetailResponse, error) {
	tokens, err := h.tokenRepo.ListByAPIKeyID(ctx, key.ID)
	if err != nil {
		return nil, fmt.Errorf("listing api key tokens: %w", err)
	}

	response := dto.FromAPIKey(key)
	if response == nil {
		return nil, errNilAPIKeyResponse
	}

	return &dto.APIKeyDetailResponse{
		APIKeyResponse: *response,
		Tokens:         dto.FromAPIKeyTokens(tokens),
	}, nil
}

func (h *APIKeyHandler) invalidateAPIKey(ctx context.Context, keyID string) {
	if h.invalidator != nil {
		_ = h.invalidator.InvalidateKeyByID(ctx, keyID)
	}
}

func (h *APIKeyHandler) loadAPIKey(w http.ResponseWriter, r *http.Request) *domain.APIKey {
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

func applyAPIKeyUpdate(key *domain.APIKey, req dto.UpdateAPIKeyRequest) error {
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
	keyBytes := make([]byte, apiKeyByteLength)

	_, err := rand.Read(keyBytes)
	if err != nil {
		return "", "", fmt.Errorf("generating api key secret: %w", err)
	}

	rawKey := hex.EncodeToString(keyBytes)
	hash := sha256.Sum256([]byte(rawKey))
	tokenHash := hex.EncodeToString(hash[:])

	return rawKey, tokenHash, nil
}

func (h *APIKeyHandler) logRotateAudit(r *http.Request, key *domain.APIKey, token *domain.APIKeyToken, graceUntil *time.Time, revokeExisting bool) {
	if h.auditRepo == nil {
		return
	}

	event := middleware.NewAuditEvent(r, domain.ActionRotate, "api_key", key.ID, nil, map[string]any{
		"api_key_id":                  key.ID,
		"token_id":                    token.ID,
		"previous_tokens_grace_until": graceUntil,
		"revoke_existing":             revokeExisting,
	})
	_ = h.auditRepo.Create(r.Context(), event)
}

func readOptionalJSON(r *http.Request, v any) error {
	err := helper.ReadJSON(r, v)
	if errors.Is(err, io.EOF) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("reading optional json: %w", err)
	}

	return nil
}
