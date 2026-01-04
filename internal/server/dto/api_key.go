package dto

import "time"

// CreateApiKeyRequest is the request to create an API key.
//
//	@Description Request body for creating a new API key
type CreateApiKeyRequest struct {
	// Name is a human-readable name for the key (required)
	Name string `json:"name" validate:"required"`

	// Scopes define which tag patterns this key can access
	Scopes []string `json:"scopes"`

	// RateLimitOverride allows per-key rate limit customization
	RateLimitOverride *int `json:"rate_limit_override,omitempty"`
}

// ApiKeyResponse is the API response for an API key (without sensitive data).
//
//	@Description API key data returned by the API (excludes token hash)
type ApiKeyResponse struct {
	// ID is the unique identifier for this API key
	ID string `json:"id"`

	// Name is a human-readable name for the key
	Name string `json:"name"`

	// Scopes define which tag patterns this key can access
	Scopes []string `json:"scopes"`

	// RateLimitOverride allows per-key rate limit customization
	RateLimitOverride *int `json:"rate_limit_override,omitempty"`

	// IsActive indicates whether this key is currently active
	IsActive bool `json:"is_active"`

	// CreatedAt is when the key was created
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the key expires (optional)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateApiKeyResponse includes the raw key (shown only once).
//
//	@Description Response after creating an API key, includes raw key value
type CreateApiKeyResponse struct {
	ApiKeyResponse

	// RawKey is the plain-text API key (only shown once during creation)
	RawKey string `json:"raw_key"`
}

// ListApiKeysResponse is the paginated list of API keys
type ListApiKeysResponse = PaginatedResponse[ApiKeyResponse]
