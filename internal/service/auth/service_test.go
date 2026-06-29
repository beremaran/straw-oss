package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	keyID   = "key-id"
	tokenID = "token-id"
)

type MockRepo struct {
	mock.Mock
}

func (m *MockRepo) GetByID(ctx context.Context, id string) (*domain.APIKey, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.APIKey), args.Error(1)
}

func (m *MockRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.APIKey, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.APIKey), args.Error(1)
}

func (m *MockRepo) Create(ctx context.Context, key *domain.APIKey) error {
	args := m.Called(ctx, key)

	return args.Error(0)
}

func (m *MockRepo) Update(ctx context.Context, key *domain.APIKey) error {
	args := m.Called(ctx, key)

	return args.Error(0)
}

func (m *MockRepo) List(ctx context.Context, limit, offset int) ([]domain.APIKey, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.APIKey), args.Int(1), args.Error(2)
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

func (m *MockTokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.APIKeyToken, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.APIKeyToken), args.Error(1)
}

func (m *MockTokenRepo) Create(ctx context.Context, token *domain.APIKeyToken) error {
	args := m.Called(ctx, token)

	return args.Error(0)
}

func (m *MockTokenRepo) ListByAPIKeyID(ctx context.Context, apiKeyID string) ([]domain.APIKeyToken, error) {
	args := m.Called(ctx, apiKeyID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.APIKeyToken), args.Error(1)
}

func (m *MockTokenRepo) Rotate(ctx context.Context, apiKeyID string, token *domain.APIKeyToken, graceUntil *time.Time, revokeExisting bool) error {
	args := m.Called(ctx, apiKeyID, token, graceUntil, revokeExisting)

	return args.Error(0)
}

func (m *MockTokenRepo) UpdateStatus(ctx context.Context, id string, status domain.TokenStatus) error {
	args := m.Called(ctx, id, status)

	return args.Error(0)
}

func TestAuthService_ValidateKey(t *testing.T) {
	ctx := context.Background()

	testToken := "test-bearer-token-12345"
	testTokenHash := sha256Hash(testToken)

	validAPIKey := &domain.APIKey{
		ID:        "key-id-123",
		TokenHash: testTokenHash,
		IsActive:  true,
	}

	validToken := &domain.APIKeyToken{
		ID:        "token-id-123",
		APIKeyID:  "key-id-123",
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
		mockRepo.On("GetByID", ctx, "key-id-123").Return(validAPIKey, nil).Once()

		key, err := service.ValidateKey(ctx, testToken)
		require.NoError(t, err)
		assert.Equal(t, validAPIKey, key)

		cached, err := cache.GetKey(ctx, testTokenHash)
		require.NoError(t, err)
		assert.Equal(t, validAPIKey.ID, cached.ID)

		mockRepo.AssertExpectations(t)
	})

	t.Run("Valid Token - Cache Hit", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		err := cache.SetKey(ctx, testTokenHash, validAPIKey)
		require.NoError(t, err)

		key, err := service.ValidateKey(ctx, testToken)
		require.NoError(t, err)
		assert.Equal(t, validAPIKey.ID, key.ID)
	})

	t.Run("Token Not Found in DB", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, _ := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		unknownToken := "unknown-token-xyz"
		unknownHash := sha256Hash(unknownToken)

		mockTokenRepo.On("GetByTokenHash", ctx, unknownHash).Return((*domain.APIKeyToken)(nil), nil).Once()

		key, err := service.ValidateKey(ctx, unknownToken)
		require.ErrorIs(t, err, ErrInvalidKey)
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
		inactiveAPIKey := &domain.APIKey{
			ID:        "key-id-inactive",
			TokenHash: inactiveHash,
			IsActive:  false,
		}

		inactiveToken := &domain.APIKeyToken{
			ID:        "token-id-inactive",
			APIKeyID:  "key-id-inactive",
			TokenHash: inactiveHash,
			Status:    domain.TokenStatusActive,
		}

		mockTokenRepo.On("GetByTokenHash", ctx, inactiveHash).Return(inactiveToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id-inactive").Return(inactiveAPIKey, nil).Once()

		key, err := service.ValidateKey(ctx, inactiveTokenString)
		require.ErrorIs(t, err, ErrInvalidKey)
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

		mockTokenRepo.On("GetByTokenHash", ctx, errorHash).Return((*domain.APIKeyToken)(nil), errors.New("db error")).Once()

		key, err := service.ValidateKey(ctx, errorToken)
		require.Error(t, err)
		assert.Nil(t, key)
	})

	t.Run("Cache Set Error", func(t *testing.T) {
		mockRepo := new(MockRepo)
		client, mr := newTestRedis(t)
		cache := NewAuthCache(client, time.Minute)
		mockTokenRepo := new(MockTokenRepo)
		service := NewAuthService(mockRepo, mockTokenRepo, cache)

		cacheErrorTestValue := "cache-error-token"
		cacheErrorHash := sha256Hash(cacheErrorTestValue)
		cacheErrorKey := &domain.APIKey{ID: keyID, TokenHash: cacheErrorHash, IsActive: true}

		cacheErrorTokenModel := &domain.APIKeyToken{
			ID:        tokenID,
			APIKeyID:  cacheErrorKey.ID,
			TokenHash: cacheErrorHash,
			Status:    domain.TokenStatusActive,
		}
		mockTokenRepo.On("GetByTokenHash", ctx, cacheErrorHash).Return(cacheErrorTokenModel, nil).Once()
		mockRepo.On("GetByID", ctx, cacheErrorTokenModel.APIKeyID).Return(cacheErrorKey, nil).Once()

		mr.Close()

		key, err := service.ValidateKey(ctx, cacheErrorTestValue)
		require.NoError(t, err)
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
		expiredAPIKey := &domain.APIKey{
			ID:        "key-id-expired",
			TokenHash: expiredHash,
			IsActive:  true,
			ExpiresAt: &expiredTime,
		}

		expiredToken := &domain.APIKeyToken{
			ID:        "token-id-expired",
			APIKeyID:  "key-id-expired",
			TokenHash: expiredHash,
			Status:    domain.TokenStatusActive,
		}

		mockTokenRepo.On("GetByTokenHash", ctx, expiredHash).Return(expiredToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id-expired").Return(expiredAPIKey, nil).Once()

		key, err := service.ValidateKey(ctx, expiredTokenString)
		require.ErrorIs(t, err, ErrInvalidKey)
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
		expiredCachedKey := &domain.APIKey{
			ID:        keyID,
			TokenHash: expiredCacheHash,
			IsActive:  true,
			ExpiresAt: &pastTime,
		}
		freshKey := &domain.APIKey{
			ID:        keyID,
			TokenHash: expiredCacheHash,
			IsActive:  true,
		}
		freshToken := &domain.APIKeyToken{
			ID:        tokenID,
			APIKeyID:  keyID,
			TokenHash: expiredCacheHash,
			Status:    domain.TokenStatusActive,
		}

		err := cache.SetKey(ctx, expiredCacheHash, expiredCachedKey)
		require.NoError(t, err)

		mockTokenRepo.On("GetByTokenHash", ctx, expiredCacheHash).Return(freshToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id").Return(freshKey, nil).Once()

		key, err := service.ValidateKey(ctx, expiredCacheToken)
		require.NoError(t, err)
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
		inactiveCachedKey := &domain.APIKey{
			ID:        keyID,
			TokenHash: inactiveCacheHash,
			IsActive:  false,
		}
		freshKey := &domain.APIKey{
			ID:        keyID,
			TokenHash: inactiveCacheHash,
			IsActive:  true,
		}
		freshToken := &domain.APIKeyToken{
			ID:        tokenID,
			APIKeyID:  keyID,
			TokenHash: inactiveCacheHash,
			Status:    domain.TokenStatusActive,
		}

		err := cache.SetKey(ctx, inactiveCacheHash, inactiveCachedKey)
		require.NoError(t, err)

		mockTokenRepo.On("GetByTokenHash", ctx, inactiveCacheHash).Return(freshToken, nil).Once()
		mockRepo.On("GetByID", ctx, "key-id").Return(freshKey, nil).Once()

		key, err := service.ValidateKey(ctx, inactiveCacheToken)
		require.NoError(t, err)
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

		err := cache.SetKey(ctx, tokenHash, &domain.APIKey{ID: "test"})
		require.NoError(t, err)

		err = service.InvalidateKey(ctx, rawToken)
		require.NoError(t, err)

		cached, err := cache.GetKey(ctx, tokenHash)
		require.NoError(t, err)
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
		require.Error(t, err)
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
		tokenHash := sha256Hash("token-to-invalidate")
		require.NoError(t, cache.SetKey(ctx, tokenHash, &domain.APIKey{ID: "test-id"}))
		mockTokenRepo.On("ListByAPIKeyID", ctx, "test-id").Return([]domain.APIKeyToken{
			{ID: "token-1", APIKeyID: "test-id", TokenHash: tokenHash, Status: domain.TokenStatusActive},
		}, nil).Once()

		err := service.InvalidateKeyByID(ctx, "test-id")
		require.NoError(t, err)

		cached, cacheErr := cache.GetKey(ctx, tokenHash)
		require.NoError(t, cacheErr)
		assert.Nil(t, cached)
	})
}
