package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
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

func (m *MockRepo) Exists(ctx context.Context) (bool, error) {
	args := m.Called(ctx)

	return args.Bool(0), args.Error(1)
}

func (m *MockRepo) Revoke(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

type MockTokenRepo struct {
	mock.Mock
}

func (m *MockTokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ApiKeyToken, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.ApiKeyToken), args.Error(1)
}

func (m *MockTokenRepo) Create(ctx context.Context, token *domain.ApiKeyToken) error {
	args := m.Called(ctx, token)

	return args.Error(0)
}

func (m *MockTokenRepo) ListByApiKeyID(ctx context.Context, apiKeyID string) ([]domain.ApiKeyToken, error) {
	args := m.Called(ctx, apiKeyID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.ApiKeyToken), args.Error(1)
}

func (m *MockTokenRepo) UpdateStatus(ctx context.Context, id string, status domain.TokenStatus) error {
	args := m.Called(ctx, id, status)

	return args.Error(0)
}

func TestAuthService_ValidateKey(t *testing.T) {
	ctx := context.Background()

	testToken := "test-bearer-token-12345"
	testTokenHash := sha256Hash(testToken)

	validApiKey := &domain.ApiKey{
		ID:        "key-id-123",
		TokenHash: testTokenHash,
		IsActive:  true,
	}
	
	validToken := &domain.ApiKeyToken{
		ID:        "token-id-123",
		ApiKeyID:  "key-id-123",
		TokenHash: testTokenHash,
		Status:    domain.TokenStatusActive,
	}

	t.Run("Valid Token - Cache Miss - DB Hit", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		mockTokenRepo.On("GetByTokenHash", ctx, testTokenHash).Return(validToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id-123").Return(validApiKey, nil).Once()

		key, err := service.ValidateKey(ctx, testToken)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey, key)

		cached, err := cache.GetKey(ctx, testTokenHash)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey.ID, cached.ID)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Valid Token - Cache Hit", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		err := cache.SetKey(ctx, testTokenHash, validApiKey)
		assert.NoError(t, err)

		key, err := service.ValidateKey(ctx, testToken)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey.ID, key.ID)
	})

	t.Run("Token Not Found in DB", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		unknownToken := "unknown-token-xyz"
		unknownHash := sha256Hash(unknownToken)

		mockTokenRepo.On("GetByTokenHash", ctx, unknownHash).Return((*domain.ApiKeyToken)(nil), nil).Once()

		key, err := service.ValidateKey(ctx, unknownToken)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Inactive Key", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		inactiveTokenString := "inactive-token"
		inactiveHash := sha256Hash(inactiveTokenString)
		inactiveApiKey := &domain.ApiKey{
			ID:        "key-id-inactive",
			TokenHash: inactiveHash,
			IsActive:  false,
		}
		
		inactiveToken := &domain.ApiKeyToken{
			ID:        "token-id-inactive",
			ApiKeyID:  "key-id-inactive",
			TokenHash: inactiveHash,
			Status:    domain.TokenStatusActive,
		}

		mockTokenRepo.On("GetByTokenHash", ctx, inactiveHash).Return(inactiveToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id-inactive").Return(inactiveApiKey, nil).Once()

		key, err := service.ValidateKey(ctx, inactiveTokenString)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("DB Error", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		errorToken := "error-causing-token"
		errorHash := sha256Hash(errorToken)

		mockTokenRepo.On("GetByTokenHash", ctx, errorHash).Return((*domain.ApiKeyToken)(nil), errors.New("db error")).Once()

		key, err := service.ValidateKey(ctx, errorToken)
		assert.Error(t, err)
		assert.Nil(t, key)
	})

	t.Run("Cache Set Error", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, mr := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		cacheErrorToken := "cache-error-token"
		cacheErrorHash := sha256Hash(cacheErrorToken)
		cacheErrorKey := &domain.ApiKey{ID: "key-id", TokenHash: cacheErrorHash, IsActive: true}

		cacheErrorTokenModel := &domain.ApiKeyToken{
			ID:        "token-id",
			ApiKeyID:  cacheErrorKey.ID,
			TokenHash: cacheErrorHash,
			Status:    domain.TokenStatusActive,
		}
		mockTokenRepo.On("GetByTokenHash", ctx, cacheErrorHash).Return(cacheErrorTokenModel, nil).Once()
		mockRepo.On("GetByID", ctx, cacheErrorTokenModel.ApiKeyID).Return(cacheErrorKey, nil).Once()

		mr.Close()

		key, err := service.ValidateKey(ctx, cacheErrorToken)
		assert.NoError(t, err)
		assert.Equal(t, cacheErrorKey.ID, key.ID)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Expired Key", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		expiredTokenString := "expired-token"
		expiredHash := sha256Hash(expiredTokenString)
		expiredTime := time.Now().Add(-1 * time.Hour)
		expiredApiKey := &domain.ApiKey{
			ID:        "key-id-expired",
			TokenHash: expiredHash,
			IsActive:  true,
			ExpiresAt: &expiredTime,
		}
		
		expiredToken := &domain.ApiKeyToken{
			ID:        "token-id-expired",
			ApiKeyID:  "key-id-expired",
			TokenHash: expiredHash,
			Status:    domain.TokenStatusActive,
		}

		mockTokenRepo.On("GetByTokenHash", ctx, expiredHash).Return(expiredToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id-expired").Return(expiredApiKey, nil).Once()

		key, err := service.ValidateKey(ctx, expiredTokenString)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Cache Hit with Invalid Cached Key (Expired)", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

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
		freshToken := &domain.ApiKeyToken{
			ID:        "token-id",
			ApiKeyID:  "key-id",
			TokenHash: expiredCacheHash,
			Status:    domain.TokenStatusActive,
		}

		err := cache.SetKey(ctx, expiredCacheHash, expiredCachedKey)
		assert.NoError(t, err)

		mockTokenRepo.On("GetByTokenHash", ctx, expiredCacheHash).Return(freshToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id").Return(freshKey, nil).Once()

		key, err := service.ValidateKey(ctx, expiredCacheToken)
		assert.NoError(t, err)
		assert.Equal(t, freshKey.ID, key.ID)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Cache Hit with Invalid Cached Key (Inactive)", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

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
		freshToken := &domain.ApiKeyToken{
			ID:        "token-id",
			ApiKeyID:  "key-id",
			TokenHash: inactiveCacheHash,
			Status:    domain.TokenStatusActive,
		}

		err := cache.SetKey(ctx, inactiveCacheHash, inactiveCachedKey)
		assert.NoError(t, err)

		mockTokenRepo.On("GetByTokenHash", ctx, inactiveCacheHash).Return(freshToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id").Return(freshKey, nil).Once()

		key, err := service.ValidateKey(ctx, inactiveCacheToken)
		assert.NoError(t, err)
		assert.Equal(t, freshKey.ID, key.ID)

		mockRepo.AssertExpectations(t)
	})
}

func TestAuthService_InvalidateKey(t *testing.T) {
	mockRepo := new(MockRepo)
	ctx := context.Background()

	t.Run("Successfully Invalidate Token", func(t *testing.T) {
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		rawToken := "some-token"
		tokenHash := sha256Hash(rawToken)

		err := cache.SetKey(ctx, tokenHash, &domain.ApiKey{ID: "test"})
		assert.NoError(t, err)

		err = service.InvalidateKey(ctx, rawToken)
		assert.NoError(t, err)

		cached, err := cache.GetKey(ctx, tokenHash)
		assert.NoError(t, err)
		assert.Nil(t, cached)
	})

	t.Run("Cache Error During Invalidation", func(t *testing.T) {
		client, mr := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		rawToken := "error-token"

		mr.Close()

		err := service.InvalidateKey(ctx, rawToken)
		assert.Error(t, err)
	})
}

func TestAuthService_InvalidateKeyByID(t *testing.T) {
	mockRepo := new(MockRepo)
	client, _ := newTestRedis(t)
	cache := NewAuthCache(client, time.Minute)
	mockTokenRepo := new(MockTokenRepo)
	service := NewAuthService(mockRepo, mockTokenRepo, cache)
	ctx := context.Background()

	t.Run("InvalidateKeyByID Returns Nil", func(t *testing.T) {
		err := service.InvalidateKeyByID(ctx, "test-id")
		assert.NoError(t, err)
	})
}
