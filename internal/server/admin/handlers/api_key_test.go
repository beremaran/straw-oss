package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestApiKeyHandler_HandleListApiKeys(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/api-keys?page=1&limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mockRepo := new(MockApiKeyRepo)
	handler := NewApiKeyHandler(mockRepo)

	keys := []domain.ApiKey{
		{ID: "key1", Name: "Key 1"},
		{ID: "key2", Name: "Key 2"},
	}
	mockRepo.On("List", mock.Anything, 10, 0).Return(keys, 2, nil).Once()

	err := handler.HandleListApiKeys(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), response["total"])
}

func TestApiKeyHandler_HandleCreateApiKey(t *testing.T) {
	e := echo.New()
	body := `{"name":"New Key", "scopes":["target:*"]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api-keys", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mockRepo := new(MockApiKeyRepo)
	handler := NewApiKeyHandler(mockRepo)

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(k *domain.ApiKey) bool {
		return k.Name == "New Key" && len(k.Scopes) == 1 && k.Scopes[0] == "target:*"
	})).Return(nil).Once()

	err := handler.HandleCreateApiKey(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEmpty(t, response["raw_key"])
	assert.Equal(t, "New Key", response["name"])
}

func TestApiKeyHandler_HandleRevokeApiKey(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/admin/api-keys/key1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/admin/api-keys/:id")
	c.SetParamNames("id")
	c.SetParamValues("key1")

	mockRepo := new(MockApiKeyRepo)
	handler := NewApiKeyHandler(mockRepo)

	mockRepo.On("Revoke", mock.Anything, "key1").Return(nil).Once()

	err := handler.HandleRevokeApiKey(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
