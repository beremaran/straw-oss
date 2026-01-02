package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type ApiKeyHandler struct {
	repo domain.ApiKeyRepository
}

func NewApiKeyHandler(repo domain.ApiKeyRepository) *ApiKeyHandler {
	return &ApiKeyHandler{repo: repo}
}

// HandleListApiKeys lists all API keys with pagination
func (h *ApiKeyHandler) HandleListApiKeys(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	keys, total, err := h.repo.List(c.Request().Context(), limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list api keys"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":  keys,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// HandleCreateApiKey creates a new API key
func (h *ApiKeyHandler) HandleCreateApiKey(c echo.Context) error {
	type createRequest struct {
		Name              string   `json:"name"`
		Scopes            []string `json:"scopes"`
		RateLimitOverride *int     `json:"rate_limit_override"`
	}

	var req createRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	// Generate a random API key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate key"})
	}
	rawKey := hex.EncodeToString(keyBytes)

	// Hash the key using bcrypt
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to hash key"})
	}

	apiKey := domain.NewApiKey(uuid.New().String(), string(hashBytes), req.Name, req.Scopes)
	apiKey.RateLimitOverride = req.RateLimitOverride

	if err := h.repo.Create(c.Request().Context(), apiKey); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create api key"})
	}

	// Return the raw key ONLY ONCE
	type createResponse struct {
		*domain.ApiKey
		RawKey string `json:"raw_key"`
	}

	return c.JSON(http.StatusCreated, createResponse{
		ApiKey: apiKey,
		RawKey: rawKey,
	})
}

// HandleRevokeApiKey revokes an API key
func (h *ApiKeyHandler) HandleRevokeApiKey(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	if err := h.repo.Revoke(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke api key"})
	}

	return c.NoContent(http.StatusNoContent)
}
