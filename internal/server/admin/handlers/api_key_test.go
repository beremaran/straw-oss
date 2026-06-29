package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testKeyName = "Key 1"
	testKeyID   = "key1"
)

type MockAPIKeyInvalidator struct {
	mock.Mock
}

func (m *MockAPIKeyInvalidator) InvalidateKeyByID(ctx context.Context, keyID string) error {
	args := m.Called(ctx, keyID)

	return args.Error(0)
}

func TestAPIKeyHandler_HandleListAPIKeys(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/api-keys?page=1&limit=10", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockAPIKeyRepo)
	mockTokenRepo := new(MockAPIKeyTokenRepo)
	handler := NewAPIKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	keys := []domain.APIKey{
		{ID: testKeyID, Name: testKeyName},
		{ID: "key2", Name: "Key 2"},
	}
	mockRepo.On("List", mock.Anything, 10, 0).Return(keys, 2, nil).Once()

	handler.HandleListAPIKeys(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.InEpsilon(t, float64(2), response["total"], 0.01)
}

func TestAPIKeyHandler_HandleCreateAPIKey(t *testing.T) {
	body := `{"name":"New Key","scopes":["target:*"]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mockRepo := new(MockAPIKeyRepo)
	mockTokenRepo := new(MockAPIKeyTokenRepo)
	handler := NewAPIKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(k *domain.APIKey) bool {
		return k.Name == "New Key" && len(k.Scopes) == 1 && k.Scopes[0] == "target:*"
	})).Return(nil).Once()

	handler.HandleCreateAPIKey(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var response map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response["raw_key"])
	assert.Equal(t, "New Key", response["name"])
}

func TestAPIKeyHandler_HandleGetAPIKey(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/api-keys/key1", nil)
	req.SetPathValue("id", testKeyID)
	rec := httptest.NewRecorder()

	mockRepo := new(MockAPIKeyRepo)
	mockTokenRepo := new(MockAPIKeyTokenRepo)
	handler := NewAPIKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	key := &domain.APIKey{ID: testKeyID, Name: testKeyName, IsActive: true}
	tokens := []domain.APIKeyToken{
		{ID: "token-1", APIKeyID: testKeyID, Status: domain.TokenStatusActive, CreatedAt: time.Now()},
	}
	mockRepo.On("GetByID", mock.Anything, testKeyID).Return(key, nil).Once()
	mockTokenRepo.On("ListByAPIKeyID", mock.Anything, testKeyID).Return(tokens, nil).Once()

	handler.HandleGetAPIKey(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "token_hash")
	assert.Contains(t, rec.Body.String(), "\"tokens\"")
}

func TestAPIKeyHandler_HandleUpdateAPIKey(t *testing.T) {
	body := `{"name":"Updated Key","scopes":["target:*"],"rate_limit_override":12,"expires_at":"","is_active":false}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/api-keys/key1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", testKeyID)
	rec := httptest.NewRecorder()

	mockRepo := new(MockAPIKeyRepo)
	mockTokenRepo := new(MockAPIKeyTokenRepo)
	invalidator := new(MockAPIKeyInvalidator)
	handler := NewAPIKeyHandler(mockRepo, mockTokenRepo, nil, invalidator)

	key := &domain.APIKey{ID: testKeyID, Name: "Old Key", IsActive: true, ExpiresAt: &time.Time{}}
	mockRepo.On("GetByID", mock.Anything, testKeyID).Return(key, nil).Once()
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(updated *domain.APIKey) bool {
		return updated.ID == testKeyID &&
			updated.Name == "Updated Key" &&
			updated.RateLimitOverride != nil &&
			*updated.RateLimitOverride == 12 &&
			updated.ExpiresAt == nil &&
			!updated.IsActive
	})).Return(nil).Once()
	mockTokenRepo.On("ListByAPIKeyID", mock.Anything, testKeyID).Return([]domain.APIKeyToken{}, nil).Once()
	invalidator.On("InvalidateKeyByID", mock.Anything, testKeyID).Return(nil).Once()

	handler.HandleUpdateAPIKey(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "\"name\":\"Updated Key\"")
	assert.Contains(t, rec.Body.String(), "\"is_active\":false")
}

func TestAPIKeyHandler_HandleRotateAPIKey_RedactsAuditPayload(t *testing.T) {
	body := `{"grace_period":"24h"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/api-keys/key1/rotate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", testKeyID)
	rec := httptest.NewRecorder()

	mockRepo := new(MockAPIKeyRepo)
	mockTokenRepo := new(MockAPIKeyTokenRepo)
	mockAuditRepo := new(MockManagementAuditRepo)
	invalidator := new(MockAPIKeyInvalidator)
	handler := NewAPIKeyHandler(mockRepo, mockTokenRepo, mockAuditRepo, invalidator)

	key := &domain.APIKey{ID: testKeyID, Name: testKeyName, IsActive: true}
	mockRepo.On("GetByID", mock.Anything, testKeyID).Return(key, nil).Once()
	mockTokenRepo.On("Rotate", mock.Anything, testKeyID, mock.AnythingOfType("*domain.APIKeyToken"), mock.AnythingOfType("*time.Time"), false).Return(nil).Once()
	invalidator.On("InvalidateKeyByID", mock.Anything, testKeyID).Return(nil).Once()
	mockAuditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		payload, err := json.Marshal(event.NewValue)
		if err != nil {
			return false
		}

		return event.Action == domain.ActionRotate && !strings.Contains(string(payload), "raw_key")
	})).Return(nil).Once()

	handler.HandleRotateAPIKey(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "\"raw_key\"")
	assert.Contains(t, rec.Body.String(), "\"token_id\"")
}

func TestAPIKeyHandler_HandleReactivateAPIKey_Expired(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/api-keys/key1/reactivate", nil)
	req.SetPathValue("id", testKeyID)
	rec := httptest.NewRecorder()

	mockRepo := new(MockAPIKeyRepo)
	mockTokenRepo := new(MockAPIKeyTokenRepo)
	handler := NewAPIKeyHandler(mockRepo, mockTokenRepo, nil, nil)

	expired := time.Now().Add(-time.Hour)
	mockRepo.On("GetByID", mock.Anything, testKeyID).Return(&domain.APIKey{
		ID:        testKeyID,
		Name:      "Expired Key",
		IsActive:  false,
		ExpiresAt: &expired,
	}, nil).Once()

	handler.HandleReactivateAPIKey(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "expired api key cannot be reactivated")
}

func TestAPIKeyHandler_HandleRevokeAPIKey(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/api-keys/key1", nil)
	req.SetPathValue("id", testKeyID)
	rec := httptest.NewRecorder()

	mockRepo := new(MockAPIKeyRepo)
	mockTokenRepo := new(MockAPIKeyTokenRepo)
	invalidator := new(MockAPIKeyInvalidator)
	handler := NewAPIKeyHandler(mockRepo, mockTokenRepo, nil, invalidator)

	mockRepo.On("GetByID", mock.Anything, testKeyID).Return(&domain.APIKey{ID: testKeyID, Name: testKeyName, IsActive: true}, nil).Once()
	mockRepo.On("Revoke", mock.Anything, testKeyID).Return(nil).Once()
	invalidator.On("InvalidateKeyByID", mock.Anything, testKeyID).Return(nil).Once()

	handler.HandleRevokeAPIKey(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
