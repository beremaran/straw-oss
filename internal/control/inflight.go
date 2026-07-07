package control

import (
	"context"
	"errors"
	"fmt"
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

// InFlightCrossInstance resolves a cancel for a request_id whose in-flight
// entry lives in a sibling Control process. It is backed by the Redis
// runtime-state tier (docs/planning/21: "short-lived in-flight request state"
// with a TTL, plus an ephemeral cancel pub/sub channel). When an
// InFlightRegistry has no cross-instance collaborator (the default), it stays
// pure in-process and single-Control exactly as P0 task 27 left it
// (docs/tasks/p1/23).
type InFlightCrossInstance interface {
	// Record advertises that this instance owns requestID for tenantID so a
	// sibling instance can authorize and route a cancel to it. Best-effort:
	// a backend failure only disables cross-instance cancel for that request.
	Record(ctx context.Context, requestID, tenantID string)
	// Clear removes the ownership record when the request completes.
	Clear(ctx context.Context, requestID string)
	// Lookup returns the owning tenant_id for a request_id not owned locally;
	// ok is false when no instance currently advertises ownership.
	Lookup(ctx context.Context, requestID string) (tenantID string, ok bool)
	// SignalCancel asks the owning instance to cancel requestID.
	SignalCancel(ctx context.Context, requestID string) error
}

// InFlightRegistry maps a running request_id to its owning tenant and the
// context cancel function that drives Control-initiated cancellation. It is
// in-process by default (docs/tasks/p0/27): registrations do not survive a
// Control restart. When a cross-instance collaborator is attached
// (docs/tasks/p1/23), a cancel for a request_id not owned locally is routed
// to the sibling Control instance that owns it; the local in-process path is
// unchanged and never touches the shared backend.
type InFlightRegistry struct {
	mu      sync.Mutex
	entries map[string]inflightEntry
	cross   InFlightCrossInstance
}

// NewInFlightRegistry builds an empty registry.
func NewInFlightRegistry() *InFlightRegistry {
	return &InFlightRegistry{entries: make(map[string]inflightEntry)}
}

// SetCrossInstance attaches the cross-Control-instance resolver
// (docs/tasks/p1/23). With it set, Register/Deregister advertise and clear
// ownership in the shared backend and Cancel falls back to it for request_ids
// not owned locally. Nil keeps the registry pure in-process/single-Control.
func (r *InFlightRegistry) SetCrossInstance(cross InFlightCrossInstance) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cross = cross
}

// Register records a dispatched request's tenant and cancel function. Call
// Deregister (typically via defer) once the request completes. ctx scopes the
// best-effort cross-instance ownership advertisement; the local registration
// always succeeds.
func (r *InFlightRegistry) Register(ctx context.Context, requestID, tenantID string, cancel func()) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.entries[requestID] = inflightEntry{tenantID: tenantID, cancel: cancel}
	cross := r.cross
	r.mu.Unlock()

	if cross != nil {
		cross.Record(ctx, requestID, tenantID)
	}
}

// Deregister removes a completed request's entry. ctx scopes the best-effort
// cross-instance ownership clear.
func (r *InFlightRegistry) Deregister(ctx context.Context, requestID string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	delete(r.entries, requestID)
	cross := r.cross
	r.mu.Unlock()

	if cross != nil {
		cross.Clear(ctx, requestID)
	}
}

// Cancel authorizes and triggers cancellation of an in-flight request
// (docs/planning/26 "Runtime Admin Endpoints"). A platform system_admin may
// cancel any request; a tenant-scoped caller may cancel only a request
// belonging to its own tenant. A tenant-scoped caller referencing a foreign
// or unknown request_id receives ErrInsufficientPermissions without
// disclosing whether the request exists. A platform caller referencing an
// unknown request_id receives ErrRequestNotFound.
//
// When the request is not owned locally and a cross-instance collaborator is
// attached (docs/tasks/p1/23), Cancel resolves the owning tenant from the
// shared backend, applies the same authorization, and signals the owning
// Control instance to tear the request down. The local path is the fast path
// and never touches the shared backend.
func (r *InFlightRegistry) Cancel(ctx context.Context, identity Identity, requestID string) error {
	if r == nil {
		return ErrRequestNotFound
	}

	r.mu.Lock()
	entry, ok := r.entries[requestID]
	cross := r.cross
	r.mu.Unlock()

	if ok {
		err := AuthorizeAdminCancel(identity, entry.tenantID)
		if err != nil {
			return err
		}

		entry.cancel()

		return nil
	}

	if cross != nil {
		if tenantID, found := cross.Lookup(ctx, requestID); found {
			return cancelViaCrossInstance(ctx, cross, identity, tenantID, requestID)
		}
	}

	if identity.IsPlatform() {
		return ErrRequestNotFound
	}

	return ErrInsufficientPermissions
}

// cancelViaCrossInstance authorizes and signals a cancel for a request owned by
// a sibling Control instance (docs/tasks/p1/23). Authorization uses the owning
// tenant resolved from the shared backend and is unchanged from the local path.
func cancelViaCrossInstance(ctx context.Context, cross InFlightCrossInstance, identity Identity, tenantID, requestID string) error {
	err := AuthorizeAdminCancel(identity, tenantID)
	if err != nil {
		return err
	}

	err = cross.SignalCancel(ctx, requestID)
	if err != nil {
		return fmt.Errorf("signal cross-instance cancel: %w", err)
	}

	return nil
}

// cancelLocal cancels a locally-owned request without re-authorizing. It is
// applied by the cross-instance cancel subscriber to a cancel a sibling
// instance already authorized (docs/tasks/p1/23); it is a no-op when this
// instance does not own the request. Returns true if this instance owned it.
func (r *InFlightRegistry) cancelLocal(requestID string) bool {
	if r == nil {
		return false
	}

	r.mu.Lock()
	entry, ok := r.entries[requestID]
	r.mu.Unlock()

	if !ok {
		return false
	}

	entry.cancel()

	return true
}
