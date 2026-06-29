package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestIdentityProviderHandler_HandleListIdentityProviders(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/identity-providers", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	handler := NewIdentityProviderHandler(mockRepo)

	providers := []domain.AdminIdentityProvider{
		{ID: "idp1", Name: "Okta", Type: "oidc", IsEnabled: true},
		{ID: "idp2", Name: "Google", Type: "oidc", IsEnabled: false},
	}
	mockRepo.On("ListIdentityProviders", mock.Anything).Return(providers, nil).Once()

	handler.HandleListIdentityProviders(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 2)
}

func TestIdentityProviderHandler_HandleCreateIdentityProvider(t *testing.T) {
	t.Run("Create success", func(t *testing.T) {
		body := `{"name":"Okta","type":"oidc","client_id":"client-id","client_secret_ref":"vault://secret"}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/identity-providers", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewIdentityProviderHandler(mockRepo)

		mockRepo.On("CreateIdentityProvider", mock.Anything, mock.MatchedBy(func(p *domain.AdminIdentityProvider) bool {
			return p.Name == "Okta" && p.Type == "oidc" && p.ClientID == "client-id" && p.ClientSecretRef == "vault://secret"
		})).Return(nil).Once()

		handler.HandleCreateIdentityProvider(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("Create fails on plaintext secret config", func(t *testing.T) {
		body := `{"name":"Okta","type":"oidc","config":{"client_secret":"plaintext"}}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/identity-providers", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewIdentityProviderHandler(mockRepo)

		mockRepo.On("CreateIdentityProvider", mock.Anything, mock.Anything).Return(postgres.ErrPlaintextProviderSecret).Once()

		handler.HandleCreateIdentityProvider(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "identity provider config must not contain secrets")
	})
}

func TestIdentityProviderHandler_HandleDeleteIdentityProvider(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/identity-providers/idp1", nil)
	req.SetPathValue("id", "idp1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	handler := NewIdentityProviderHandler(mockRepo)

	provider := &domain.AdminIdentityProvider{ID: "idp1", Name: "Okta", Type: "oidc", IsEnabled: true}
	mockRepo.On("GetIdentityProviderByID", mock.Anything, "idp1").Return(provider, nil).Once()
	mockRepo.On("DisableIdentityProvider", mock.Anything, "idp1").Return(nil).Once()

	handler.HandleDeleteIdentityProvider(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
