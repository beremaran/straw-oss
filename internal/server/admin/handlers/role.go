package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/google/uuid"
)

type RoleHandler struct {
	repo domain.IdentityRepository
}

func NewRoleHandler(repo domain.IdentityRepository) *RoleHandler {
	return &RoleHandler{repo: repo}
}

func (h *RoleHandler) HandleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.repo.ListRoles(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list roles")

		return
	}

	data := make([]dto.AdminRoleResponse, len(roles))
	for i, role := range roles {
		data[i] = dto.FromDomainRole(role)
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListRolesResponse{
		Data: data,
	})
}

func (h *RoleHandler) HandleCreateRole(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateRoleRequest
	if err := helper.ReadJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		helper.WriteError(w, http.StatusBadRequest, "role name is required")

		return
	}

	roleID := uuid.New().String()
	role := &domain.AdminRole{
		ID:          roleID,
		Name:        name,
		Description: req.Description,
		IsBuiltin:   false,
		Permissions: req.Permissions,
	}

	if err := h.repo.CreateRole(r.Context(), role); err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			helper.WriteError(w, http.StatusConflict, "role name already exists")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to create role")
		}

		return
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "role:create",
		"entity_type", "role",
		"entity_id", roleID,
		"new_value", role,
	)

	helper.WriteJSON(w, http.StatusCreated, dto.FromDomainRole(*role))
}

func (h *RoleHandler) HandleUpdateRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	role, err := h.repo.GetRoleByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch role")

		return
	}
	if role == nil {
		helper.WriteError(w, http.StatusNotFound, "role not found")

		return
	}

	if role.IsBuiltin {
		helper.WriteError(w, http.StatusBadRequest, "cannot update a built-in role")

		return
	}

	var req dto.UpdateRoleRequest
	if err := helper.ReadJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		helper.WriteError(w, http.StatusBadRequest, "role name is required")

		return
	}

	oldRole := *role
	role.Name = name
	role.Description = req.Description
	role.Permissions = req.Permissions

	if err := h.repo.UpdateRole(r.Context(), role); err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			helper.WriteError(w, http.StatusConflict, "role name already exists")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to update role")
		}

		return
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "role:update",
		"entity_type", "role",
		"entity_id", id,
		"old_value", oldRole,
		"new_value", role,
	)

	helper.WriteJSON(w, http.StatusOK, dto.FromDomainRole(*role))
}

func (h *RoleHandler) HandleDeleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	role, err := h.repo.GetRoleByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch role")

		return
	}
	if role == nil {
		helper.WriteError(w, http.StatusNotFound, "role not found")

		return
	}

	if role.IsBuiltin {
		helper.WriteError(w, http.StatusBadRequest, "cannot delete a built-in role")

		return
	}

	if err := h.repo.DeleteRole(r.Context(), id); err != nil {
		if errors.Is(err, postgres.ErrBuiltinRoleProtected) {
			helper.WriteError(w, http.StatusBadRequest, "cannot delete a built-in role")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to delete role")
		}

		return
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "role:delete",
		"entity_type", "role",
		"entity_id", id,
	)

	w.WriteHeader(http.StatusNoContent)
}
