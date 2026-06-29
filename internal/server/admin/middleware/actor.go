package middleware

import (
	"context"

	"github.com/beremaran/straw/internal/domain"
)

const (
	ActorTypeLegacy = "legacy"
	ActorTypeUser   = "user"

	PermissionAll                   = domain.PermissionAll
	PermissionManagementRead        = domain.PermissionManagementRead
	PermissionUsersRead             = domain.PermissionUsersRead
	PermissionUsersWrite            = domain.PermissionUsersWrite
	PermissionAPIKeysRead           = domain.PermissionAPIKeysRead
	PermissionAPIKeysWrite          = domain.PermissionAPIKeysWrite
	PermissionAPIKeysRotate         = domain.PermissionAPIKeysRotate
	PermissionAPIKeysRevoke         = domain.PermissionAPIKeysRevoke
	PermissionRoutingRulesRead      = domain.PermissionRoutingRulesRead
	PermissionRoutingRulesWrite     = domain.PermissionRoutingRulesWrite
	PermissionEndpointsRead         = domain.PermissionEndpointsRead
	PermissionEndpointsWrite        = domain.PermissionEndpointsWrite
	PermissionEndpointsControl      = domain.PermissionEndpointsControl
	PermissionEndpointsLogs         = domain.PermissionEndpointsLogs
	PermissionFingerprintsRead      = domain.PermissionFingerprintsRead
	PermissionFingerprintsWrite     = domain.PermissionFingerprintsWrite
	PermissionFingerprintsDelete    = domain.PermissionFingerprintsDelete
	PermissionFingerprintsBroadcast = domain.PermissionFingerprintsBroadcast
	PermissionUsageRead             = domain.PermissionUsageRead
	PermissionBillingRead           = domain.PermissionBillingRead
	PermissionCostMultipliersRead   = domain.PermissionCostMultipliersRead
	PermissionCostMultipliersWrite  = domain.PermissionCostMultipliersWrite
	PermissionAuditRead             = domain.PermissionAuditRead
	PermissionReportsRead           = domain.PermissionReportsRead
	PermissionReportsWrite          = domain.PermissionReportsWrite
	PermissionReportsRun            = domain.PermissionReportsRun
	PermissionAlertsRead            = domain.PermissionAlertsRead
	PermissionAlertsWrite           = domain.PermissionAlertsWrite
	PermissionNotificationsWrite    = domain.PermissionNotificationsWrite
	PermissionCacheRead             = domain.PermissionCacheRead
	PermissionCacheWrite            = domain.PermissionCacheWrite
)

type actorContextKey struct{}

type Actor struct {
	Type        string
	ID          string
	DisplayName string
	SessionID   string
	Permissions []string
}

func LegacyAdminActor() Actor {
	return Actor{
		Type:        ActorTypeLegacy,
		ID:          "system:legacy-admin",
		DisplayName: "Legacy admin",
		Permissions: []string{PermissionAll},
	}
}

func ContextWithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

func ActorFromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(Actor)

	return actor, ok
}

func (a Actor) HasPermission(permission string) bool {
	for _, p := range a.Permissions {
		if p == PermissionAll || p == permission {
			return true
		}
	}

	return false
}
