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
	"github.com/beremaran/straw/internal/server/dto"
)

func TestUserHandler_HandleListUsers(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/users?page=1&limit=10", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	handler := NewUserHandler(mockRepo, nil)

	users := []domain.AdminUser{
		{ID: "user1", Email: "user1@test.com"},
		{ID: "user2", Email: "user2@test.com"},
	}
	mockRepo.On("ListUsers", mock.Anything, 10, 0).Return(users, 2, nil).Once()

	handler.HandleListUsers(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.InEpsilon(t, float64(2), response["total"], 0.01)
}

func TestUserHandler_HandleCreateUser(t *testing.T) {
	body := `{"email":"new@test.com","display_name":"New User","password":"password123","is_active":true}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	auditRepo := new(MockManagementAuditRepo)
	handler := NewUserHandler(mockRepo, auditRepo)

	mockRepo.On("CreateUser", mock.Anything, mock.MatchedBy(func(u *domain.AdminUser) bool {
		return u.Email == "new@test.com" && u.DisplayName == "New User"
	})).Return(nil).Once()
	auditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		_, ok := event.NewValue.(dto.AdminUserResponse)

		return event.Action == domain.ActionCreate && event.EntityType == "user" && ok
	})).Return(nil).Once()

	handler.HandleCreateUser(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	auditRepo.AssertExpectations(t)
}

func TestUserHandler_HandleGetUser(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/users/user1", nil)
	req.SetPathValue("id", "user1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockIdentityRepo)
	handler := NewUserHandler(mockRepo, nil)

	user := &domain.AdminUser{ID: "user1", Email: "user1@test.com", DisplayName: "User One", IsActive: true}
	mockRepo.On("GetUserByID", mock.Anything, "user1").Return(user, nil).Once()
	mockRepo.On("ListUserRoles", mock.Anything, "user1").Return([]domain.AdminRole{}, nil).Once()

	handler.HandleGetUser(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestUserHandler_HandleUpdateUser_LastActiveOwnerProtection(t *testing.T) {
	t.Run("Deactivate last active owner", func(t *testing.T) {
		body := `{"is_active":false}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/users/owner1", strings.NewReader(body))
		req.SetPathValue("id", "owner1")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewUserHandler(mockRepo, nil)

		user := &domain.AdminUser{ID: "owner1", Email: "owner1@test.com", DisplayName: "Owner One", IsActive: true}
		mockRepo.On("GetUserByID", mock.Anything, "owner1").Return(user, nil).Once()

		roles := []domain.AdminRole{{ID: "r1", Name: domain.RoleOwner}}
		mockRepo.On("ListUserRoles", mock.Anything, "owner1").Return(roles, nil).Once()
		mockRepo.On("CountActiveOwners", mock.Anything).Return(1, nil).Once()

		handler.HandleUpdateUser(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "cannot deactivate the last active owner")
	})

	t.Run("Remove Owner role from last active owner", func(t *testing.T) {
		body := `{"role_ids":[]}`
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/users/owner1", strings.NewReader(body))
		req.SetPathValue("id", "owner1")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mockRepo := new(MockIdentityRepo)
		handler := NewUserHandler(mockRepo, nil)

		user := &domain.AdminUser{ID: "owner1", Email: "owner1@test.com", DisplayName: "Owner One", IsActive: true}
		mockRepo.On("GetUserByID", mock.Anything, "owner1").Return(user, nil).Once()

		ownerRole := &domain.AdminRole{ID: "owner-role-id", Name: domain.RoleOwner}
		mockRepo.On("GetRoleByName", mock.Anything, domain.RoleOwner).Return(ownerRole, nil).Once()

		currentRoles := []domain.AdminRole{*ownerRole}
		mockRepo.On("ListUserRoles", mock.Anything, "owner1").Return(currentRoles, nil).Twice()
		mockRepo.On("CountActiveOwners", mock.Anything).Return(1, nil).Once()

		handler.HandleUpdateUser(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "cannot remove the Owner role from the last active owner")
	})
}
