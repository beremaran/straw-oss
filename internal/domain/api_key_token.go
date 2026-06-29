package domain

import (
	"context"
	"time"
)

// TokenStatus represents the lifecycle status of an API key token.
type TokenStatus string

const (
	// TokenStatusActive indicates the token is valid and in use.
	TokenStatusActive TokenStatus = "active"
	// TokenStatusGrace indicates the token is in a grace period (e.g. during rotation).
	TokenStatusGrace TokenStatus = "grace"
	// TokenStatusRevoked indicates the token has been revoked.
	TokenStatusRevoked TokenStatus = "revoked"
)

// APIKeyToken represents a token derived from an APIKey, supporting rotation.
type APIKeyToken struct {
	ID        string      `json:"id"`
	APIKeyID  string      `json:"api_key_id"`
	TokenHash string      `json:"token_hash"`
	Status    TokenStatus `json:"status"`
	ExpiresAt *time.Time  `json:"expires_at,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// NewAPIKeyToken creates a new active APIKeyToken with the given id, API key ID, and token hash.
func NewAPIKeyToken(id, apiKeyID, tokenHash string) *APIKeyToken {
	return &APIKeyToken{
		ID:        id,
		APIKeyID:  apiKeyID,
		TokenHash: tokenHash,
		Status:    TokenStatusActive,
		CreatedAt: time.Now(),
	}
}

// IsValid reports whether the APIKeyToken is active and not expired.
func (t *APIKeyToken) IsValid() bool {
	if t.Status == TokenStatusRevoked {
		return false
	}

	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return false
	}

	return true
}

// APIKeyTokenRepository provides persistence operations for APIKeyToken entities.
type APIKeyTokenRepository interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (*APIKeyToken, error)
	Create(ctx context.Context, token *APIKeyToken) error
	ListByAPIKeyID(ctx context.Context, apiKeyID string) ([]APIKeyToken, error)
	Rotate(ctx context.Context, apiKeyID string, token *APIKeyToken, graceUntil *time.Time, revokeExisting bool) error
	UpdateStatus(ctx context.Context, id string, status TokenStatus) error
}
