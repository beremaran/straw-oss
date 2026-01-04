package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockValidator struct {
	mock.Mock
}

func (m *MockValidator) ValidateKey(ctx context.Context, rawKey string) (*domain.ApiKey, error) {
	args := m.Called(ctx, rawKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}

func TestAuthMiddleware(t *testing.T) {
	e := echo.New()
	mockValidator := new(MockValidator)
	mw := AuthMiddleware(mockValidator)

	// dummy handler
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	t.Run("Missing Bearer Token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler(c)
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
		assert.Contains(t, httpErr.Message, "missing bearer token")
	})

	t.Run("Invalid Bearer Token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		mockValidator.On("ValidateKey", mock.Anything, "invalid-token").Return(nil, auth.ErrInvalidKey).Once()

		err := handler(c)
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
		assert.Contains(t, httpErr.Message, "invalid bearer token")
		mockValidator.AssertExpectations(t)
	})

	t.Run("Valid Bearer Token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		validApiKey := &domain.ApiKey{ID: "test"}
		mockValidator.On("ValidateKey", mock.Anything, "valid-token").Return(validApiKey, nil).Once()

		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, validApiKey, c.Get(ContextKeyAPIKey))
		mockValidator.AssertExpectations(t)
	})

	t.Run("Invalid Authorization Header Format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Basic abc123") // Wrong auth type
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler(c)
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	})

	t.Run("Internal Error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer error-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		mockValidator.On("ValidateKey", mock.Anything, "error-token").Return(nil, errors.New("db down")).Once()

		err := handler(c)
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
		mockValidator.AssertExpectations(t)
	})
}
