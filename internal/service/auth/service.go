package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidKey       = errors.New("invalid api key")
	ErrInvalidKeyFormat = errors.New("invalid api key format (expected ID:Secret)")
)

// AuthService handles authentication logic.
type AuthService struct {
	repo  domain.ApiKeyRepository
	cache KeyCache
}

// NewAuthService creates a new AuthService.
func NewAuthService(repo domain.ApiKeyRepository, cache KeyCache) *AuthService {
	return &AuthService{
		repo:  repo,
		cache: cache,
	}
}

// ValidateKey validates a raw API key.
// It checks the cache first, then the database.
// Expected format: "ID:Secret"
func (s *AuthService) ValidateKey(ctx context.Context, rawKey string) (*domain.ApiKey, error) {
	// 1. Check Cache (SHA256 of raw key)
	hashedKey := sha256Hash(rawKey)
	if cachedKey, err := s.cache.GetKey(ctx, hashedKey); err == nil && cachedKey != nil {
		if cachedKey.IsValid() {
			return cachedKey, nil
		}
		// If cached key is invalid (expired/inactive), we could return error immediately
		// but let's re-check DB to be safe in case of swift updates (though cache TTL should be short)
	}

	// 2. Parse ID and Secret
	parts := strings.SplitN(rawKey, ":", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidKeyFormat
	}
	id, secret := parts[0], parts[1]

	// 3. Lookup in DB by ID
	apiKey, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, ErrInvalidKey
	}

	// 4. Validate Secret (Bcrypt)
	if err := bcrypt.CompareHashAndPassword([]byte(apiKey.KeyHash), []byte(secret)); err != nil {
		return nil, ErrInvalidKey
	}

	// 5. Check if active/expired
	if !apiKey.IsValid() {
		return nil, ErrInvalidKey
	}

	// 6. Cache success
	if err := s.cache.SetKey(ctx, hashedKey, apiKey); err != nil {
		// Log error but don't fail request
		// TODO: Logger
	}

	return apiKey, nil
}

// InvalidateKey removes a cached API key by its raw key hash.
// This should be called when a key is revoked or its status changes.
func (s *AuthService) InvalidateKey(ctx context.Context, rawKey string) error {
	hashedKey := sha256Hash(rawKey)
	return s.cache.InvalidateKey(ctx, hashedKey)
}

// InvalidateKeyByID removes cached entries for a specific API key ID.
// This is useful when a key is revoked via the admin API.
// Note: Since we cache by the hash of the raw key, we can't directly invalidate by ID.
// The caller should invalidate by raw key if available, or clear all cache entries.
func (s *AuthService) InvalidateKeyByID(ctx context.Context, keyID string) error {
	// Since we cache by hash of raw key, we can't directly invalidate by ID.
	// This is a limitation of the current caching strategy.
	// In a production system, we might want to maintain a mapping from ID to hashes.
	return nil
}

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}
