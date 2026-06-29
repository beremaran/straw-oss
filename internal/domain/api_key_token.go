package domain

import (
	"context"
	"time"
)

type TokenStatus string

const (
	TokenStatusActive  TokenStatus = "active"
	TokenStatusGrace   TokenStatus = "grace"
	TokenStatusRevoked TokenStatus = "revoked"
)

type ApiKeyToken struct {
	ID        string      `json:"id"`
	ApiKeyID  string      `json:"api_key_id"`
	TokenHash string      `json:"token_hash"`
	Status    TokenStatus `json:"status"`
	ExpiresAt *time.Time  `json:"expires_at,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

func NewApiKeyToken(id, apiKeyID, tokenHash string) *ApiKeyToken {
	return &ApiKeyToken{
		ID:        id,
		ApiKeyID:  apiKeyID,
		TokenHash: tokenHash,
		Status:    TokenStatusActive,
		CreatedAt: time.Now(),
	}
}

func (t *ApiKeyToken) IsValid() bool {
	if t.Status == TokenStatusRevoked {
		return false
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return false
	}

	return true
}

type ApiKeyTokenRepository interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (*ApiKeyToken, error)
	Create(ctx context.Context, token *ApiKeyToken) error
	ListByApiKeyID(ctx context.Context, apiKeyID string) ([]ApiKeyToken, error)
	Rotate(ctx context.Context, apiKeyID string, token *ApiKeyToken, graceUntil *time.Time, revokeExisting bool) error
	UpdateStatus(ctx context.Context, id string, status TokenStatus) error
}
