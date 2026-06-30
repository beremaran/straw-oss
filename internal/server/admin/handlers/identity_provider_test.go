package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/server/dto"
)

const (
	testProviderName = "Okta"
	testProviderType = "oidc"
)

func TestIdentityProviderHandler_HandleListIdentityProviders(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/identity-providers", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	handler := NewIdentityProviderHandler(mockRepo, nil)

	providers := []domain.AdminIdentityProvider{
		{ID: "idp1", Name: testProviderName, Type: testProviderType, IsEnabled: true},
		{ID: "idp2", Name: "Google", Type: testProviderType, IsEnabled: false},
	}
	mockRepo.On("ListIdentityProviders", mock.Anything).Return(providers, nil).Once()

	handler.HandleListIdentityProviders(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Len(t, response["data"], 2)
}

func TestIdentityProviderHandler_HandleCreateIdentityProvider(t *testing.T) {
	t.Run("Create success", func(t *testing.T) {
		body := `{"name":"Okta","type":"oidc","client_id":"client-id","client_secret_ref":"vault://secret"}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/identity-providers", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		auditRepo := new(MockManagementAuditRepo)
		handler := NewIdentityProviderHandler(mockRepo, auditRepo)

		mockRepo.On("CreateIdentityProvider", mock.Anything, mock.MatchedBy(func(p *domain.AdminIdentityProvider) bool {
			return p.Name == testProviderName && p.Type == testProviderType && p.ClientID == "client-id" && p.ClientSecretRef == "vault://secret"
		})).Return(nil).Once()
		auditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
			_, ok := event.NewValue.(dto.AdminIdentityProviderResponse)

			return event.Action == domain.ActionCreate && event.EntityType == "identity_provider" && ok
		})).Return(nil).Once()

		handler.HandleCreateIdentityProvider(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		auditRepo.AssertExpectations(t)
	})

	t.Run("Create fails on plaintext secret config", func(t *testing.T) {
		body := `{"name":"Okta","type":"oidc","config":{"client_secret":"plaintext"}}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/identity-providers", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewIdentityProviderHandler(mockRepo, nil)

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
	handler := NewIdentityProviderHandler(mockRepo, nil)

	provider := &domain.AdminIdentityProvider{ID: "idp1", Name: testProviderName, Type: testProviderType, IsEnabled: true}
	mockRepo.On("GetIdentityProviderByID", mock.Anything, "idp1").Return(provider, nil).Once()
	mockRepo.On("DisableIdentityProvider", mock.Anything, "idp1").Return(nil).Once()

	handler.HandleDeleteIdentityProvider(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
