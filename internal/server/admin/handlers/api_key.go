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
//
//	@Summary		List API Keys
//	@Description	Returns paginated list of all API keys (without raw key values)
//	@Tags			api-keys
//	@Accept			json
//	@Produce		json
//	@Param			page	query		int	false	"Page number (default: 1)"
//	@Param			limit	query		int	false	"Items per page (default: 20, max: 100)"
//	@Success		200		{object}	ListApiKeysResponse	"Paginated list of API keys"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/api-keys [get]
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

	return c.JSON(http.StatusOK, ListApiKeysResponse{
		Data:  keys,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleCreateApiKey creates a new API key
//
//	@Summary		Create API Key
//	@Description	Creates a new API key. The raw key is returned only once in the response.
//	@Tags			api-keys
//	@Accept			json
//	@Produce		json
//	@Param			request	body		object{name=string,scopes=[]string,rate_limit_override=int}	true	"API key creation request"
//	@Success		201		{object}	CreateApiKeyResponse	"Created API key with raw key"
//	@Failure		400		{object}	map[string]string	"Invalid request"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/api-keys [post]
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
	return c.JSON(http.StatusCreated, CreateApiKeyResponse{
		ApiKey: apiKey,
		RawKey: rawKey,
	})
}

// HandleRevokeApiKey revokes an API key
//
//	@Summary		Revoke API Key
//	@Description	Revokes an API key by ID, preventing further use
//	@Tags			api-keys
//	@Param			id	path		string	true	"API Key ID"
//	@Success		204	"API key revoked successfully"
//	@Failure		400	{object}	map[string]string	"ID required"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/api-keys/{id} [delete]
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
