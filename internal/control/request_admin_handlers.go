package control

import (
	"errors"
	"net/http"
)

// cancelRequestResponse is the JSON body for a successful admin cancel.
type cancelRequestResponse struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

// CancelRequest handles POST /api/v1/admin/requests/{request_id}/cancel
// (docs/planning/26 "Runtime Admin Endpoints"). A platform system_admin may
// cancel any in-flight request; tenant-scoped tenant_admin and operator keys
// may cancel only a request belonging to their own tenant. A foreign-tenant
// or unknown request_id from a tenant-scoped key returns
// insufficient_permissions without disclosing whether the request exists.
func (h *AdminHandlers) CancelRequest(w http.ResponseWriter, r *http.Request) {
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

	if h.InFlight == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	requestID := r.PathValue("request_id")

	err = h.InFlight.Cancel(identity, requestID)
	switch {
	case err == nil:
		recordAudit(r.Context(), h.Audit, identity, "request", requestID, "cancel", 0, "", nil, nil, false)
		writeJSON(w, http.StatusOK, cancelRequestResponse{RequestID: requestID, Status: "cancelling"})
	case errors.Is(err, ErrInsufficientPermissions):
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, requestID, nil))
	case errors.Is(err, ErrRequestNotFound):
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, requestID, map[string]string{
			errorDetailReasonKey: "unknown request_id",
		}))
	default:
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, requestID, nil))
	}
}
