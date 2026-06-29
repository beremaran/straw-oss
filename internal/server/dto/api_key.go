package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	errRateLimitOverrideType = errors.New("rate_limit_override must be an integer or null")
	errExpiresAtType         = errors.New("expires_at must be an RFC3339 string or null")
	errExpiresAtFormat       = errors.New("expires_at must be a valid RFC3339 timestamp")
)

type CreateApiKeyRequest struct {
	Name              string   `json:"name"                          validate:"required"`
	Scopes            []string `json:"scopes"`
	RateLimitOverride *int     `json:"rate_limit_override,omitempty"`
}

type UpdateApiKeyRequest struct {
	Name                 *string    `json:"-"`
	Scopes               *[]string  `json:"-"`
	RateLimitOverride    *int       `json:"-"`
	RateLimitOverrideSet bool       `json:"-"`
	ExpiresAt            *time.Time `json:"-"`
	ExpiresAtSet         bool       `json:"-"`
	IsActive             *bool      `json:"-"`
}

func (r *UpdateApiKeyRequest) UnmarshalJSON(data []byte) error {
	type rawUpdateApiKeyRequest struct {
		Name              *string          `json:"name"`
		Scopes            *[]string        `json:"scopes"`
		RateLimitOverride *json.RawMessage `json:"rate_limit_override"`
		ExpiresAt         *json.RawMessage `json:"expires_at"`
		IsActive          *bool            `json:"is_active"`
	}

	var raw rawUpdateApiKeyRequest
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
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

type RotateApiKeyRequest struct {
	GracePeriod    string `json:"grace_period,omitempty"`
	RevokeExisting bool   `json:"revoke_existing,omitempty"`
}

type ApiKeyResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Scopes            []string   `json:"scopes"`
	RateLimitOverride *int       `json:"rate_limit_override,omitempty"`
	IsActive          bool       `json:"is_active"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

type CreateApiKeyResponse struct {
	ApiKeyResponse
	RawKey string `json:"raw_key"`
}

type ApiKeyTokenResponse struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type ApiKeyDetailResponse struct {
	ApiKeyResponse
	Tokens []ApiKeyTokenResponse `json:"tokens"`
}

type RotateApiKeyResponse struct {
	APIKeyID                 string     `json:"api_key_id"`
	RawKey                   string     `json:"raw_key"`
	TokenID                  string     `json:"token_id"`
	PreviousTokensGraceUntil *time.Time `json:"previous_tokens_grace_until,omitempty"`
}

type ListApiKeysResponse = PaginatedResponse[ApiKeyResponse]
