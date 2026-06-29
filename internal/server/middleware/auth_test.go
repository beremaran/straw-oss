package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/service/auth"
	goredis "github.com/redis/go-redis/v9"
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

func (m *MockRepo) Update(ctx context.Context, key *domain.ApiKey) error {
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

func (m *MockTokenRepo) Rotate(ctx context.Context, apiKeyID string, token *domain.ApiKeyToken, graceUntil *time.Time, revokeExisting bool) error {
	args := m.Called(ctx, apiKeyID, token, graceUntil, revokeExisting)

	return args.Error(0)
}
func (m *MockTokenRepo) UpdateStatus(ctx context.Context, id string, status domain.TokenStatus) error {
	args := m.Called(ctx, id, status)

	return args.Error(0)
}

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))

	return hex.EncodeToString(hash[:])
}

func TestAuthMiddleware(t *testing.T) {
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})
	redisClient := &redis.Client{Client: rdb}
	authCache := auth.NewAuthCache(redisClient, time.Minute)

	mockRepo := new(MockRepo)
	mockTokenRepo := new(MockTokenRepo)
	authService := auth.NewAuthService(mockRepo, mockTokenRepo, authCache)
	mw := AuthMiddleware(authService)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := GetAPIKey(r)
		if apiKey != nil {
			_, _ = w.Write([]byte("success"))
		}
	}))

	t.Run("Missing Bearer Token", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "missing bearer token")
	})

	t.Run("Invalid Bearer Token", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		tokenHash := sha256Hash("invalid-token")
		mockTokenRepo.On("GetByTokenHash", mock.Anything, tokenHash).Return((*domain.ApiKeyToken)(nil), nil).Once()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid bearer token")
		mockRepo.AssertExpectations(t)
	})

	t.Run("Valid Bearer Token", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		tokenHash := sha256Hash("valid-token")
		validApiKey := &domain.ApiKey{ID: "test", TokenHash: tokenHash, IsActive: true}
		validToken := &domain.ApiKeyToken{ID: "token", ApiKeyID: "test", TokenHash: tokenHash, Status: domain.TokenStatusActive}

		mockTokenRepo.On("GetByTokenHash", mock.Anything, tokenHash).Return(validToken, nil).Once()
		mockRepo.On("GetByID", mock.Anything, validApiKey.ID).Return(validApiKey, nil).Once()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "success", rec.Body.String())
		mockRepo.AssertExpectations(t)
	})

	t.Run("Inactive API Key", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer inactive-token")
		rec := httptest.NewRecorder()

		tokenHash := sha256Hash("inactive-token")
		inactiveApiKey := &domain.ApiKey{ID: "test", TokenHash: tokenHash, IsActive: false}
		inactiveToken := &domain.ApiKeyToken{ID: "token", ApiKeyID: "test", TokenHash: tokenHash, Status: domain.TokenStatusActive}

		mockTokenRepo.On("GetByTokenHash", mock.Anything, tokenHash).Return(inactiveToken, nil).Once()
		mockRepo.On("GetByID", mock.Anything, inactiveApiKey.ID).Return(inactiveApiKey, nil).Once()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid bearer token")
		mockRepo.AssertExpectations(t)
	})

	t.Run("Invalid Authorization Header Format", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Basic abc123")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("Internal Error", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer error-token")
		rec := httptest.NewRecorder()

		tokenHash := sha256Hash("error-token")
		_ = authCache.InvalidateKey(context.Background(), tokenHash)
		mockTokenRepo.On("GetByTokenHash", mock.Anything, tokenHash).Return((*domain.ApiKeyToken)(nil), errors.New("db down")).Once()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		mockRepo.AssertExpectations(t)
	})
}
