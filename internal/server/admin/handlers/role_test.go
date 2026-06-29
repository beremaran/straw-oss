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

func TestRoleHandler_HandleListRoles(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/roles", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	handler := NewRoleHandler(mockRepo)

	roles := []domain.AdminRole{
		{ID: "role1", Name: "Role 1", IsBuiltin: true},
		{ID: "role2", Name: "Role 2", IsBuiltin: false},
	}
	mockRepo.On("ListRoles", mock.Anything).Return(roles, nil).Once()

	handler.HandleListRoles(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 2)
}

func TestRoleHandler_HandleCreateRole(t *testing.T) {
	body := `{"name":"Custom Operator","description":"My custom role","permissions":["routing_rules:read"]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/roles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	handler := NewRoleHandler(mockRepo)

	mockRepo.On("CreateRole", mock.Anything, mock.MatchedBy(func(r *domain.AdminRole) bool {
		return r.Name == "Custom Operator" && len(r.Permissions) == 1 && r.Permissions[0] == "routing_rules:read"
	})).Return(nil).Once()

	handler.HandleCreateRole(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestRoleHandler_HandleUpdateRole_BuiltinProtection(t *testing.T) {
	t.Run("Update built-in role is blocked", func(t *testing.T) {
		body := `{"name":"Owner","permissions":[]}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/roles/builtin-id", strings.NewReader(body))
		req.SetPathValue("id", "builtin-id")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewRoleHandler(mockRepo)

		role := &domain.AdminRole{ID: "builtin-id", Name: "Owner", IsBuiltin: true}
		mockRepo.On("GetRoleByID", mock.Anything, "builtin-id").Return(role, nil).Once()

		handler.HandleUpdateRole(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "cannot update a built-in role")
	})
}

func TestRoleHandler_HandleDeleteRole_BuiltinProtection(t *testing.T) {
	t.Run("Delete built-in role is blocked in handler", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/roles/builtin-id", nil)
		req.SetPathValue("id", "builtin-id")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewRoleHandler(mockRepo)

		role := &domain.AdminRole{ID: "builtin-id", Name: "Owner", IsBuiltin: true}
		mockRepo.On("GetRoleByID", mock.Anything, "builtin-id").Return(role, nil).Once()

		handler.HandleDeleteRole(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "cannot delete a built-in role")
	})

	t.Run("Delete custom role checks DB protection", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/roles/custom-id", nil)
		req.SetPathValue("id", "custom-id")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewRoleHandler(mockRepo)

		role := &domain.AdminRole{ID: "custom-id", Name: "Custom Role", IsBuiltin: false}
		mockRepo.On("GetRoleByID", mock.Anything, "custom-id").Return(role, nil).Once()
		mockRepo.On("DeleteRole", mock.Anything, "custom-id").Return(postgres.ErrBuiltinRoleProtected).Once()

		handler.HandleDeleteRole(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "cannot delete a built-in role")
	})
}
