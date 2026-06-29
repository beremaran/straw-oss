package middleware

import "context"

const (
	ActorTypeLegacy = "legacy"
	ActorTypeUser   = "user"

	PermissionAll                   = "*"
	PermissionAPIKeysRead           = "api_keys:read"
	PermissionAPIKeysWrite          = "api_keys:write"
	PermissionAPIKeysRevoke         = "api_keys:revoke"
	PermissionRoutingRulesRead      = "routing_rules:read"
	PermissionRoutingRulesWrite     = "routing_rules:write"
	PermissionEndpointsRead         = "endpoints:read"
	PermissionEndpointsControl      = "endpoints:control"
	PermissionFingerprintsRead      = "fingerprints:read"
	PermissionFingerprintsWrite     = "fingerprints:write"
	PermissionFingerprintsBroadcast = "fingerprints:broadcast"
	PermissionUsageRead             = "usage:read"
	PermissionBillingRead           = "billing:read"
	PermissionCacheRead             = "cache:read"
	PermissionCacheWrite            = "cache:write"
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
