package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beremaran/straw/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestApiKeyHandler_HandleListApiKeys(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/api-keys?page=1&limit=10", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	handler := NewApiKeyHandler(mockRepo, nil)

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
	body := `{"name":"New Key", "scopes":["target:*"]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	handler := NewApiKeyHandler(mockRepo, nil)

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

func TestApiKeyHandler_HandleRevokeApiKey(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/api-keys/key1", nil)
	req.SetPathValue("id", "key1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockApiKeyRepo)
	handler := NewApiKeyHandler(mockRepo, nil)

	mockRepo.On("Revoke", mock.Anything, "key1").Return(nil).Once()

	handler.HandleRevokeApiKey(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
