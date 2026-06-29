package domain

import (
	"context"
	"time"
)

const (
	PermissionAll                   = "*"
	PermissionManagementRead        = "management:read"
	PermissionUsersRead             = "users:read"
	PermissionUsersWrite            = "users:write"
	PermissionAPIKeysRead           = "api_keys:read"
	PermissionAPIKeysWrite          = "api_keys:write"
	PermissionAPIKeysRotate         = "api_keys:rotate"
	PermissionAPIKeysRevoke         = "api_keys:revoke"
	PermissionRoutingRulesRead      = "routing_rules:read"
	PermissionRoutingRulesWrite     = "routing_rules:write"
	PermissionEndpointsRead         = "endpoints:read"
	PermissionEndpointsWrite        = "endpoints:write"
	PermissionEndpointsControl      = "endpoints:control"
	PermissionEndpointsLogs         = "endpoints:logs"
	PermissionFingerprintsRead      = "fingerprints:read"
	PermissionFingerprintsWrite     = "fingerprints:write"
	PermissionFingerprintsDelete    = "fingerprints:delete"
	PermissionFingerprintsBroadcast = "fingerprints:broadcast"
	PermissionUsageRead             = "usage:read"
	PermissionBillingRead           = "billing:read"
	PermissionCostMultipliersRead   = "cost_multipliers:read"
	PermissionCostMultipliersWrite  = "cost_multipliers:write"
	PermissionAuditRead             = "audit:read"
	PermissionReportsRead           = "reports:read"
	PermissionReportsWrite          = "reports:write"
	PermissionReportsRun            = "reports:run"
	PermissionAlertsRead            = "alerts:read"
	PermissionAlertsWrite           = "alerts:write"
	PermissionNotificationsWrite    = "notifications:write"
	PermissionCacheRead             = "cache:read"
	PermissionCacheWrite            = "cache:write"

	RoleOwner           = "Owner"
	RoleOperator        = "Operator"
	RoleSecurityAuditor = "Security auditor"
	RoleFinance         = "Finance"
	RoleReadOnly        = "Read only"
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

func AllPermissions() []string {
	return append([]string(nil), allPermissions...)
}

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

type AdminRole struct {
	ID          string
	Name        string
	Description string
	IsBuiltin   bool
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

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

	CreateRole(ctx context.Context, role *AdminRole) error
	UpdateRole(ctx context.Context, role *AdminRole) error
	DeleteRole(ctx context.Context, id string) error
	GetRoleByID(ctx context.Context, id string) (*AdminRole, error)
	GetRoleByName(ctx context.Context, name string) (*AdminRole, error)
	ListRoles(ctx context.Context) ([]AdminRole, error)

	CreateSession(ctx context.Context, session *AdminSession) error
	GetSessionByRefreshTokenHash(ctx context.Context, hash string) (*AdminSession, error)
	UpdateSessionRefreshHash(ctx context.Context, id, hash string) error
	RevokeSession(ctx context.Context, id string) error
	RevokeUserSessions(ctx context.Context, userID string) error

	CreateIdentityProvider(ctx context.Context, provider *AdminIdentityProvider) error
	UpdateIdentityProvider(ctx context.Context, provider *AdminIdentityProvider) error
	GetIdentityProviderByName(ctx context.Context, name string) (*AdminIdentityProvider, error)
	ListIdentityProviders(ctx context.Context) ([]AdminIdentityProvider, error)
	DisableIdentityProvider(ctx context.Context, id string) error
}
