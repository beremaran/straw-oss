// Package middleware provides HTTP middleware handlers for the admin server.
package middleware

import (
	"context"

	"github.com/beremaran/straw/internal/domain"
)

const (
	// ActorTypeLegacy identifies the legacy admin actor.
	ActorTypeLegacy = "legacy"

	// ActorTypeUser identifies a user actor.
	ActorTypeUser = "user"

	// PermissionAll grants all permissions.
	PermissionAll = domain.PermissionAll

	// PermissionManagementRead grants read access to management settings.
	PermissionManagementRead = domain.PermissionManagementRead

	// PermissionUsersRead grants read access to users.
	PermissionUsersRead = domain.PermissionUsersRead

	// PermissionUsersWrite grants write access to users.
	PermissionUsersWrite = domain.PermissionUsersWrite

	// PermissionAPIKeysRead grants read access to API keys.
	PermissionAPIKeysRead = domain.PermissionAPIKeysRead

	// PermissionAPIKeysWrite grants write access to API keys.
	PermissionAPIKeysWrite = domain.PermissionAPIKeysWrite

	// PermissionAPIKeysRotate grants rotate access to API keys.
	PermissionAPIKeysRotate = domain.PermissionAPIKeysRotate

	// PermissionAPIKeysRevoke grants revoke access to API keys.
	PermissionAPIKeysRevoke = domain.PermissionAPIKeysRevoke

	// PermissionRoutingRulesRead grants read access to routing rules.
	PermissionRoutingRulesRead = domain.PermissionRoutingRulesRead

	// PermissionRoutingRulesWrite grants write access to routing rules.
	PermissionRoutingRulesWrite = domain.PermissionRoutingRulesWrite

	// PermissionEndpointsRead grants read access to endpoints.
	PermissionEndpointsRead = domain.PermissionEndpointsRead

	// PermissionEndpointsWrite grants write access to endpoints.
	PermissionEndpointsWrite = domain.PermissionEndpointsWrite

	// PermissionEndpointsControl grants control access to endpoints.
	PermissionEndpointsControl = domain.PermissionEndpointsControl

	// PermissionEndpointsLogs grants read access to endpoint logs.
	PermissionEndpointsLogs = domain.PermissionEndpointsLogs

	// PermissionFingerprintsRead grants read access to fingerprints.
	PermissionFingerprintsRead = domain.PermissionFingerprintsRead

	// PermissionFingerprintsWrite grants write access to fingerprints.
	PermissionFingerprintsWrite = domain.PermissionFingerprintsWrite

	// PermissionFingerprintsDelete grants delete access to fingerprints.
	PermissionFingerprintsDelete = domain.PermissionFingerprintsDelete

	// PermissionFingerprintsBroadcast grants broadcast access to fingerprints.
	PermissionFingerprintsBroadcast = domain.PermissionFingerprintsBroadcast

	// PermissionUsageRead grants read access to usage data.
	PermissionUsageRead = domain.PermissionUsageRead

	// PermissionBillingRead grants read access to billing.
	PermissionBillingRead = domain.PermissionBillingRead

	// PermissionCostMultipliersRead grants read access to cost multipliers.
	PermissionCostMultipliersRead = domain.PermissionCostMultipliersRead

	// PermissionCostMultipliersWrite grants write access to cost multipliers.
	PermissionCostMultipliersWrite = domain.PermissionCostMultipliersWrite

	// PermissionAuditRead grants read access to audit logs.
	PermissionAuditRead = domain.PermissionAuditRead

	// PermissionReportsRead grants read access to reports.
	PermissionReportsRead = domain.PermissionReportsRead

	// PermissionReportsWrite grants write access to reports.
	PermissionReportsWrite = domain.PermissionReportsWrite

	// PermissionReportsRun grants execution access to reports.
	PermissionReportsRun = domain.PermissionReportsRun

	// PermissionAlertsRead grants read access to alerts.
	PermissionAlertsRead = domain.PermissionAlertsRead

	// PermissionAlertsWrite grants write access to alerts.
	PermissionAlertsWrite = domain.PermissionAlertsWrite

	// PermissionNotificationsWrite grants write access to notifications.
	PermissionNotificationsWrite = domain.PermissionNotificationsWrite

	// PermissionCacheRead grants read access to cache.
	PermissionCacheRead = domain.PermissionCacheRead

	// PermissionCacheWrite grants write access to cache.
	PermissionCacheWrite = domain.PermissionCacheWrite
)

type actorContextKey struct{}

// Actor represents an authenticated actor making requests through the admin server.
type Actor struct {
	Type        string
	ID          string
	Email       string
	DisplayName string
	SessionID   string
	Permissions []string
}

// LegacyAdminActor returns an actor with all permissions for legacy management token auth.
func LegacyAdminActor() Actor {
	return Actor{
		Type:        ActorTypeLegacy,
		ID:          "system:legacy-admin",
		DisplayName: "Legacy admin",
		Permissions: []string{PermissionAll},
	}
}

// ContextWithActor stores an Actor in the context.
func ContextWithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// ActorFromContext retrieves an Actor from the context.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(Actor)

	return actor, ok
}

// HasPermission reports whether the actor has the given permission, or has all permissions.
func (a Actor) HasPermission(permission string) bool {
	for _, p := range a.Permissions {
		if p == PermissionAll || p == permission {
			return true
		}
	}

	return false
}
