package control

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/beremaran/straw/v2/internal/config"
)

const deploymentIdentityID = "deployment"

// ScopeType distinguishes platform-scoped credentials from tenant-scoped
// credentials, per docs/planning/06-identity-roles-and-tenant-isolation.md.
type ScopeType string

const (
	// ScopePlatform marks platform-scoped credentials.
	ScopePlatform ScopeType = "platform"
	// ScopeTenant marks tenant-scoped credentials.
	ScopeTenant ScopeType = "tenant"
)

// Role is an API key role. Platform scope has exactly one role
// (system_admin); tenant scope has requester, viewer, operator, and
// tenant_admin.
type Role string

const (
	// RoleSystemAdmin is the platform administrator role.
	RoleSystemAdmin Role = "system_admin"
	// RoleRequester may execute data-plane requests.
	RoleRequester Role = "requester"
	// RoleViewer is read-only.
	RoleViewer Role = "viewer"
	// RoleOperator is reserved for tenant policy in later work.
	RoleOperator Role = "operator"
	// RoleTenantAdmin manages tenant-scoped resources.
	RoleTenantAdmin Role = "tenant_admin"
)

// ValidPlatformRole reports whether role is valid for scope_type = platform.
func ValidPlatformRole(role Role) bool {
	return role == RoleSystemAdmin
}

// ValidTenantRole reports whether role is valid for scope_type = tenant.
func ValidTenantRole(role Role) bool {
	switch role {
	case RoleSystemAdmin:
		return false
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
	store           APIKeyStore
	pepper          []byte
	tenants         TenantStore
	deploymentToken string
	deploymentMode  bool
}

// NewAuthenticator builds an Authenticator backed by store. pepper may be
// nil/empty for local development; production deployments should load it
// from a secret manager or environment variable.
func NewAuthenticator(store APIKeyStore, pepper []byte) *Authenticator {
	return &Authenticator{store: store, pepper: pepper}
}

// NewDeploymentAuthenticator authenticates the single deployment boundary.
// An empty token allows unauthenticated requests for local development. A
// non-empty token requires an exact Bearer token match.
func NewDeploymentAuthenticator(token string) *Authenticator {
	return &Authenticator{deploymentToken: token, deploymentMode: true}
}

// SetTenantStore wires tenant status enforcement into Authenticate: a
// tenant-scoped key whose tenant is not active fails with
// ErrTenantNotFound (docs/implementation-history.md#p0-29). A nil tenants store (the default)
// skips this check, matching pre-task-29 behavior for callers that only
// need API-key validity (e.g. platform-only auth in tests).
func (a *Authenticator) SetTenantStore(tenants TenantStore) *Authenticator {
	a.tenants = tenants

	return a
}

// Authenticate resolves the Authorization header value into an Identity.
// It never returns information distinguishing "no such prefix" from
// "wrong secret" from "revoked key" — all such cases collapse to
// ErrAuthFailure so callers cannot probe key validity.
func (a *Authenticator) Authenticate(ctx context.Context, authorizationHeader string) (Identity, error) {
	if a.deploymentMode {
		return a.authenticateDeployment(authorizationHeader)
	}

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

		tenantErr := a.checkTenantActive(ctx, candidate)
		if tenantErr != nil {
			return Identity{}, tenantErr
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

func (a *Authenticator) authenticateDeployment(authorizationHeader string) (Identity, error) {
	if a.deploymentToken != "" {
		token, err := BearerToken(authorizationHeader)
		if err != nil || len(token) != len(a.deploymentToken) || subtle.ConstantTimeCompare([]byte(token), []byte(a.deploymentToken)) != 1 {
			return Identity{}, ErrAuthFailure
		}
	}

	return Identity{
		APIKeyID:  deploymentIdentityID,
		ScopeType: ScopeTenant,
		TenantID:  config.DefaultDeploymentID,
		Role:      RoleRequester,
	}, nil
}

// checkTenantActive enforces tenant status for a tenant-scoped candidate
// key (docs/implementation-history.md#p0-29). A nil tenants store skips the check. Missing,
// suspended, and deleted tenants all collapse to ErrTenantNotFound so
// callers cannot probe tenant state.
func (a *Authenticator) checkTenantActive(ctx context.Context, candidate APIKeyRecord) error {
	if candidate.ScopeType != ScopeTenant || a.tenants == nil {
		return nil
	}

	tenant, err := a.tenants.Get(ctx, candidate.TenantID)
	if err != nil || tenant.Status != TenantStatusActive {
		return ErrTenantNotFound
	}

	return nil
}
