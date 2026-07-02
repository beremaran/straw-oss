package control

// RBAC enforcement helpers per the role matrix in
// docs/planning/06-identity-roles-and-tenant-isolation.md and the endpoint
// table in docs/planning/26-config-management-api-surface.md.
//
// P0 note on the `operator` role: the planning doc marks data-plane
// execution for `operator` as "Optional by tenant policy". P0 does not
// implement a per-tenant policy toggle for this (no such config resource
// exists in this task's scope), so P0 conservatively defaults `operator`
// to no data-plane execution. This is a deny-by-default deviation, not an
// permission expansion, and is documented in the task handoff.

import "slices"

// RequireRole returns ErrInsufficientPermissions unless identity.Role is
// one of allowed.
func RequireRole(identity Identity, allowed ...Role) error {
	if slices.Contains(allowed, identity.Role) {
		return nil
	}

	return ErrInsufficientPermissions
}

// RequirePlatformScope returns ErrInsufficientPermissions unless identity is
// platform-scoped.
func RequirePlatformScope(identity Identity) error {
	if !identity.IsPlatform() {
		return ErrInsufficientPermissions
	}

	return nil
}

// RequireTenantScope returns ErrInsufficientPermissions unless identity is
// tenant-scoped.
func RequireTenantScope(identity Identity) error {
	if identity.IsPlatform() {
		return ErrInsufficientPermissions
	}

	return nil
}

// RequireOwnTenant returns ErrInsufficientPermissions unless identity is
// tenant-scoped and belongs to tenantID. Used to enforce tenant isolation
// on tenant-scoped resources (API keys, worker credentials, quotas, ...).
func RequireOwnTenant(identity Identity, tenantID string) error {
	err := RequireTenantScope(identity)
	if err != nil {
		return err
	}

	if identity.TenantID != tenantID {
		return ErrInsufficientPermissions
	}

	return nil
}

// CanExecuteDataPlane reports whether identity may execute
// POST /api/v1/requests. Platform-scoped keys can never execute data-plane
// requests (docs/planning/06, docs/planning/07).
func CanExecuteDataPlane(identity Identity) bool {
	if identity.IsPlatform() {
		return false
	}

	switch identity.Role {
	case RoleRequester, RoleTenantAdmin:
		return true
	default:
		// viewer: never. operator: P0 defaults to no execution (see note
		// above).
		return false
	}
}
