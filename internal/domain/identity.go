package domain

import (
	"context"
	"time"
)

const (
	// PermissionAll grants access to all resources.
	PermissionAll = "*"
	// PermissionManagementRead grants read access to management resources.
	PermissionManagementRead = "management:read"
	// PermissionUsersRead grants read access to users.
	PermissionUsersRead = "users:read"
	// PermissionUsersWrite grants write access to users.
	PermissionUsersWrite = "users:write"
	// PermissionAPIKeysRead grants read access to API keys.
	PermissionAPIKeysRead = "api_" + "keys:read"
	// PermissionAPIKeysWrite grants write access to API keys.
	PermissionAPIKeysWrite = "api_" + "keys:write"
	// PermissionAPIKeysRotate grants permission to rotate API keys.
	PermissionAPIKeysRotate = "api_keys:rotate"
	// PermissionAPIKeysRevoke grants permission to revoke API keys.
	PermissionAPIKeysRevoke = "api_keys:revoke"
	// PermissionRoutingRulesRead grants read access to routing rules.
	PermissionRoutingRulesRead = "routing_rules:read"
	// PermissionRoutingRulesWrite grants write access to routing rules.
	PermissionRoutingRulesWrite = "routing_rules:write"
	// PermissionEndpointsRead grants read access to endpoints.
	PermissionEndpointsRead = "endpoints:read"
	// PermissionEndpointsWrite grants write access to endpoints.
	PermissionEndpointsWrite = "endpoints:write"
	// PermissionEndpointsControl grants control access to endpoints.
	PermissionEndpointsControl = "endpoints:control"
	// PermissionEndpointsLogs grants read access to endpoint logs.
	PermissionEndpointsLogs = "endpoints:logs"
	// PermissionFingerprintsRead grants read access to fingerprints.
	PermissionFingerprintsRead = "fingerprints:read"
	// PermissionFingerprintsWrite grants write access to fingerprints.
	PermissionFingerprintsWrite = "fingerprints:write"
	// PermissionFingerprintsDelete grants delete access to fingerprints.
	PermissionFingerprintsDelete = "fingerprints:delete"
	// PermissionFingerprintsBroadcast grants broadcast access to fingerprints.
	PermissionFingerprintsBroadcast = "fingerprints:broadcast"
	// PermissionUsageRead grants read access to usage data.
	PermissionUsageRead = "usage:read"
	// PermissionBillingRead grants read access to billing data.
	PermissionBillingRead = "billing:read"
	// PermissionCostMultipliersRead grants read access to cost multipliers.
	PermissionCostMultipliersRead = "cost_multipliers:read"
	// PermissionCostMultipliersWrite grants write access to cost multipliers.
	PermissionCostMultipliersWrite = "cost_multipliers:write"
	// PermissionAuditRead grants read access to audit logs.
	PermissionAuditRead = "audit:read"
	// PermissionReportsRead grants read access to reports.
	PermissionReportsRead = "reports:read"
	// PermissionReportsWrite grants write access to reports.
	PermissionReportsWrite = "reports:write"
	// PermissionReportsRun grants permission to run reports.
	PermissionReportsRun = "reports:run"
	// PermissionAlertsRead grants read access to alerts.
	PermissionAlertsRead = "alerts:read"
	// PermissionAlertsWrite grants write access to alerts.
	PermissionAlertsWrite = "alerts:write"
	// PermissionNotificationsWrite grants write access to notifications.
	PermissionNotificationsWrite = "notifications:write"
	// PermissionCacheRead grants read access to cache.
	PermissionCacheRead = "cache:read"
	// PermissionCacheWrite grants write access to cache.
	PermissionCacheWrite = "cache:write"

	// RoleOwner is the owner role with full permissions.
	RoleOwner = "Owner"
	// RoleOperator is the operator role.
	RoleOperator = "Operator"
	// RoleSecurityAuditor is the security auditor role.
	RoleSecurityAuditor = "Security auditor"
	// RoleFinance is the finance role.
	RoleFinance = "Finance"
	// RoleReadOnly is the read-only role.
	RoleReadOnly = "Read only"
)

var allPermissions = []string{
	PermissionManagementRead,
	PermissionUsersRead,
	PermissionUsersWrite,
	PermissionAPIKeysRead,
	PermissionAPIKeysWrite,
	PermissionAPIKeysRotate,
	PermissionAPIKeysRevoke,
	PermissionRoutingRulesRead,
	PermissionRoutingRulesWrite,
	PermissionEndpointsRead,
	PermissionEndpointsWrite,
	PermissionEndpointsControl,
	PermissionEndpointsLogs,
	PermissionFingerprintsRead,
	PermissionFingerprintsWrite,
	PermissionFingerprintsDelete,
	PermissionFingerprintsBroadcast,
	PermissionUsageRead,
	PermissionBillingRead,
	PermissionCostMultipliersRead,
	PermissionCostMultipliersWrite,
	PermissionAuditRead,
	PermissionReportsRead,
	PermissionReportsWrite,
	PermissionReportsRun,
	PermissionAlertsRead,
	PermissionAlertsWrite,
	PermissionNotificationsWrite,
	PermissionCacheRead,
	PermissionCacheWrite,
}

// AllPermissions returns a copy of all defined permission strings.
func AllPermissions() []string {
	return append([]string(nil), allPermissions...)
}

// AdminUser represents a user with administrative access.
type AdminUser struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	IsActive     bool
	IsSuperAdmin bool
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AdminRole represents a role with a set of permissions.
type AdminRole struct {
	ID          string
	Name        string
	Description string
	IsBuiltin   bool
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AdminSession represents an authenticated admin session.
type AdminSession struct {
	ID               string
	UserID           string
	RefreshTokenHash string
	UserAgent        string
	IP               string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
	LastUsedAt       time.Time
}

// AdminIdentityProvider represents an external identity provider for admin authentication.
type AdminIdentityProvider struct {
	ID              string
	Name            string
	Type            string
	IssuerURL       string
	ClientID        string
	ClientSecretRef string
	JWKSURL         string
	Scopes          []string
	RoleClaim       string
	DefaultRoleID   string
	IsEnabled       bool
	Config          ConfigMap
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IdentityRepository provides persistence operations for admin users, roles, sessions, and identity providers.
type IdentityRepository interface {
	CreateUser(ctx context.Context, user *AdminUser) error
	UpdateUser(ctx context.Context, user *AdminUser) error
	GetUserByID(ctx context.Context, id string) (*AdminUser, error)
	GetUserByEmail(ctx context.Context, email string) (*AdminUser, error)
	ListUsers(ctx context.Context, limit, offset int) ([]AdminUser, int, error)
	SetUserRoles(ctx context.Context, userID string, roleIDs []string) error
	ListUserRoles(ctx context.Context, userID string) ([]AdminRole, error)
	EffectivePermissions(ctx context.Context, userID string) ([]string, error)
	ActiveOwnerExists(ctx context.Context) (bool, error)
	CountActiveOwners(ctx context.Context) (int, error)

	CreateRole(ctx context.Context, role *AdminRole) error
	UpdateRole(ctx context.Context, role *AdminRole) error
	DeleteRole(ctx context.Context, id string) error
	GetRoleByID(ctx context.Context, id string) (*AdminRole, error)
	GetRoleByName(ctx context.Context, name string) (*AdminRole, error)
	ListRoles(ctx context.Context) ([]AdminRole, error)

	CreateSession(ctx context.Context, session *AdminSession) error
	GetSessionByID(ctx context.Context, id string) (*AdminSession, error)
	GetSessionByRefreshTokenHash(ctx context.Context, hash string) (*AdminSession, error)
	UpdateSessionRefreshHash(ctx context.Context, id, hash string) error
	RevokeSession(ctx context.Context, id string) error
	RevokeUserSessions(ctx context.Context, userID string) error

	CreateIdentityProvider(ctx context.Context, provider *AdminIdentityProvider) error
	UpdateIdentityProvider(ctx context.Context, provider *AdminIdentityProvider) error
	GetIdentityProviderByID(ctx context.Context, id string) (*AdminIdentityProvider, error)
	GetIdentityProviderByName(ctx context.Context, name string) (*AdminIdentityProvider, error)
	ListIdentityProviders(ctx context.Context) ([]AdminIdentityProvider, error)
	DisableIdentityProvider(ctx context.Context, id string) error
}
