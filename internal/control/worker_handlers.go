package control

import (
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

	if err := RequireRole(identity, RoleSystemAdmin, RoleTenantAdmin, RoleOperator); err != nil {
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

	if err := RequirePlatformScope(identity); err != nil {
		return Identity{}, err
	}

	if err := RequireRole(identity, RoleSystemAdmin); err != nil {
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

	if err := RequireTenantScope(identity); err != nil {
		return Identity{}, err
	}

	if err := RequireRole(identity, allowed...); err != nil {
		return Identity{}, err
	}

	return identity, nil
}

// globalWorkerAction is the shared body for the four global admin endpoints.
func (h *AdminHandlers) globalWorkerAction(w http.ResponseWriter, r *http.Request, action string, apply func(workerID string)) {
	identity, err := h.requirePlatformSystemAdmin(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	workerID := r.PathValue("worker_id")
	apply(workerID)
	recordAudit(r.Context(), h.Audit, identity, "worker", workerID, action)
	writeJSON(w, http.StatusOK, h.workerStateResponse(identity, workerID))
}

// DisableWorker handles POST /workers/{worker_id}/disable.
func (h *AdminHandlers) DisableWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "disable", func(id string) { h.Workers.SetGlobalAdmin(id, AdminDisabled) })
}

// EnableWorker handles POST /workers/{worker_id}/enable.
func (h *AdminHandlers) EnableWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "enable", func(id string) { h.Workers.SetGlobalAdmin(id, AdminEnabled) })
}

// DrainWorker handles POST /workers/{worker_id}/drain.
func (h *AdminHandlers) DrainWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "drain", func(id string) { h.Workers.SetGlobalDrain(id, true) })
}

// UndrainWorker handles POST /workers/{worker_id}/undrain.
func (h *AdminHandlers) UndrainWorker(w http.ResponseWriter, r *http.Request) {
	h.globalWorkerAction(w, r, "undrain", func(id string) { h.Workers.SetGlobalDrain(id, false) })
}

// tenantWorkerAction is the shared body for the four tenant admin endpoints.
func (h *AdminHandlers) tenantWorkerAction(w http.ResponseWriter, r *http.Request, action string, allowed []Role, apply func(workerID, tenantID string)) {
	identity, err := h.requireTenantRole(r, allowed...)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	workerID := r.PathValue("worker_id")
	apply(workerID, identity.TenantID)
	recordAudit(r.Context(), h.Audit, identity, "worker", workerID, action)
	writeJSON(w, http.StatusOK, h.workerStateResponse(identity, workerID))
}

// TenantDisableWorker handles POST /workers/{worker_id}/tenant-disable.
func (h *AdminHandlers) TenantDisableWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_disable", []Role{RoleTenantAdmin}, func(id, tid string) {
		h.Workers.SetTenantAdmin(id, tid, AdminDisabled)
	})
}

// TenantEnableWorker handles POST /workers/{worker_id}/tenant-enable.
func (h *AdminHandlers) TenantEnableWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_enable", []Role{RoleTenantAdmin}, func(id, tid string) {
		h.Workers.SetTenantAdmin(id, tid, AdminEnabled)
	})
}

// TenantDrainWorker handles POST /workers/{worker_id}/tenant-drain.
func (h *AdminHandlers) TenantDrainWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_drain", []Role{RoleTenantAdmin, RoleOperator}, func(id, tid string) {
		h.Workers.SetTenantDrain(id, tid, true)
	})
}

// TenantUndrainWorker handles POST /workers/{worker_id}/tenant-undrain.
func (h *AdminHandlers) TenantUndrainWorker(w http.ResponseWriter, r *http.Request) {
	h.tenantWorkerAction(w, r, "tenant_undrain", []Role{RoleTenantAdmin, RoleOperator}, func(id, tid string) {
		h.Workers.SetTenantDrain(id, tid, false)
	})
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
