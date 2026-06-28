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
	repo  domain.ApiKeyRepository
	cache *Cache
}

func NewAuthService(repo domain.ApiKeyRepository, cache *Cache) *Service {
	return &Service{
		repo:  repo,
		cache: cache,
	}
}

func (s *Service) ValidateKey(ctx context.Context, rawToken string) (*domain.ApiKey, error) {
	tokenHash := sha256Hash(rawToken)

	if s.cache != nil {
		if cachedKey, err := s.cache.GetKey(ctx, tokenHash); err == nil && cachedKey != nil {
			if cachedKey.IsValid() {
				return cachedKey, nil
			}
		}
	}

	apiKey, err := s.repo.GetByTokenHash(ctx, tokenHash)
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
		err := s.cache.SetKey(ctx, tokenHash, apiKey)
		if err != nil {

		}
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
