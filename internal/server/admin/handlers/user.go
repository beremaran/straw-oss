package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/auth"
)

var (
	// ErrVerifyOwner indicates failure to verify active owner state.
	ErrVerifyOwner = errors.New("failed to verify active owner state")
	// ErrDeactivateLastOwner indicates cannot deactivate the last active owner.
	ErrDeactivateLastOwner = errors.New("cannot deactivate the last active owner")
	// ErrFetchOwnerRole indicates failure to fetch owner role.
	ErrFetchOwnerRole = errors.New("failed to fetch owner role")
	// ErrFetchCurrentRoles indicates failure to fetch current roles.
	ErrFetchCurrentRoles = errors.New("failed to fetch current roles")
	// ErrRemoveLastOwnerRole indicates cannot remove the Owner role from the last active owner.
	ErrRemoveLastOwnerRole = errors.New("cannot remove the Owner role from the last active owner")
	// ErrEmailEmpty indicates email cannot be empty.
	ErrEmailEmpty = errors.New("email cannot be empty")
)

// UserHandler manages admin user operations.
type UserHandler struct {
	repo      domain.IdentityRepository
	auditRepo domain.ManagementAuditRepository
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(repo domain.IdentityRepository, auditRepo domain.ManagementAuditRepository) *UserHandler {
	return &UserHandler{repo: repo, auditRepo: auditRepo}
}

// HandleListUsers lists all admin users.
func (h *UserHandler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	users, total, err := h.repo.ListUsers(r.Context(), limit, offset)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list users")

		return
	}

	data := make([]dto.AdminUserResponse, len(users))
	for i, u := range users {
		data[i] = dto.FromDomainUser(u)
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListUsersResponse{
		Data:  data,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleCreateUser creates a new admin user.
func (h *UserHandler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateUserRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		helper.WriteError(w, http.StatusBadRequest, "email is required")

		return
	}

	passwordHash, err := hashOptionalAdminPassword(req.Password)
	if err != nil {
		writePasswordHashError(w, err)

		return
	}

	userID := uuid.New().String()
	user := &domain.AdminUser{
		ID:           userID,
		Email:        email,
		DisplayName:  req.DisplayName,
		PasswordHash: passwordHash,
		IsActive:     req.IsActive,
		IsSuperAdmin: req.IsSuperAdmin,
	}

	err = h.repo.CreateUser(r.Context(), user)
	if err != nil {
		writeConflictOrServerError(w, err, "email is already taken", "failed to create user")

		return
	}

	if len(req.RoleIDs) > 0 {
		err = h.repo.SetUserRoles(r.Context(), userID, req.RoleIDs)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to set user roles")

			return
		}
	}

	recordAuditEvent(r, h.auditRepo, domain.ActionCreate, "user", userID, nil, dto.FromDomainUser(*user))

	helper.WriteJSON(w, http.StatusCreated, dto.FromDomainUser(*user))
}

// HandleGetUser retrieves a single admin user.
func (h *UserHandler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	user, err := h.repo.GetUserByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch user")

		return
	}

	if user == nil {
		helper.WriteError(w, http.StatusNotFound, "user not found")

		return
	}

	roles, err := h.repo.ListUserRoles(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch user roles")

		return
	}

	rolesDTO := make([]dto.AdminRoleResponse, len(roles))
	for i, role := range roles {
		rolesDTO[i] = dto.FromDomainRole(role)
	}

	helper.WriteJSON(w, http.StatusOK, dto.UserDetailResponse{
		ID:           user.ID,
		Email:        user.Email,
		DisplayName:  user.DisplayName,
		IsActive:     user.IsActive,
		IsSuperAdmin: user.IsSuperAdmin,
		LastLoginAt:  user.LastLoginAt,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Roles:        rolesDTO,
	})
}

// HandleUpdateUser updates an admin user.
func (h *UserHandler) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	user, err := h.repo.GetUserByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch user")

		return
	}

	if user == nil {
		helper.WriteError(w, http.StatusNotFound, "user not found")

		return
	}

	var req dto.UpdateUserRequest

	err = helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	oldUser := *user

	err = h.applyUserUpdate(r.Context(), id, user, req)
	if err != nil {
		writeUserUpdateError(w, err)

		return
	}

	err = h.repo.UpdateUser(r.Context(), user)
	if err != nil {
		writeConflictOrServerError(w, err, "email is already taken", "failed to update user")

		return
	}

	err = h.saveUpdatedUserRoles(r.Context(), id, req)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to set user roles")

		return
	}

	if deactivatesUser(req) {
		_ = h.repo.RevokeUserSessions(r.Context(), id)
	}

	recordAuditEvent(r, h.auditRepo, domain.ActionUpdate, "user", id, dto.FromDomainUser(oldUser), dto.FromDomainUser(*user))
	h.writeUserDetail(r.Context(), w, id, user, "failed to fetch updated user roles")
}

// HandleDeactivateUser deactivates an admin user.
func (h *UserHandler) HandleDeactivateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	user, err := h.repo.GetUserByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch user")

		return
	}

	if user == nil {
		helper.WriteError(w, http.StatusNotFound, "user not found")

		return
	}

	oldUser := *user
	if user.IsActive {
		isLast, err := h.checkLastActiveOwner(r.Context(), id)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to verify active owner state")

			return
		}

		if isLast {
			helper.WriteError(w, http.StatusBadRequest, "cannot deactivate the last active owner")

			return
		}

		user.IsActive = false

		err = h.repo.UpdateUser(r.Context(), user)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to deactivate user")

			return
		}

		_ = h.repo.RevokeUserSessions(r.Context(), id)
	}

	recordAuditEvent(r, h.auditRepo, domain.ActionDelete, "user", id, dto.FromDomainUser(oldUser), dto.FromDomainUser(*user))

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) applyUserUpdate(ctx context.Context, id string, user *domain.AdminUser, req dto.UpdateUserRequest) error {
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			return ErrEmailEmpty
		}

		user.Email = email
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}

	if req.Password != nil {
		passwordHash, err := auth.HashAdminPassword(*req.Password)
		if err != nil {
			return fmt.Errorf("hashing admin password: %w", err)
		}

		user.PasswordHash = passwordHash
	}

	err := h.ensureCanSetActive(ctx, id, user, req)
	if err != nil {
		return err
	}

	err = h.validateRoleUpdate(ctx, id, user, req)
	if err != nil {
		return err
	}

	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	if req.IsSuperAdmin != nil {
		user.IsSuperAdmin = *req.IsSuperAdmin
	}

	return nil
}

func (h *UserHandler) ensureCanSetActive(ctx context.Context, id string, user *domain.AdminUser, req dto.UpdateUserRequest) error {
	if req.IsActive == nil || *req.IsActive || !user.IsActive {
		return nil
	}

	isLast, err := h.checkLastActiveOwner(ctx, id)
	if err != nil {
		return ErrVerifyOwner
	}

	if isLast {
		return ErrDeactivateLastOwner
	}

	return nil
}

func (h *UserHandler) writeUserDetail(ctx context.Context, w http.ResponseWriter, id string, user *domain.AdminUser, roleErrMsg string) {
	roles, err := h.repo.ListUserRoles(ctx, id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, roleErrMsg)

		return
	}

	rolesDTO := make([]dto.AdminRoleResponse, len(roles))
	for i, role := range roles {
		rolesDTO[i] = dto.FromDomainRole(role)
	}

	helper.WriteJSON(w, http.StatusOK, dto.UserDetailResponse{
		ID:           user.ID,
		Email:        user.Email,
		DisplayName:  user.DisplayName,
		IsActive:     user.IsActive,
		IsSuperAdmin: user.IsSuperAdmin,
		LastLoginAt:  user.LastLoginAt,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Roles:        rolesDTO,
	})
}

func (h *UserHandler) saveUpdatedUserRoles(ctx context.Context, id string, req dto.UpdateUserRequest) error {
	if req.RoleIDs == nil {
		return nil
	}

	return fmt.Errorf("setting user roles: %w", h.repo.SetUserRoles(ctx, id, *req.RoleIDs))
}

func (h *UserHandler) checkLastActiveOwner(ctx context.Context, userID string) (bool, error) {
	roles, err := h.repo.ListUserRoles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("listing user roles: %w", err)
	}

	if !hasRoleName(roles, domain.RoleOwner) {
		return false, nil
	}

	count, err := h.repo.CountActiveOwners(ctx)
	if err != nil {
		return false, fmt.Errorf("counting active owners: %w", err)
	}

	return count <= 1, nil
}

func (h *UserHandler) validateRoleUpdate(ctx context.Context, id string, user *domain.AdminUser, req dto.UpdateUserRequest) error {
	if req.RoleIDs == nil {
		return nil
	}

	ownerRole, err := h.repo.GetRoleByName(ctx, domain.RoleOwner)
	if err != nil {
		return ErrFetchOwnerRole
	}

	if ownerRole == nil {
		return nil
	}

	currentRoles, err := h.repo.ListUserRoles(ctx, id)
	if err != nil {
		return ErrFetchCurrentRoles
	}

	if !hasRoleID(currentRoles, ownerRole.ID) || !userActiveAfterUpdate(user, req) || stringSliceContains(*req.RoleIDs, ownerRole.ID) {
		return nil
	}

	isLast, err := h.checkLastActiveOwner(ctx, id)
	if err != nil {
		return ErrVerifyOwner
	}

	if isLast {
		return ErrRemoveLastOwnerRole
	}

	return nil
}

func hashOptionalAdminPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}

	hash, err := auth.HashAdminPassword(password)
	if err != nil {
		return "", fmt.Errorf("hashing admin password: %w", err)
	}

	return hash, nil
}

func writePasswordHashError(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrWeakPassword) {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	helper.WriteError(w, http.StatusInternalServerError, "failed to hash password")
}

func writeUserUpdateError(w http.ResponseWriter, err error) {
	if isInternalUserUpdateError(err) {
		helper.WriteError(w, http.StatusInternalServerError, err.Error())

		return
	}

	if errors.Is(err, auth.ErrWeakPassword) {
		writePasswordHashError(w, err)

		return
	}

	helper.WriteError(w, http.StatusBadRequest, err.Error())
}

func isInternalUserUpdateError(err error) bool {
	return errors.Is(err, ErrVerifyOwner) || errors.Is(err, ErrFetchOwnerRole) || errors.Is(err, ErrFetchCurrentRoles)
}

func hasRoleName(roles []domain.AdminRole, name string) bool {
	for _, role := range roles {
		if role.Name == name {
			return true
		}
	}

	return false
}

func hasRoleID(roles []domain.AdminRole, id string) bool {
	for _, role := range roles {
		if role.ID == id {
			return true
		}
	}

	return false
}

func stringSliceContains(values []string, value string) bool {
	return slices.Contains(values, value)
}

func userActiveAfterUpdate(user *domain.AdminUser, req dto.UpdateUserRequest) bool {
	if req.IsActive != nil {
		return *req.IsActive
	}

	return user.IsActive
}

func deactivatesUser(req dto.UpdateUserRequest) bool {
	return req.IsActive != nil && !*req.IsActive
}
