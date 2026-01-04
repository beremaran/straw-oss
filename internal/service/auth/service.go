package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

var (
	ErrInvalidKey = errors.New("invalid api key")
)

// Service handles authentication logic.
type Service struct {
	repo  domain.ApiKeyRepository
	cache KeyCache
}

// NewAuthService creates a new AuthService.
func NewAuthService(repo domain.ApiKeyRepository, cache KeyCache) *Service {
	return &Service{
		repo:  repo,
		cache: cache,
	}
}

// ValidateKey validates a raw Bearer token.
// It checks the cache first, then the database.
// Expected format: Plain token (UUID recommended)
func (s *Service) ValidateKey(ctx context.Context, rawToken string) (*domain.ApiKey, error) {
	// 1. Hash the token
	tokenHash := sha256Hash(rawToken)

	// 2. Check Cache
	if cachedKey, err := s.cache.GetKey(ctx, tokenHash); err == nil && cachedKey != nil {
		if cachedKey.IsValid() {
			return cachedKey, nil
		}
		// If cached key is invalid (expired/inactive), re-check DB
	}

	// 3. Lookup in DB by token hash
	apiKey, err := s.repo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, ErrInvalidKey
	}

	// 4. Check if active/expired
	if !apiKey.IsValid() {
		return nil, ErrInvalidKey
	}

	// 5. Cache success
	if err := s.cache.SetKey(ctx, tokenHash, apiKey); err != nil {
		// Log error but don't fail request
		// TODO: Logger
	}

	return apiKey, nil
}

// InvalidateKey removes a cached API key by its raw token hash.
// This should be called when a key is revoked or its status changes.
func (s *Service) InvalidateKey(ctx context.Context, rawToken string) error {
	tokenHash := sha256Hash(rawToken)
	return s.cache.InvalidateKey(ctx, tokenHash)
}

// InvalidateKeyByID removes cached entries for a specific API key ID.
// This is useful when a key is revoked via the admin API.
// Note: Since we cache by the hash of the raw token, we can't directly invalidate by ID.
// The caller should invalidate by raw token if available, or clear all cache entries.
func (s *Service) InvalidateKeyByID(ctx context.Context, keyID string) error {
	// Since we cache by hash of raw token, we can't directly invalidate by ID.
	// This is a limitation of the current caching strategy.
	// In a production system, we might want to maintain a mapping from ID to hashes.
	return nil
}

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}
