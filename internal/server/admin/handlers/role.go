package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

// RoleHandler manages admin role operations.
type RoleHandler struct {
	repo      domain.IdentityRepository
	auditRepo domain.ManagementAuditRepository
}

// NewRoleHandler creates a new RoleHandler.
func NewRoleHandler(repo domain.IdentityRepository, auditRepo domain.ManagementAuditRepository) *RoleHandler {
	return &RoleHandler{repo: repo, auditRepo: auditRepo}
}

// HandleListRoles lists all admin roles.
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

// HandleCreateRole creates a new admin role.
func (h *RoleHandler) HandleCreateRole(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateRoleRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
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

	err = h.repo.CreateRole(r.Context(), role)
	if err != nil {
		writeConflictOrServerError(w, err, "role name already exists", "failed to create role")

		return
	}

	recordAuditEvent(r, h.auditRepo, domain.ActionCreate, "role", roleID, nil, dto.FromDomainRole(*role))

	helper.WriteJSON(w, http.StatusCreated, dto.FromDomainRole(*role))
}

// HandleUpdateRole updates an admin role.
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

	err = helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		helper.WriteError(w, http.StatusBadRequest, "role name is required")

		return
	}

	oldRole := *role
	applyRoleUpdate(role, req, name)

	err = h.repo.UpdateRole(r.Context(), role)
	if err != nil {
		writeConflictOrServerError(w, err, "role name already exists", "failed to update role")

		return
	}

	recordAuditEvent(r, h.auditRepo, domain.ActionUpdate, "role", id, dto.FromDomainRole(oldRole), dto.FromDomainRole(*role))

	helper.WriteJSON(w, http.StatusOK, dto.FromDomainRole(*role))
}

// HandleDeleteRole deletes an admin role.
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

	err = h.repo.DeleteRole(r.Context(), id)
	if err != nil {
		if errors.Is(err, postgres.ErrBuiltinRoleProtected) {
			helper.WriteError(w, http.StatusBadRequest, "cannot delete a built-in role")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to delete role")
		}

		return
	}

	recordAuditEvent(r, h.auditRepo, domain.ActionDelete, "role", id, dto.FromDomainRole(*role), nil)

	w.WriteHeader(http.StatusNoContent)
}

func applyRoleUpdate(role *domain.AdminRole, req dto.UpdateRoleRequest, name string) {
	role.Name = name
	role.Description = req.Description
	role.Permissions = req.Permissions
}
