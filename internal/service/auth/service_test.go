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

func TestAuthService_ValidateKey(t *testing.T) {
	ctx := context.Background()

	testToken := "test-bearer-token-12345"
	testTokenHash := sha256Hash(testToken)

	validApiKey := &domain.ApiKey{
		ID:        "key-id-123",
		TokenHash: testTokenHash,
		IsActive:  true,
	}

	t.Run("Valid Token - Cache Miss - DB Hit", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		service := NewAuthService(mockRepo, cache)

		mockRepo.On("GetByTokenHash", ctx, testTokenHash).Return(validApiKey, nil).Once()

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
		service := NewAuthService(mockRepo, cache)

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
		service := NewAuthService(mockRepo, cache)

		unknownToken := "unknown-token-xyz"
		unknownHash := sha256Hash(unknownToken)

		mockRepo.On("GetByTokenHash", ctx, unknownHash).Return(nil, nil).Once()

		key, err := service.ValidateKey(ctx, unknownToken)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Inactive Key", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		service := NewAuthService(mockRepo, cache)

		inactiveToken := "inactive-token"
		inactiveHash := sha256Hash(inactiveToken)
		inactiveKey := &domain.ApiKey{ID: "key-id", TokenHash: inactiveHash, IsActive: false}

		mockRepo.On("GetByTokenHash", ctx, inactiveHash).Return(inactiveKey, nil).Once()

		key, err := service.ValidateKey(ctx, inactiveToken)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("DB Error", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		service := NewAuthService(mockRepo, cache)

		errorToken := "error-causing-token"
		errorHash := sha256Hash(errorToken)

		mockRepo.On("GetByTokenHash", ctx, errorHash).Return(nil, errors.New("db error")).Once()

		key, err := service.ValidateKey(ctx, errorToken)
		assert.Error(t, err)
		assert.Nil(t, key)
	})

	t.Run("Cache Set Error", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, mr := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		service := NewAuthService(mockRepo, cache)

		cacheErrorToken := "cache-error-token"
		cacheErrorHash := sha256Hash(cacheErrorToken)
		cacheErrorKey := &domain.ApiKey{ID: "key-id", TokenHash: cacheErrorHash, IsActive: true}

		mockRepo.On("GetByTokenHash", ctx, cacheErrorHash).Return(cacheErrorKey, nil).Once()

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
		service := NewAuthService(mockRepo, cache)

		expiredToken := "expired-token"
		expiredHash := sha256Hash(expiredToken)
		pastTime := time.Now().Add(-1 * time.Hour)
		expiredKey := &domain.ApiKey{
			ID:        "key-id",
			TokenHash: expiredHash,
			IsActive:  true,
			ExpiresAt: &pastTime,
		}

		mockRepo.On("GetByTokenHash", ctx, expiredHash).Return(expiredKey, nil).Once()

		key, err := service.ValidateKey(ctx, expiredToken)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Cache Hit with Invalid Cached Key (Expired)", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		service := NewAuthService(mockRepo, cache)

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

		err := cache.SetKey(ctx, expiredCacheHash, expiredCachedKey)
		assert.NoError(t, err)

		mockRepo.On("GetByTokenHash", ctx, expiredCacheHash).Return(freshKey, nil).Once()

		key, err := service.ValidateKey(ctx, expiredCacheToken)
		assert.NoError(t, err)
		assert.Equal(t, freshKey.ID, key.ID)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Cache Hit with Invalid Cached Key (Inactive)", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		service := NewAuthService(mockRepo, cache)

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

		err := cache.SetKey(ctx, inactiveCacheHash, inactiveCachedKey)
		assert.NoError(t, err)

		mockRepo.On("GetByTokenHash", ctx, inactiveCacheHash).Return(freshKey, nil).Once()

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
		service := NewAuthService(mockRepo, cache)

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
		service := NewAuthService(mockRepo, cache)

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
	service := NewAuthService(mockRepo, cache)
	ctx := context.Background()

	t.Run("InvalidateKeyByID Returns Nil", func(t *testing.T) {

		err := service.InvalidateKeyByID(ctx, "test-id")
		assert.NoError(t, err)
	})
}
