// Package dto defines data transfer objects for the API server.
package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	errRateLimitOverrideType = errors.New("rate_limit_override must be an integer or null")
	errExpiresAtType         = errors.New("expires_at must be an RFC3339 string or null")
	errExpiresAtFormat       = errors.New("expires_at must be a valid RFC3339 timestamp")
)

// CreateAPIKeyRequest is the request body for creating a new API key.
type CreateAPIKeyRequest struct {
	Name              string   `json:"name"                          validate:"required"`
	Scopes            []string `json:"scopes"`
	RateLimitOverride *int     `json:"rate_limit_override,omitempty"`
}

// UpdateAPIKeyRequest is the request body for updating an existing API key.
type UpdateAPIKeyRequest struct {
	Name                 *string    `json:"-"`
	Scopes               *[]string  `json:"-"`
	RateLimitOverride    *int       `json:"-"`
	RateLimitOverrideSet bool       `json:"-"`
	ExpiresAt            *time.Time `json:"-"`
	ExpiresAtSet         bool       `json:"-"`
	IsActive             *bool      `json:"-"`
}

// UnmarshalJSON implements custom JSON deserialization for UpdateAPIKeyRequest.
func (r *UpdateAPIKeyRequest) UnmarshalJSON(data []byte) error {
	type rawUpdateAPIKeyRequest struct {
		Name              *string          `json:"name"`
		Scopes            *[]string        `json:"scopes"`
		RateLimitOverride *json.RawMessage `json:"rate_limit_override"`
		ExpiresAt         *json.RawMessage `json:"expires_at"`
		IsActive          *bool            `json:"is_active"`
	}

	var raw rawUpdateAPIKeyRequest

	err := json.Unmarshal(data, &raw)
	if err != nil {
		return fmt.Errorf("unmarshal update api key request: %w", err)
	}

	r.Name = raw.Name
	r.Scopes = raw.Scopes
	r.IsActive = raw.IsActive

	if raw.RateLimitOverride != nil {
		r.RateLimitOverrideSet = true

		rateLimit, err := parseOptionalRateLimit(*raw.RateLimitOverride)
		if err != nil {
			return err
		}

		r.RateLimitOverride = rateLimit
	}

	if raw.ExpiresAt != nil {
		r.ExpiresAtSet = true

		expiresAt, err := parseOptionalExpiresAt(*raw.ExpiresAt)
		if err != nil {
			return err
		}

		r.ExpiresAt = expiresAt
	}

	return nil
}

func parseOptionalRateLimit(raw json.RawMessage) (*int, error) {
	value := bytes.TrimSpace(raw)
	if bytes.Equal(value, []byte("null")) {
		return nil, nil
	}

	var rateLimit int

	err := json.Unmarshal(value, &rateLimit)
	if err != nil {
		return nil, errRateLimitOverrideType
	}

	return &rateLimit, nil
}

func parseOptionalExpiresAt(raw json.RawMessage) (*time.Time, error) {
	value := bytes.TrimSpace(raw)
	if bytes.Equal(value, []byte("null")) {
		return nil, nil
	}

	var expiresAt string

	err := json.Unmarshal(value, &expiresAt)
	if err != nil {
		return nil, errExpiresAtType
	}

	if strings.TrimSpace(expiresAt) == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, errExpiresAtFormat
	}

	return &parsed, nil
}

// RotateAPIKeyRequest is the request body for rotating an API key.
type RotateAPIKeyRequest struct {
	GracePeriod    string `json:"grace_period,omitempty"`
	RevokeExisting bool   `json:"revoke_existing,omitempty"`
}

// APIKeyResponse represents an API key in API responses.
type APIKeyResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Scopes            []string   `json:"scopes"`
	RateLimitOverride *int       `json:"rate_limit_override,omitempty"`
	IsActive          bool       `json:"is_active"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKeyResponse is the response body for creating an API key.
type CreateAPIKeyResponse struct {
	APIKeyResponse
	RawKey string `json:"raw_key"`
}

// APIKeyTokenResponse represents an API key token in API responses.
type APIKeyTokenResponse struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// APIKeyDetailResponse represents an API key with its tokens.
type APIKeyDetailResponse struct {
	APIKeyResponse
	Tokens []APIKeyTokenResponse `json:"tokens"`
}

// RotateAPIKeyResponse is the response body for rotating an API key.
type RotateAPIKeyResponse struct {
	APIKeyID                 string     `json:"api_key_id"`
	RawKey                   string     `json:"raw_key"`
	TokenID                  string     `json:"token_id"`
	PreviousTokensGraceUntil *time.Time `json:"previous_tokens_grace_until,omitempty"`
}

// ListAPIKeysResponse is a paginated list of API keys.
type ListAPIKeysResponse = PaginatedResponse[APIKeyResponse]
