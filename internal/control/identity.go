package control

import (
	"context"
	"errors"
)

// ScopeType distinguishes platform-scoped credentials from tenant-scoped
// credentials, per docs/planning/06-identity-roles-and-tenant-isolation.md.
type ScopeType string

const (
	ScopePlatform ScopeType = "platform"
	ScopeTenant   ScopeType = "tenant"
)

// Role is an API key role. Platform scope has exactly one role
// (system_admin); tenant scope has requester, viewer, operator, and
// tenant_admin.
type Role string

const (
	RoleSystemAdmin Role = "system_admin"
	RoleRequester   Role = "requester"
	RoleViewer      Role = "viewer"
	RoleOperator    Role = "operator"
	RoleTenantAdmin Role = "tenant_admin"
)

// ValidPlatformRole reports whether role is valid for scope_type = platform.
func ValidPlatformRole(role Role) bool {
	return role == RoleSystemAdmin
}

// ValidTenantRole reports whether role is valid for scope_type = tenant.
func ValidTenantRole(role Role) bool {
	switch role {
	case RoleRequester, RoleViewer, RoleOperator, RoleTenantAdmin:
		return true
	default:
		return false
	}
}

// Identity is the resolved caller identity derived exclusively from a
// validated API key. Clients and workers cannot supply or override tenant
// identity through any other channel (see planning doc 06).
type Identity struct {
	APIKeyID  string
	ScopeType ScopeType
	TenantID  string // empty for platform-scoped identities
	Role      Role
}

// IsPlatform reports whether this identity is platform-scoped.
func (id Identity) IsPlatform() bool {
	return id.ScopeType == ScopePlatform
}

// Sentinel authentication errors. Handlers map these to the canonical
// auth_failure / insufficient_permissions error codes without leaking
// details about which check failed, per the redaction rules in
// docs/planning/27-security-controls.md.
var (
	ErrAuthFailure             = errors.New("auth failure")
	ErrInsufficientPermissions = errors.New("insufficient permissions")
)

// Authenticator resolves a bearer token into an Identity by prefix lookup
// followed by constant-time secret comparison.
type Authenticator struct {
	store  APIKeyStore
	pepper []byte
}

// NewAuthenticator builds an Authenticator backed by store. pepper may be
// nil/empty for local development; production deployments should load it
// from a secret manager or environment variable.
func NewAuthenticator(store APIKeyStore, pepper []byte) *Authenticator {
	return &Authenticator{store: store, pepper: pepper}
}

// Authenticate resolves the Authorization header value into an Identity.
// It never returns information distinguishing "no such prefix" from
// "wrong secret" from "revoked key" — all such cases collapse to
// ErrAuthFailure so callers cannot probe key validity.
func (a *Authenticator) Authenticate(ctx context.Context, authorizationHeader string) (Identity, error) {
	token, err := BearerToken(authorizationHeader)
	if err != nil {
		return Identity{}, ErrAuthFailure
	}

	prefix, err := ExtractKeyPrefix(token)
	if err != nil {
		return Identity{}, ErrAuthFailure
	}

	candidates, err := a.store.FindByPrefix(ctx, prefix)
	if err != nil {
		return Identity{}, ErrAuthFailure
	}

	for _, candidate := range candidates {
		if candidate.Status != APIKeyStatusActive {
			continue
		}

		if !VerifyAPIKeySecret(token, candidate.SecretHash, a.pepper) {
			continue
		}

		return Identity{
			APIKeyID:  candidate.ID,
			ScopeType: candidate.ScopeType,
			TenantID:  candidate.TenantID,
			Role:      candidate.Role,
		}, nil
	}

	return Identity{}, ErrAuthFailure
}
