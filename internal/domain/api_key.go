package domain

import (
	"context"
	"strings"
	"time"
)

// ApiKey represents an API key for client authentication.
type ApiKey struct {
	// ID is the unique identifier for this API key.
	ID string `json:"id"`

	// TokenHash is the SHA256 hash of the Bearer token.
	// The actual token is only shown once during creation.
	TokenHash string `json:"token_hash"`

	// Name is a human-readable name for the key.
	Name string `json:"name"`

	// Scopes define which tag patterns this key can access.
	// Supports wildcards, e.g., ["target:*", "type:search"]
	Scopes []string `json:"scopes"`

	// RateLimitOverride allows per-key rate limit customization.
	// If set, overrides the routing rule's rate limit.
	RateLimitOverride *int `json:"rate_limit_override,omitempty"`

	// IsActive indicates whether this key is currently active.
	IsActive bool `json:"is_active"`

	// CreatedAt is when the key was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when the key expires (optional).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IsValid checks if the API key is currently valid.
// A key is valid if it's active and not expired.
func (k *ApiKey) IsValid() bool {
	if !k.IsActive {
		return false
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return false
	}
	return true
}

// HasScope checks if the API key has permission to access a given tag.
// Supports wildcard matching where scope pattern "target:*" matches tag "target:amazon".
func (k *ApiKey) HasScope(tag Tag) bool {
	tagStr := tag.String()

	for _, scope := range k.Scopes {
		if matchScope(scope, tagStr) {
			return true
		}
	}
	return false
}

// HasScopeForTags checks if the API key has permission for all given tags.
func (k *ApiKey) HasScopeForTags(tags []Tag) bool {
	for _, tag := range tags {
		if !k.HasScope(tag) {
			return false
		}
	}
	return true
}

// matchScope checks if a scope pattern matches a tag string.
// Supports wildcards: "target:*" matches "target:anything"
// Supports full wildcards: "*" matches everything
func matchScope(scope, tag string) bool {
	// Full wildcard
	if scope == "*" {
		return true
	}

	// Exact match
	if scope == tag {
		return true
	}

	// Wildcard suffix match
	if strings.HasSuffix(scope, ":*") {
		prefix := strings.TrimSuffix(scope, "*")
		return strings.HasPrefix(tag, prefix)
	}

	// Wildcard prefix match (e.g., "*:search")
	if strings.HasPrefix(scope, "*:") {
		suffix := strings.TrimPrefix(scope, "*")
		return strings.HasSuffix(tag, suffix)
	}

	return false
}

// NewApiKey creates a new API key with the given parameters.
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

// ApiKeyRepository defines the interface for API key storage.
type ApiKeyRepository interface {
	GetByID(ctx context.Context, id string) (*ApiKey, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*ApiKey, error)
	Create(ctx context.Context, key *ApiKey) error
	List(ctx context.Context, limit, offset int) ([]ApiKey, int, error)
	Revoke(ctx context.Context, id string) error
}
