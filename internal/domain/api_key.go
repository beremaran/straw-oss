package domain

import (
	"context"
	"strings"
	"time"
)

type ApiKey struct {
	ID string `json:"id"`

	TokenHash string `json:"token_hash"`

	Name string `json:"name"`

	Scopes []string `json:"scopes"`

	RateLimitOverride *int `json:"rate_limit_override,omitempty"`

	IsActive bool `json:"is_active"`

	CreatedAt time.Time `json:"created_at"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (k *ApiKey) IsValid() bool {
	if !k.IsActive {
		return false
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return false
	}
	return true
}

func (k *ApiKey) HasScope(tag Tag) bool {
	tagStr := tag.String()

	for _, scope := range k.Scopes {
		if matchScope(scope, tagStr) {
			return true
		}
	}
	return false
}

func (k *ApiKey) HasScopeForTags(tags []Tag) bool {
	for _, tag := range tags {
		if !k.HasScope(tag) {
			return false
		}
	}
	return true
}

func matchScope(scope, tag string) bool {

	if scope == "*" {
		return true
	}

	if scope == tag {
		return true
	}

	if strings.HasSuffix(scope, ":*") {
		prefix := strings.TrimSuffix(scope, "*")
		return strings.HasPrefix(tag, prefix)
	}

	if strings.HasPrefix(scope, "*:") {
		suffix := strings.TrimPrefix(scope, "*")
		return strings.HasSuffix(tag, suffix)
	}

	return false
}

func NewApiKey(id, tokenHash, name string, scopes []string) *ApiKey {
	return &ApiKey{
		ID:        id,
		TokenHash: tokenHash,
		Name:      name,
		Scopes:    scopes,
		IsActive:  true,
		CreatedAt: time.Now(),
	}
}

type ApiKeyRepository interface {
	GetByID(ctx context.Context, id string) (*ApiKey, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*ApiKey, error)
	Create(ctx context.Context, key *ApiKey) error
	List(ctx context.Context, limit, offset int) ([]ApiKey, int, error)
	Exists(ctx context.Context) (bool, error)
	Revoke(ctx context.Context, id string) error
}
