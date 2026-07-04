package control

import (
	"errors"
	"sync"
)

// ErrRequestNotFound is returned by InFlightRegistry.Cancel when a
// platform-scoped caller cancels a request_id with no in-flight entry. It
// never reaches a tenant-scoped caller: for tenant scope, unknown and
// foreign-tenant request_ids both collapse to ErrInsufficientPermissions so
// existence is never disclosed (docs/planning/26-config-management-api-surface.md).
var ErrRequestNotFound = errors.New("request not found")

type inflightEntry struct {
	tenantID string
	cancel   func()
}

// InFlightRegistry maps a running request_id to its owning tenant and the
// context cancel function that drives Control-initiated cancellation. It is
// in-process and single-Control for P0 (docs/tasks/p0/27, "Out of Scope"):
// registrations do not survive a Control restart and are not shared across
// Control instances.
type InFlightRegistry struct {
	mu      sync.Mutex
	entries map[string]inflightEntry
}

// NewInFlightRegistry builds an empty registry.
func NewInFlightRegistry() *InFlightRegistry {
	return &InFlightRegistry{entries: make(map[string]inflightEntry)}
}

// Register records a dispatched request's tenant and cancel function. Call
// Deregister (typically via defer) once the request completes.
func (r *InFlightRegistry) Register(requestID, tenantID string, cancel func()) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries[requestID] = inflightEntry{tenantID: tenantID, cancel: cancel}
}

// Deregister removes a completed request's entry.
func (r *InFlightRegistry) Deregister(requestID string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.entries, requestID)
}

// Cancel authorizes and triggers cancellation of an in-flight request
// (docs/planning/26 "Runtime Admin Endpoints"). A platform system_admin may
// cancel any request; a tenant-scoped caller may cancel only a request
// belonging to its own tenant. A tenant-scoped caller referencing a foreign
// or unknown request_id receives ErrInsufficientPermissions without
// disclosing whether the request exists. A platform caller referencing an
// unknown request_id receives ErrRequestNotFound.
func (r *InFlightRegistry) Cancel(identity Identity, requestID string) error {
	if r == nil {
		return ErrRequestNotFound
	}

	r.mu.Lock()
	entry, ok := r.entries[requestID]
	r.mu.Unlock()

	if !ok {
		if identity.IsPlatform() {
			return ErrRequestNotFound
		}

		return ErrInsufficientPermissions
	}

	err := AuthorizeAdminCancel(identity, entry.tenantID)
	if err != nil {
		return err
	}

	entry.cancel()

	return nil
}
