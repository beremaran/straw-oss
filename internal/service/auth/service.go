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

	if s.cache != nil {
		cachedKey, err := s.cache.GetKey(ctx, tokenHash)
		if err == nil && cachedKey != nil {
			if cachedKey.IsValid() {
				return cachedKey, nil
			}
		}
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
	if apiKey == nil {
		return nil, ErrInvalidKey
	}

	if !apiKey.IsValid() {
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
	return nil
}

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))

	return hex.EncodeToString(hash[:])
}
