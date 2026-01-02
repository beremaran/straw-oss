package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
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

	// Helpers
	keyID := "test-id"
	keySecret := "test-secret"
	rawKey := keyID + ":" + keySecret
	hashedSecret, _ := bcrypt.GenerateFromPassword([]byte(keySecret), bcrypt.DefaultCost)

	validApiKey := &domain.ApiKey{
		ID:       keyID,
		KeyHash:  string(hashedSecret),
		IsActive: true,
	}

	t.Run("Valid Key - Cache Miss - DB Hit", func(t *testing.T) {
		// Expect cache check
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()

		// Expect DB check
		mockRepo.On("GetByID", ctx, keyID).Return(validApiKey, nil).Once()

		// Expect cache set
		mockCache.On("SetKey", ctx, mock.AnythingOfType("string"), validApiKey).Return(nil).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Valid Key - Cache Hit", func(t *testing.T) {
		// Expect cache check - Hit
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(validApiKey, nil).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey, key)

		mockCache.AssertExpectations(t)
	})

	t.Run("Invalid Key Format", func(t *testing.T) {
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()

		key, err := service.ValidateKey(ctx, "invalid-format")
		assert.ErrorIs(t, err, ErrInvalidKeyFormat)
		assert.Nil(t, key)
	})

	t.Run("Invalid Secret", func(t *testing.T) {
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()
		mockRepo.On("GetByID", ctx, keyID).Return(validApiKey, nil).Once()
		// No cache set

		badKey := keyID + ":wrong-secret"
		key, err := service.ValidateKey(ctx, badKey)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Inactive Key", func(t *testing.T) {
		inactiveKey := &domain.ApiKey{ID: keyID, KeyHash: string(hashedSecret), IsActive: false}

		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()
		mockRepo.On("GetByID", ctx, keyID).Return(inactiveKey, nil).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("DB Error", func(t *testing.T) {
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()
		mockRepo.On("GetByID", ctx, keyID).Return(nil, errors.New("db error")).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.Error(t, err)
		assert.Nil(t, key)
	})

	t.Run("Cache Set Error", func(t *testing.T) {
		// Expect cache check
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()

		// Expect DB check
		mockRepo.On("GetByID", ctx, keyID).Return(validApiKey, nil).Once()

		// Expect cache set - Error
		mockCache.On("SetKey", ctx, mock.AnythingOfType("string"), validApiKey).Return(errors.New("cache error")).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.NoError(t, err) // Should not fail request
		assert.Equal(t, validApiKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Expired Key", func(t *testing.T) {
		pastTime := time.Now().Add(-1 * time.Hour)
		expiredKey := &domain.ApiKey{
			ID:        keyID,
			KeyHash:   string(hashedSecret),
			IsActive:  true,
			ExpiresAt: &pastTime,
		}

		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()
		mockRepo.On("GetByID", ctx, keyID).Return(expiredKey, nil).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})

	t.Run("Cache Hit with Invalid Cached Key (Expired)", func(t *testing.T) {
		pastTime := time.Now().Add(-1 * time.Hour)
		expiredCachedKey := &domain.ApiKey{
			ID:        keyID,
			KeyHash:   string(hashedSecret),
			IsActive:  true,
			ExpiresAt: &pastTime,
		}

		// Cache returns an expired key
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(expiredCachedKey, nil).Once()
		// Should re-check DB
		mockRepo.On("GetByID", ctx, keyID).Return(validApiKey, nil).Once()
		// Should update cache
		mockCache.On("SetKey", ctx, mock.AnythingOfType("string"), validApiKey).Return(nil).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Cache Hit with Invalid Cached Key (Inactive)", func(t *testing.T) {
		inactiveCachedKey := &domain.ApiKey{
			ID:       keyID,
			KeyHash:  string(hashedSecret),
			IsActive: false,
		}

		// Cache returns an inactive key
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(inactiveCachedKey, nil).Once()
		// Should re-check DB
		mockRepo.On("GetByID", ctx, keyID).Return(validApiKey, nil).Once()
		// Should update cache
		mockCache.On("SetKey", ctx, mock.AnythingOfType("string"), validApiKey).Return(nil).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.NoError(t, err)
		assert.Equal(t, validApiKey, key)

		mockCache.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Key Not Found in DB", func(t *testing.T) {
		mockCache.On("GetKey", ctx, mock.AnythingOfType("string")).Return(nil, nil).Once()
		mockRepo.On("GetByID", ctx, keyID).Return(nil, nil).Once()

		key, err := service.ValidateKey(ctx, rawKey)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, key)
	})
}

func TestAuthService_InvalidateKey(t *testing.T) {
	mockRepo := new(MockRepo)
	mockCache := new(MockCache)
	service := NewAuthService(mockRepo, mockCache)
	ctx := context.Background()

	t.Run("Successfully Invalidate Key", func(t *testing.T) {
		rawKey := "test-id:test-secret"
		mockCache.On("InvalidateKey", ctx, mock.AnythingOfType("string")).Return(nil).Once()

		err := service.InvalidateKey(ctx, rawKey)
		assert.NoError(t, err)
		mockCache.AssertExpectations(t)
	})

	t.Run("Cache Error During Invalidation", func(t *testing.T) {
		rawKey := "test-id:test-secret"
		mockCache.On("InvalidateKey", ctx, mock.AnythingOfType("string")).Return(errors.New("cache error")).Once()

		err := service.InvalidateKey(ctx, rawKey)
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
