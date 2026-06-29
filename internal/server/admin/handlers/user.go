package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/auth"
	"github.com/google/uuid"
)

type UserHandler struct {
	repo domain.IdentityRepository
}

func NewUserHandler(repo domain.IdentityRepository) *UserHandler {
	return &UserHandler{repo: repo}
}

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

func (h *UserHandler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateUserRequest
	if err := helper.ReadJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		helper.WriteError(w, http.StatusBadRequest, "email is required")

		return
	}

	var passwordHash string
	if req.Password != "" {
		var err error
		passwordHash, err = auth.HashAdminPassword(req.Password)
		if err != nil {
			if errors.Is(err, auth.ErrWeakPassword) {
				helper.WriteError(w, http.StatusBadRequest, err.Error())
			} else {
				helper.WriteError(w, http.StatusInternalServerError, "failed to hash password")
			}

			return
		}
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

	if err := h.repo.CreateUser(r.Context(), user); err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			helper.WriteError(w, http.StatusConflict, "email is already taken")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to create user")
		}

		return
	}

	if len(req.RoleIDs) > 0 {
		if err := h.repo.SetUserRoles(r.Context(), userID, req.RoleIDs); err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to set user roles")

			return
		}
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "user:create",
		"entity_type", "user",
		"entity_id", userID,
		"new_value", user,
	)

	helper.WriteJSON(w, http.StatusCreated, dto.FromDomainUser(*user))
}

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
	if err := helper.ReadJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	oldUser := *user

	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			helper.WriteError(w, http.StatusBadRequest, "email cannot be empty")

			return
		}
		user.Email = email
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}

	if req.Password != nil {
		passwordHash, err := auth.HashAdminPassword(*req.Password)
		if err != nil {
			if errors.Is(err, auth.ErrWeakPassword) {
				helper.WriteError(w, http.StatusBadRequest, err.Error())
			} else {
				helper.WriteError(w, http.StatusInternalServerError, "failed to hash password")
			}

			return
		}
		user.PasswordHash = passwordHash
	}

	if req.IsActive != nil {
		if !*req.IsActive && user.IsActive {
			isLast, err := h.checkLastActiveOwner(r.Context(), id)
			if err != nil {
				helper.WriteError(w, http.StatusInternalServerError, "failed to verify active owner state")

				return
			}
			if isLast {
				helper.WriteError(w, http.StatusBadRequest, "cannot deactivate the last active owner")

				return
			}
		}
	}

	if req.RoleIDs != nil {
		ownerRole, err := h.repo.GetRoleByName(r.Context(), domain.RoleOwner)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to fetch owner role")

			return
		}

		if ownerRole != nil {
			hasOwnerNow := false
			currentRoles, err := h.repo.ListUserRoles(r.Context(), id)
			if err != nil {
				helper.WriteError(w, http.StatusInternalServerError, "failed to fetch current roles")

				return
			}
			for _, r := range currentRoles {
				if r.ID == ownerRole.ID {
					hasOwnerNow = true
					break
				}
			}

			isActiveNow := user.IsActive
			if req.IsActive != nil {
				isActiveNow = *req.IsActive
			}

			if hasOwnerNow && isActiveNow {
				hasOwnerNew := false
				for _, rID := range *req.RoleIDs {
					if rID == ownerRole.ID {
						hasOwnerNew = true
						break
					}
				}

				if !hasOwnerNew {
					isLast, err := h.checkLastActiveOwner(r.Context(), id)
					if err != nil {
						helper.WriteError(w, http.StatusInternalServerError, "failed to verify active owner state")

						return
					}
					if isLast {
						helper.WriteError(w, http.StatusBadRequest, "cannot remove the Owner role from the last active owner")

						return
					}
				}
			}
		}
	}

	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if req.IsSuperAdmin != nil {
		user.IsSuperAdmin = *req.IsSuperAdmin
	}

	if err := h.repo.UpdateUser(r.Context(), user); err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			helper.WriteError(w, http.StatusConflict, "email is already taken")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to update user")
		}

		return
	}

	if req.RoleIDs != nil {
		if err := h.repo.SetUserRoles(r.Context(), id, *req.RoleIDs); err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to set user roles")

			return
		}
	}

	if req.IsActive != nil && !*req.IsActive {
		_ = h.repo.RevokeUserSessions(r.Context(), id)
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "user:update",
		"entity_type", "user",
		"entity_id", id,
		"old_value", oldUser,
		"new_value", user,
	)

	roles, err := h.repo.ListUserRoles(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch updated user roles")

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
		if err := h.repo.UpdateUser(r.Context(), user); err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to deactivate user")

			return
		}

		_ = h.repo.RevokeUserSessions(r.Context(), id)
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "user:deactivate",
		"entity_type", "user",
		"entity_id", id,
	)

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) checkLastActiveOwner(ctx context.Context, userID string) (bool, error) {
	roles, err := h.repo.ListUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}

	hasOwnerRole := false
	for _, r := range roles {
		if r.Name == domain.RoleOwner {
			hasOwnerRole = true
			break
		}
	}
	if !hasOwnerRole {
		return false, nil
	}

	count, err := h.repo.CountActiveOwners(ctx)
	if err != nil {
		return false, err
	}

	return count <= 1, nil
}
