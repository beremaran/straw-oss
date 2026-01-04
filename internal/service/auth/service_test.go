package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepo struct {
	mock.Mock
}

func (m *MockRepo) GetByID(ctx context.Context, id string) (*domain.ApiKey, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}

func (m *MockRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ApiKey, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}

func (m *MockRepo) Create(ctx context.Context, key *domain.ApiKey) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockRepo) List(ctx context.Context, limit, offset int) ([]domain.ApiKey, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]domain.ApiKey), args.Int(1), args.Error(2)
}

func (m *MockRepo) Revoke(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

type MockCache struct {
	mock.Mock
}

func (m *MockCache) GetKey(ctx context.Context, keyHash string) (*domain.ApiKey, error) {
	args := m.Called(ctx, keyHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}

func (m *MockCache) SetKey(ctx context.Context, keyHash string, apiKey *domain.ApiKey) error {
	args := m.Called(ctx, keyHash, apiKey)
	return args.Error(0)
}

func (m *MockCache) InvalidateKey(ctx context.Context, keyHash string) error {
	args := m.Called(ctx, keyHash)
	return args.Error(0)
}

func TestAuthService_ValidateKey(t *testing.T) {
	// Setup
	mockRepo := new(MockRepo)
	mockCache := new(MockCache)
	service := NewAuthService(mockRepo, mockCache)
	ctx := context.Background()

	// Test token - any string is valid now, we just hash it
	testToken := "test-bearer-token-12345"
	testTokenHash := sha256Hash(testToken)

	validApiKey := &domain.ApiKey{
		ID:        "key-id-123",
		TokenHash: testTokenHash,
		IsActive:  true,
	}

	t.Run("Valid Token - Cache Miss - DB Hit", func(t *testing.T) {
		// Expect cache check
		mockCache.On("GetKey", ctx, testTokenHash).Return(nil, nil).Once()

		// Expect DB check by token hash
		mockRepo.On("GetByTokenHash", ctx, testTokenHash).Return(validApiKey, nil).Once()

		// Expect cache set
		mockCache.On("SetKey", ctx, testTokenHash, validApiKey).Return(nil).Once()

		key, err := service.ValidateKey(ctx, testToken)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Valid Token - Cache Hit", func(t *testing.T) {
		// Expect cache check - Hit
		mockCache.On("GetKey", ctx, testTokenHash).Return(validApiKey, nil).Once()

		key, err := service.ValidateKey(ctx, testToken)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey, key)

		mockCache.AssertExpectations(t)
	})

	t.Run("Token Not Found in DB", func(t *testing.T) {
		unknownToken := "unknown-token-xyz"
		unknownHash := sha256Hash(unknownToken)

		mockCache.On("GetKey", ctx, unknownHash).Return(nil, nil).Once()
		mockRepo.On("GetByTokenHash", ctx, unknownHash).Return(nil, nil).Once()

		key, err := service.ValidateKey(ctx, unknownToken)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Inactive Key", func(t *testing.T) {
		inactiveToken := "inactive-token"
		inactiveHash := sha256Hash(inactiveToken)
		inactiveKey := &domain.ApiKey{ID: "key-id", TokenHash: inactiveHash, IsActive: false}

		mockCache.On("GetKey", ctx, inactiveHash).Return(nil, nil).Once()
		mockRepo.On("GetByTokenHash", ctx, inactiveHash).Return(inactiveKey, nil).Once()

		key, err := service.ValidateKey(ctx, inactiveToken)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("DB Error", func(t *testing.T) {
		errorToken := "error-causing-token"
		errorHash := sha256Hash(errorToken)

		mockCache.On("GetKey", ctx, errorHash).Return(nil, nil).Once()
		mockRepo.On("GetByTokenHash", ctx, errorHash).Return(nil, errors.New("db error")).Once()

		key, err := service.ValidateKey(ctx, errorToken)
		assert.Error(t, err)
		assert.Nil(t, key)
	})

	t.Run("Cache Set Error", func(t *testing.T) {
		cacheErrorToken := "cache-error-token"
		cacheErrorHash := sha256Hash(cacheErrorToken)
		cacheErrorKey := &domain.ApiKey{ID: "key-id", TokenHash: cacheErrorHash, IsActive: true}

		// Expect cache check
		mockCache.On("GetKey", ctx, cacheErrorHash).Return(nil, nil).Once()

		// Expect DB check
		mockRepo.On("GetByTokenHash", ctx, cacheErrorHash).Return(cacheErrorKey, nil).Once()

		// Expect cache set - Error
		mockCache.On("SetKey", ctx, cacheErrorHash, cacheErrorKey).Return(errors.New("cache error")).Once()

		key, err := service.ValidateKey(ctx, cacheErrorToken)
		assert.NoError(t, err) // Should not fail request
		assert.Equal(t, cacheErrorKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Expired Key", func(t *testing.T) {
		expiredToken := "expired-token"
		expiredHash := sha256Hash(expiredToken)
		pastTime := time.Now().Add(-1 * time.Hour)
		expiredKey := &domain.ApiKey{
			ID:        "key-id",
			TokenHash: expiredHash,
			IsActive:  true,
			ExpiresAt: &pastTime,
		}

		mockCache.On("GetKey", ctx, expiredHash).Return(nil, nil).Once()
		mockRepo.On("GetByTokenHash", ctx, expiredHash).Return(expiredKey, nil).Once()

		key, err := service.ValidateKey(ctx, expiredToken)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Cache Hit with Invalid Cached Key (Expired)", func(t *testing.T) {
		expiredCacheToken := "expired-cache-token"
		expiredCacheHash := sha256Hash(expiredCacheToken)
		pastTime := time.Now().Add(-1 * time.Hour)
		expiredCachedKey := &domain.ApiKey{
			ID:        "key-id",
			TokenHash: expiredCacheHash,
			IsActive:  true,
			ExpiresAt: &pastTime,
		}
		freshKey := &domain.ApiKey{
			ID:        "key-id",
			TokenHash: expiredCacheHash,
			IsActive:  true,
		}

		// Cache returns an expired key
		mockCache.On("GetKey", ctx, expiredCacheHash).Return(expiredCachedKey, nil).Once()
		// Should re-check DB
		mockRepo.On("GetByTokenHash", ctx, expiredCacheHash).Return(freshKey, nil).Once()
		// Should update cache
		mockCache.On("SetKey", ctx, expiredCacheHash, freshKey).Return(nil).Once()

		key, err := service.ValidateKey(ctx, expiredCacheToken)
		assert.NoError(t, err)
		assert.Equal(t, freshKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Cache Hit with Invalid Cached Key (Inactive)", func(t *testing.T) {
		inactiveCacheToken := "inactive-cache-token"
		inactiveCacheHash := sha256Hash(inactiveCacheToken)
		inactiveCachedKey := &domain.ApiKey{
			ID:        "key-id",
			TokenHash: inactiveCacheHash,
			IsActive:  false,
		}
		freshKey := &domain.ApiKey{
			ID:        "key-id",
			TokenHash: inactiveCacheHash,
			IsActive:  true,
		}

		// Cache returns an inactive key
		mockCache.On("GetKey", ctx, inactiveCacheHash).Return(inactiveCachedKey, nil).Once()
		// Should re-check DB
		mockRepo.On("GetByTokenHash", ctx, inactiveCacheHash).Return(freshKey, nil).Once()
		// Should update cache
		mockCache.On("SetKey", ctx, inactiveCacheHash, freshKey).Return(nil).Once()

		key, err := service.ValidateKey(ctx, inactiveCacheToken)
		assert.NoError(t, err)
		assert.Equal(t, freshKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})
}

func TestAuthService_InvalidateKey(t *testing.T) {
	mockRepo := new(MockRepo)
	mockCache := new(MockCache)
	service := NewAuthService(mockRepo, mockCache)
	ctx := context.Background()

	t.Run("Successfully Invalidate Token", func(t *testing.T) {
		rawToken := "some-token"
		tokenHash := sha256Hash(rawToken)
		mockCache.On("InvalidateKey", ctx, tokenHash).Return(nil).Once()

		err := service.InvalidateKey(ctx, rawToken)
		assert.NoError(t, err)
		mockCache.AssertExpectations(t)
	})

	t.Run("Cache Error During Invalidation", func(t *testing.T) {
		rawToken := "error-token"
		tokenHash := sha256Hash(rawToken)
		mockCache.On("InvalidateKey", ctx, tokenHash).Return(errors.New("cache error")).Once()

		err := service.InvalidateKey(ctx, rawToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cache error")
		mockCache.AssertExpectations(t)
	})
}

func TestAuthService_InvalidateKeyByID(t *testing.T) {
	mockRepo := new(MockRepo)
	mockCache := new(MockCache)
	service := NewAuthService(mockRepo, mockCache)
	ctx := context.Background()

	t.Run("InvalidateKeyByID Returns Nil", func(t *testing.T) {
		// This is a no-op since we can't invalidate by ID with current cache strategy
		err := service.InvalidateKeyByID(ctx, "test-id")
		assert.NoError(t, err)
	})
}
