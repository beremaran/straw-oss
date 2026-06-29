package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/beremaran/straw/internal/domain"
)

var (
	ErrInvalidKey = errors.New("invalid api key")
)

type Service struct {
	repo      domain.ApiKeyRepository
	tokenRepo domain.ApiKeyTokenRepository
	cache     *Cache
}

func NewAuthService(repo domain.ApiKeyRepository, tokenRepo domain.ApiKeyTokenRepository, cache *Cache) *Service {
	return &Service{
		repo:      repo,
		tokenRepo: tokenRepo,
		cache:     cache,
	}
}

func (s *Service) ValidateKey(ctx context.Context, rawToken string) (*domain.ApiKey, error) {
	tokenHash := sha256Hash(rawToken)

	cachedKey := s.cachedValidKey(ctx, tokenHash)
	if cachedKey != nil {
		return cachedKey, nil
	}

	token, err := s.tokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if token == nil || !token.IsValid() {
		return nil, ErrInvalidKey
	}

	apiKey, err := s.repo.GetByID(ctx, token.ApiKeyID)
	if err != nil {
		return nil, err
	}
	if !validAPIKey(apiKey) {
		return nil, ErrInvalidKey
	}

	if s.cache != nil {
		_ = s.cache.SetKey(ctx, tokenHash, apiKey)
	}

	return apiKey, nil
}

func (s *Service) InvalidateKey(ctx context.Context, rawToken string) error {
	if s.cache == nil {
		return nil
	}
	tokenHash := sha256Hash(rawToken)

	return s.cache.InvalidateKey(ctx, tokenHash)
}

func (s *Service) InvalidateKeyByID(ctx context.Context, keyID string) error {
	if s.cache == nil {
		return nil
	}

	tokens, err := s.tokenRepo.ListByApiKeyID(ctx, keyID)
	if err != nil {
		return err
	}

	for _, token := range tokens {
		err = s.cache.InvalidateKey(ctx, token.TokenHash)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) cachedValidKey(ctx context.Context, tokenHash string) *domain.ApiKey {
	if s.cache == nil {
		return nil
	}

	cachedKey, err := s.cache.GetKey(ctx, tokenHash)
	if err != nil || !validAPIKey(cachedKey) {
		return nil
	}

	return cachedKey
}

func validAPIKey(apiKey *domain.ApiKey) bool {
	return apiKey != nil && apiKey.IsValid()
}

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))

	return hex.EncodeToString(hash[:])
}
