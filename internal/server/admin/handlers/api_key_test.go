package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockAPIKeyInvalidator struct {
	mock.Mock
}

func (m *MockAPIKeyInvalidator) InvalidateKeyByID(ctx context.Context, keyID string) error {
	args := m.Called(ctx, keyID)

	return args.Error(0)
}

func TestApiKeyHandler_HandleListApiKeys(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/api-keys?page=1&limit=10", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	mockTokenRepo := new(MockApiKeyTokenRepo)
	handler := NewApiKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	keys := []domain.ApiKey{
		{ID: "key1", Name: "Key 1"},
		{ID: "key2", Name: "Key 2"},
	}
	mockRepo.On("List", mock.Anything, 10, 0).Return(keys, 2, nil).Once()

	handler.HandleListApiKeys(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), response["total"])
}

func TestApiKeyHandler_HandleCreateApiKey(t *testing.T) {
	body := `{"name":"New Key","scopes":["target:*"]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	mockTokenRepo := new(MockApiKeyTokenRepo)
	handler := NewApiKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(k *domain.ApiKey) bool {
		return k.Name == "New Key" && len(k.Scopes) == 1 && k.Scopes[0] == "target:*"
	})).Return(nil).Once()

	handler.HandleCreateApiKey(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response["raw_key"])
	assert.Equal(t, "New Key", response["name"])
}

func TestApiKeyHandler_HandleGetApiKey(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/api-keys/key1", nil)
	req.SetPathValue("id", "key1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	mockTokenRepo := new(MockApiKeyTokenRepo)
	handler := NewApiKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	key := &domain.ApiKey{ID: "key1", Name: "Key 1", IsActive: true}
	tokens := []domain.ApiKeyToken{
		{ID: "token-1", ApiKeyID: "key1", Status: domain.TokenStatusActive, CreatedAt: time.Now()},
	}
	mockRepo.On("GetByID", mock.Anything, "key1").Return(key, nil).Once()
	mockTokenRepo.On("ListByApiKeyID", mock.Anything, "key1").Return(tokens, nil).Once()

	handler.HandleGetApiKey(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "token_hash")
	assert.Contains(t, rec.Body.String(), "\"tokens\"")
}

func TestApiKeyHandler_HandleUpdateApiKey(t *testing.T) {
	body := `{"name":"Updated Key","scopes":["target:*"],"rate_limit_override":12,"expires_at":"","is_active":false}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/api-keys/key1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "key1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	mockTokenRepo := new(MockApiKeyTokenRepo)
	invalidator := new(MockAPIKeyInvalidator)
	handler := NewApiKeyHandler(mockRepo, mockTokenRepo, nil, invalidator)

	key := &domain.ApiKey{ID: "key1", Name: "Old Key", IsActive: true, ExpiresAt: &time.Time{}}
	mockRepo.On("GetByID", mock.Anything, "key1").Return(key, nil).Once()
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(updated *domain.ApiKey) bool {
		return updated.ID == "key1" &&
			updated.Name == "Updated Key" &&
			updated.RateLimitOverride != nil &&
			*updated.RateLimitOverride == 12 &&
			updated.ExpiresAt == nil &&
			!updated.IsActive
	})).Return(nil).Once()
	mockTokenRepo.On("ListByApiKeyID", mock.Anything, "key1").Return([]domain.ApiKeyToken{}, nil).Once()
	invalidator.On("InvalidateKeyByID", mock.Anything, "key1").Return(nil).Once()

	handler.HandleUpdateApiKey(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "\"name\":\"Updated Key\"")
	assert.Contains(t, rec.Body.String(), "\"is_active\":false")
}

func TestApiKeyHandler_HandleRotateApiKey_RedactsAuditPayload(t *testing.T) {
	body := `{"grace_period":"24h"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/api-keys/key1/rotate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "key1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	mockTokenRepo := new(MockApiKeyTokenRepo)
	mockAuditRepo := new(MockManagementAuditRepo)
	invalidator := new(MockAPIKeyInvalidator)
	handler := NewApiKeyHandler(mockRepo, mockTokenRepo, mockAuditRepo, invalidator)

	key := &domain.ApiKey{ID: "key1", Name: "Key 1", IsActive: true}
	mockRepo.On("GetByID", mock.Anything, "key1").Return(key, nil).Once()
	mockTokenRepo.On("Rotate", mock.Anything, "key1", mock.AnythingOfType("*domain.ApiKeyToken"), mock.AnythingOfType("*time.Time"), false).Return(nil).Once()
	invalidator.On("InvalidateKeyByID", mock.Anything, "key1").Return(nil).Once()
	mockAuditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		payload, err := json.Marshal(event.NewValue)
		if err != nil {
			return false
		}

		return event.Action == domain.ActionRotate && !strings.Contains(string(payload), "raw_key")
	})).Return(nil).Once()

	handler.HandleRotateApiKey(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "\"raw_key\"")
	assert.Contains(t, rec.Body.String(), "\"token_id\"")
}

func TestApiKeyHandler_HandleReactivateApiKey_Expired(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/api-keys/key1/reactivate", nil)
	req.SetPathValue("id", "key1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	mockTokenRepo := new(MockApiKeyTokenRepo)
	handler := NewApiKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	expired := time.Now().Add(-time.Hour)
	mockRepo.On("GetByID", mock.Anything, "key1").Return(&domain.ApiKey{
		ID:        "key1",
		Name:      "Expired Key",
		IsActive:  false,
		ExpiresAt: &expired,
	}, nil).Once()

	handler.HandleReactivateApiKey(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "expired api key cannot be reactivated")
}

func TestApiKeyHandler_HandleRevokeApiKey(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/api-keys/key1", nil)
	req.SetPathValue("id", "key1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	mockTokenRepo := new(MockApiKeyTokenRepo)
	invalidator := new(MockAPIKeyInvalidator)
	handler := NewApiKeyHandler(mockRepo, mockTokenRepo, nil, invalidator)

	mockRepo.On("GetByID", mock.Anything, "key1").Return(&domain.ApiKey{ID: "key1", Name: "Key 1", IsActive: true}, nil).Once()
	mockRepo.On("Revoke", mock.Anything, "key1").Return(nil).Once()
	invalidator.On("InvalidateKeyByID", mock.Anything, "key1").Return(nil).Once()

	handler.HandleRevokeApiKey(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
