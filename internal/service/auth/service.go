package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/beremaran/straw/internal/domain"
)

// ErrInvalidKey is returned when an API key is invalid.
var ErrInvalidKey = errors.New("invalid api key")

// Service handles API key validation and invalidation.
type Service struct {
	repo      domain.APIKeyRepository
	tokenRepo domain.APIKeyTokenRepository
	cache     *Cache
}

// NewAuthService creates a new Service with the given repositories and cache.
func NewAuthService(repo domain.APIKeyRepository, tokenRepo domain.APIKeyTokenRepository, cache *Cache) *Service {
	return &Service{
		repo:      repo,
		tokenRepo: tokenRepo,
		cache:     cache,
	}
}

// ValidateKey validates an API key token and returns the associated key.
func (s *Service) ValidateKey(ctx context.Context, rawToken string) (*domain.APIKey, error) {
	tokenHash := sha256Hash(rawToken)

	cachedKey := s.cachedValidKey(ctx, tokenHash)
	if cachedKey != nil {
		return cachedKey, nil
	}

	token, err := s.tokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get token by hash: %w", err)
	}

	if token == nil || !token.IsValid() {
		return nil, ErrInvalidKey
	}

	apiKey, err := s.repo.GetByID(ctx, token.APIKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get key by ID: %w", err)
	}

	if !validAPIKey(apiKey) {
		return nil, ErrInvalidKey
	}

	if s.cache != nil {
		_ = s.cache.SetKey(ctx, tokenHash, apiKey)
	}

	return apiKey, nil
}

// InvalidateKey removes a cached API key by hashing the raw token.
func (s *Service) InvalidateKey(ctx context.Context, rawToken string) error {
	if s.cache == nil {
		return nil
	}

	tokenHash := sha256Hash(rawToken)

	return s.cache.InvalidateKey(ctx, tokenHash)
}

// InvalidateKeyByID removes cached entries for all tokens belonging to an API key.
func (s *Service) InvalidateKeyByID(ctx context.Context, keyID string) error {
	if s.cache == nil {
		return nil
	}

	tokens, err := s.tokenRepo.ListByAPIKeyID(ctx, keyID)
	if err != nil {
		return fmt.Errorf("failed to list tokens by API key ID: %w", err)
	}

	for _, token := range tokens {
		err = s.cache.InvalidateKey(ctx, token.TokenHash)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) cachedValidKey(ctx context.Context, tokenHash string) *domain.APIKey {
	if s.cache == nil {
		return nil
	}

	cachedKey, err := s.cache.GetKey(ctx, tokenHash)
	if err != nil || !validAPIKey(cachedKey) {
		return nil
	}

	return cachedKey
}

func validAPIKey(apiKey *domain.APIKey) bool {
	return apiKey != nil && apiKey.IsValid()
}

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))

	return hex.EncodeToString(hash[:])
}
