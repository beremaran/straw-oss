package dto

import "time"

type CreateApiKeyRequest struct {
	Name string `json:"name" validate:"required"`

	Scopes []string `json:"scopes"`

	RateLimitOverride *int `json:"rate_limit_override,omitempty"`
}

type ApiKeyResponse struct {
	ID string `json:"id"`

	Name string `json:"name"`

	Scopes []string `json:"scopes"`

	RateLimitOverride *int `json:"rate_limit_override,omitempty"`

	IsActive bool `json:"is_active"`

	CreatedAt time.Time `json:"created_at"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreateApiKeyResponse struct {
	ApiKeyResponse

	RawKey string `json:"raw_key"`
}

type ListApiKeysResponse = PaginatedResponse[ApiKeyResponse]
