package auth

import (
	"context"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// Validator defines the interface for validating API keys.
type Validator interface {
	ValidateKey(ctx context.Context, rawKey string) (*domain.ApiKey, error)
}

// KeyCache defines the interface for API key caching.
// We might want to move this to domain or infra, but it's specific to auth service needs.
type KeyCache interface {
	GetKey(ctx context.Context, keyHash string) (*domain.ApiKey, error)
	SetKey(ctx context.Context, keyHash string, apiKey *domain.ApiKey) error
	InvalidateKey(ctx context.Context, keyHash string) error
}
