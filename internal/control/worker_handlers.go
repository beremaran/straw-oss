package control

import (
	"context"
	"fmt"
	"net/http"
)

// Worker admin HTTP surface (docs/planning/26-config-management-api-surface.md,
// "Runtime Admin Endpoints"). Global actions require a platform-scoped
// system_admin key; tenant actions derive tenant identity from a tenant-scoped
// key and affect only that tenant's routing eligibility. A global disable
// always wins over a tenant enable (enforced in WorkerRegistry.EligibleForTenant).

// workerAdminView is the JSON read model returned by GET /workers. Platform
// callers receive session_id and NATS subjects; tenant callers never do.
type workerAdminView struct {
	WorkerID         string            `json:"worker_id"`
	RuntimeState     string            `json:"runtime_state"`
	GlobalAdminState string            `json:"global_admin_state"`
	GlobalDraining   bool              `json:"global_draining"`
	TenantAdminState map[string]string `json:"tenant_admin_state,omitempty"`
	TenantDraining   map[string]bool   `json:"tenant_draining,omitempty"`
	SessionID        string            `json:"session_id,omitempty"`
	ExecutorType     string            `json:"executor_type,omitempty"`
	AssignSubject    string            `json:"assign_subject,omitempty"`
}

func adminMapToStrings(in map[string]AdminState) map[string]string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = string(v)
	}

	return out
}

// ListWorkers handles GET /workers. Platform keys see all workers with full
// state; tenant keys see only workers eligible for their tenant, without
// session_id or NATS subjects.
func (h *AdminHandlers) ListWorkers(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin, RoleTenantAdmin, RoleOperator)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	var views []WorkerView
	if identity.IsPlatform() {
		views = h.Workers.ListWorkersPlatform()
	} else {
		views = h.Workers.ListWorkersForTenant(identity.TenantID)
	}

	out := make([]workerAdminView, 0, len(views))
	for _, v := range views {
		row := workerAdminView{
			WorkerID:         v.WorkerID,
			RuntimeState:     string(v.RuntimeState),
			GlobalAdminState: string(v.GlobalAdminState),
			GlobalDraining:   v.GlobalDraining,
			TenantAdminState: adminMapToStrings(v.TenantAdmin),
			TenantDraining:   v.TenantDrain,
			ExecutorType:     v.ExecutorType,
		}
		if identity.IsPlatform() {
			row.SessionID = v.SessionID
			row.AssignSubject = v.AssignSubject
		}

		out = append(out, row)
	}

	writeJSON(w, http.StatusOK, out)
}

// requirePlatformSystemAdmin authenticates the caller and requires a
// platform-scoped system_admin key for global worker actions.
func (h *AdminHandlers) requirePlatformSystemAdmin(r *http.Request) (Identity, error) {
	identity, err := h.authenticate(r)
	if err != nil {
		return Identity{}, err
	}

	err = RequirePlatformScope(identity)
	if err != nil {
		return Identity{}, err
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		return Identity{}, err
	}

	return identity, nil
}

// requireTenantRole authenticates the caller and requires a tenant-scoped key
// with one of the allowed roles for tenant worker actions.
func (h *AdminHandlers) requireTenantRole(r *http.Request, allowed ...Role) (Identity, error) {
	identity, err := h.authenticate(r)
	if err != nil {
		return Identity{}, err
	}

	err = RequireTenantScope(identity)
	if err != nil {
		return Identity{}, err
	}

	err = RequireRole(identity, allowed...)
	if err != nil {
		return Identity{}, err
	}

	return identity, nil
}

// globalWorkerAction is the shared body for the four global admin endpoints.
// registryApply updates the in-memory runtime state; durable, when non-nil and
// a durable store is wired, persists the decision (disable/enable only — drain
// is runtime-only per docs/planning/21).
func (h *AdminHandlers) globalWorkerAction(w http.ResponseWriter, r *http.Request, action string, registryApply func(workerID string), durable func(ctx context.Context, workerID string, actor ConfigActor) (bool, error)) {
	identity, err := h.requirePlatformSystemAdmin(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	workerID := r.PathValue("worker_id")
	registryApply(workerID)

	if durable != nil && (h.ConfigWrites != nil || h.WorkerAdmin != nil) {
		audited, err := durable(r.Context(), workerID, configActor(identity))
		if err != nil {
			WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

			return
		}

		if audited {
			writeJSON(w, http.StatusOK, h.workerStateResponse(identity, workerID))

			return
		}
	}

	recordAudit(r.Context(), h.Audit, identity, "worker", workerID, action)
	writeJSON(w, http.StatusOK, h.workerStateResponse(identity, workerID))
}

// DisableWorker handles POST /workers/{worker_id}/disable.
func (h *AdminHandlers) DisableWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "disable",
		func(id string) { h.Workers.SetGlobalAdmin(id, AdminDisabled) },
		func(ctx context.Context, id string, actor ConfigActor) (bool, error) {
			return h.setGlobalWorkerAdmin(ctx, id, true, actor)
		})
}

// EnableWorker handles POST /workers/{worker_id}/enable.
func (h *AdminHandlers) EnableWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "enable",
		func(id string) { h.Workers.SetGlobalAdmin(id, AdminEnabled) },
		func(ctx context.Context, id string, actor ConfigActor) (bool, error) {
			return h.setGlobalWorkerAdmin(ctx, id, false, actor)
		})
}

// DrainWorker handles POST /workers/{worker_id}/drain.
func (h *AdminHandlers) DrainWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "drain", func(id string) { h.Workers.SetGlobalDrain(id, true) }, nil)
}

// UndrainWorker handles POST /workers/{worker_id}/undrain.
func (h *AdminHandlers) UndrainWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "undrain", func(id string) { h.Workers.SetGlobalDrain(id, false) }, nil)
}

// tenantWorkerAction is the shared body for the four tenant admin endpoints.
// For durable disable/enable it also bumps the tenant config version so tenant
// snapshots invalidate, matching the other tenant-scoped config writes.
func (h *AdminHandlers) tenantWorkerAction(w http.ResponseWriter, r *http.Request, action string, allowed []Role, registryApply func(workerID, tenantID string), durable func(ctx context.Context, tenantID, workerID string, actor ConfigActor) (bool, error)) {
	identity, err := h.requireTenantRole(r, allowed...)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	workerID := r.PathValue("worker_id")
	registryApply(workerID, identity.TenantID)

	if durable != nil && (h.ConfigWrites != nil || h.WorkerAdmin != nil) {
		audited, err := durable(r.Context(), identity.TenantID, workerID, configActor(identity))
		if err != nil {
			WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

			return
		}

		if audited {
			writeJSON(w, http.StatusOK, h.workerStateResponse(identity, workerID))

			return
		}

		_, err = h.bumpTenantVersion(r.Context(), identity.TenantID, nil)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

			return
		}
	}

	recordAudit(r.Context(), h.Audit, identity, "worker", workerID, action)
	writeJSON(w, http.StatusOK, h.workerStateResponse(identity, workerID))
}

// TenantDisableWorker handles POST /workers/{worker_id}/tenant-disable.
func (h *AdminHandlers) TenantDisableWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_disable", []Role{RoleTenantAdmin},
		func(id, tid string) { h.Workers.SetTenantAdmin(id, tid, AdminDisabled) },
		func(ctx context.Context, tid, id string, actor ConfigActor) (bool, error) {
			return h.setTenantWorkerOverride(ctx, tid, id, true, actor)
		})
}

// TenantEnableWorker handles POST /workers/{worker_id}/tenant-enable.
func (h *AdminHandlers) TenantEnableWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_enable", []Role{RoleTenantAdmin},
		func(id, tid string) { h.Workers.SetTenantAdmin(id, tid, AdminEnabled) },
		func(ctx context.Context, tid, id string, actor ConfigActor) (bool, error) {
			return h.setTenantWorkerOverride(ctx, tid, id, false, actor)
		})
}

// TenantDrainWorker handles POST /workers/{worker_id}/tenant-drain.
func (h *AdminHandlers) TenantDrainWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_drain", []Role{RoleTenantAdmin, RoleOperator},
		func(id, tid string) { h.Workers.SetTenantDrain(id, tid, true) }, nil)
}

// TenantUndrainWorker handles POST /workers/{worker_id}/tenant-undrain.
func (h *AdminHandlers) TenantUndrainWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_undrain", []Role{RoleTenantAdmin, RoleOperator},
		func(id, tid string) { h.Workers.SetTenantDrain(id, tid, false) }, nil)
}

func (h *AdminHandlers) setGlobalWorkerAdmin(ctx context.Context, workerID string, disabled bool, actor ConfigActor) (bool, error) {
	if h.ConfigWrites != nil {
		err := h.ConfigWrites.SetGlobalWorkerAdminConfig(ctx, workerID, disabled, "", actor)
		if err != nil {
			return true, fmt.Errorf("persist global worker admin config: %w", err)
		}

		return true, nil
	}

	err := h.WorkerAdmin.SetGlobalWorkerAdmin(ctx, workerID, disabled, "")
	if err != nil {
		return false, fmt.Errorf("persist global worker admin: %w", err)
	}

	return false, nil
}

func (h *AdminHandlers) setTenantWorkerOverride(ctx context.Context, tenantID, workerID string, disabled bool, actor ConfigActor) (bool, error) {
	if h.ConfigWrites != nil {
		err := h.ConfigWrites.SetTenantWorkerOverrideConfig(ctx, tenantID, workerID, disabled, "", actor)
		if err != nil {
			return true, fmt.Errorf("persist tenant worker override config: %w", err)
		}

		return true, nil
	}

	err := h.WorkerAdmin.SetTenantWorkerOverride(ctx, tenantID, workerID, disabled, "")
	if err != nil {
		return false, fmt.Errorf("persist tenant worker override: %w", err)
	}

	return false, nil
}

// workerStateResponse builds a single-worker view for the action response,
// scoped to the caller.
func (h *AdminHandlers) workerStateResponse(identity Identity, workerID string) workerAdminView {
	var views []WorkerView
	if identity.IsPlatform() {
		views = h.Workers.ListWorkersPlatform()
	} else {
		views = h.Workers.ListWorkersForTenant(identity.TenantID)
	}

	for _, v := range views {
		if v.WorkerID != workerID {
			continue
		}

		row := workerAdminView{
			WorkerID:         v.WorkerID,
			RuntimeState:     string(v.RuntimeState),
			GlobalAdminState: string(v.GlobalAdminState),
			GlobalDraining:   v.GlobalDraining,
			TenantAdminState: adminMapToStrings(v.TenantAdmin),
			TenantDraining:   v.TenantDrain,
			ExecutorType:     v.ExecutorType,
		}
		if identity.IsPlatform() {
			row.SessionID = v.SessionID
			row.AssignSubject = v.AssignSubject
		}

		return row
	}
	// Global admin state may exist for a worker with no active session; a
	// platform caller still gets a minimal row.
	return workerAdminView{WorkerID: workerID, RuntimeState: string(h.Workers.RuntimeState(workerID))}
}
