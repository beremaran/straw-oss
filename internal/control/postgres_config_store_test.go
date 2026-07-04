package control

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/beremaran/straw/v2/internal/config"
)

const (
	pgConfigBlockedHost  = "blocked.example"
	pgConfigPoolA        = "pool_a"
	pgConfigEncodedValue = "c2VjcmV0"
	pgConfigActorID      = "key_config_admin"
)

func TestPostgresConfigStoreSnapshotAssembly(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	ctx := context.Background()
	store := NewPostgresConfigStore(pool)
	actor := ConfigActor{ActorType: configActorTypeAPIKey, ActorID: pgConfigActorID, RequestID: "req_config"}

	_, _, err := store.UpsertExecutorPool(ctx, pgTestTenantA, config.ExecutorPool{
		ID:                   pgConfigPoolA,
		ExecutorType:         pgTestExecutorType,
		Tags:                 []string{"fast", "au"},
		Enabled:              true,
		AllowDegradedWorkers: true,
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertExecutorPool() error = %v", err)
	}

	_, _, err = store.UpsertRoutingRule(ctx, pgTestTenantA, config.RoutingRule{
		ID:           "route_keep",
		Priority:     20,
		Enabled:      true,
		Match:        config.MatchConditions{TargetHost: "*.example.com", Tags: []string{"fast"}},
		TargetPoolID: pgConfigPoolA,
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertRoutingRule(keep) error = %v", err)
	}

	_, _, err = store.UpsertRoutingRule(ctx, pgTestTenantA, config.RoutingRule{
		ID:           "route_delete",
		Priority:     10,
		Enabled:      true,
		TargetPoolID: pgConfigPoolA,
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertRoutingRule(delete) error = %v", err)
	}

	_, err = store.DeleteRoutingRule(ctx, pgTestTenantA, "route_delete", actor)
	if err != nil {
		t.Fatalf("DeleteRoutingRule() error = %v", err)
	}

	_, _, err = store.UpsertDenyRule(ctx, pgTestTenantA, config.DenyRule{
		ID:             "deny_host",
		RuleType:       denyRuleTypeHost,
		Action:         denyRuleActionDeny,
		Enabled:        true,
		RawPattern:     pgConfigBlockedHost,
		NormalizedHost: pgConfigBlockedHost,
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertDenyRule() error = %v", err)
	}

	_, _, err = store.UpsertInjectionPolicy(ctx, pgTestTenantA, config.InjectionPolicy{
		ID:      "inject_auth",
		Enabled: true,
		Operations: []config.InjectionOperation{{
			Op:          injectionOpSet,
			HeaderName:  "X-Test-Secret",
			ValueBase64: pgConfigEncodedValue,
		}},
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertInjectionPolicy() error = %v", err)
	}

	_, err = store.PutQuotaConfig(ctx, QuotaConfig{
		TenantID:           pgTestTenantA,
		Period:             quotaPeriodMonthly,
		MaxRequests:        123,
		MaxBandwidthBytes:  456,
		RequestCountPolicy: "count_on_admission",
		RedisFailPolicy:    postgresFailClosed,
	}, 0, actor)
	if err != nil {
		t.Fatalf("PutQuotaConfig() error = %v", err)
	}

	_, err = store.PutRateLimitConfig(ctx, RateLimitConfig{
		TenantID: pgTestTenantA,
		Limits: []RateLimitRule{{
			Dimension:     RateLimitDimTenant,
			Key:           "*",
			WindowSeconds: 60,
			MaxRequests:   50,
			FailPolicy:    RateLimitFailOpen,
		}},
	}, 0, &RateLimitCeiling{WindowSeconds: 60, MaxRequests: 100}, actor)
	if err != nil {
		t.Fatalf("PutRateLimitConfig() error = %v", err)
	}

	err = store.SetGlobalWorkerAdminConfig(ctx, "worker_global", true, "maintenance", actor)
	if err != nil {
		t.Fatalf("SetGlobalWorkerAdminConfig() error = %v", err)
	}

	err = store.SetTenantWorkerOverrideConfig(ctx, pgTestTenantA, "worker_tenant", true, "tenant block", actor)
	if err != nil {
		t.Fatalf("SetTenantWorkerOverrideConfig() error = %v", err)
	}

	version, err := store.CurrentTenantConfigVersion(ctx, pgTestTenantA)
	if err != nil {
		t.Fatalf("CurrentTenantConfigVersion() error = %v", err)
	}

	snap, err := store.LoadTenantSnapshot(ctx, pgTestTenantA, version)
	if err != nil {
		t.Fatalf("LoadTenantSnapshot() error = %v", err)
	}

	if snap.ConfigVersion != version {
		t.Fatalf("snapshot version = %d, want %d", snap.ConfigVersion, version)
	}
	if len(snap.RoutingRules) != 1 || snap.RoutingRules[0].ID != "route_keep" {
		t.Fatalf("routing rules = %+v, want only live route_keep", snap.RoutingRules)
	}
	if len(snap.ExecutorPools) != 1 || snap.ExecutorPools[0].ID != pgConfigPoolA || !snap.ExecutorPools[0].AllowDegradedWorkers {
		t.Fatalf("executor pools = %+v, want %s with allow_degraded_workers=true", snap.ExecutorPools, pgConfigPoolA)
	}
	if len(snap.DenyRules) != 1 || snap.DenyRules[0].NormalizedHost != pgConfigBlockedHost {
		t.Fatalf("deny rules = %+v, want %s", snap.DenyRules, pgConfigBlockedHost)
	}
	if len(snap.InjectionPolicies) != 1 || snap.InjectionPolicies[0].Operations[0].ValueBase64 != pgConfigEncodedValue {
		t.Fatalf("injection policies = %+v, want stored secret value in runtime snapshot", snap.InjectionPolicies)
	}
	if snap.Quota.RequestCountLimit != 123 || snap.Quota.BandwidthBytesLimit != 456 {
		t.Fatalf("quota = %+v, want request=123 bandwidth=456", snap.Quota)
	}
	if len(snap.RateLimits) != 1 || snap.RateLimits[0].MaxRequests != 50 {
		t.Fatalf("rate limits = %+v, want one max_requests=50 rule", snap.RateLimits)
	}
	if len(snap.WorkerAdminStates) != 1 || snap.WorkerAdminStates[0].WorkerID != "worker_global" {
		t.Fatalf("worker admin states = %+v, want worker_global", snap.WorkerAdminStates)
	}
	if len(snap.TenantWorkerOverrides) != 1 || snap.TenantWorkerOverrides[0].WorkerID != "worker_tenant" {
		t.Fatalf("tenant worker overrides = %+v, want worker_tenant", snap.TenantWorkerOverrides)
	}
	if !hasFingerprintProfile(snap.FingerprintProfiles, "default") {
		t.Fatalf("fingerprint profiles = %+v, want seeded default profile", snap.FingerprintProfiles)
	}

	_, err = store.LoadTenantSnapshot(ctx, pgTestTenantA, version-1)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale LoadTenantSnapshot() error = %v, want ErrVersionConflict", err)
	}
}

func TestPostgresConfigStoreRedactsInjectionPolicyAudit(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	ctx := context.Background()
	store := NewPostgresConfigStore(pool)
	actor := ConfigActor{ActorType: configActorTypeAPIKey, ActorID: pgConfigActorID}

	_, _, err := store.UpsertInjectionPolicy(ctx, pgTestTenantA, config.InjectionPolicy{
		ID:      "inject_redact",
		Enabled: true,
		Operations: []config.InjectionOperation{{
			Op:          injectionOpSet,
			HeaderName:  testAuthorizationHeader,
			ValueBase64: pgConfigEncodedValue,
		}},
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertInjectionPolicy() error = %v", err)
	}

	var auditJSON string
	err = pool.QueryRow(
		ctx,
		`SELECT new_value_json::text
		 FROM config_audit_source
		 WHERE tenant_id = $1 AND resource_type = 'injection_policy' AND resource_id = 'inject_redact'`,
		pgTestTenantA,
	).Scan(&auditJSON)
	if err != nil {
		t.Fatalf("read audit JSON: %v", err)
	}

	if !strings.Contains(auditJSON, requestMetadataRedacted) || strings.Contains(auditJSON, pgConfigEncodedValue) {
		t.Fatalf("audit JSON = %s, want redacted value without secret", auditJSON)
	}
}

// TestPostgresConfigStoreWriteIsAtomic proves the resource write, version
// increment, and audit row share one transaction: a write that fails inside the
// transaction must leave the tenant config version unchanged.
func TestPostgresConfigStoreWriteIsAtomic(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	ctx := context.Background()
	store := NewPostgresConfigStore(pool)
	actor := ConfigActor{ActorType: configActorTypeAPIKey, ActorID: pgConfigActorID}

	_, v1, err := store.UpsertRoutingRule(ctx, pgTestTenantA, config.RoutingRule{
		ID: "route_a", Priority: 10, Enabled: true, TargetPoolID: pgConfigPoolA,
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertRoutingRule() error = %v", err)
	}

	// An invalid CIDR fails the ::cidr cast inside the write transaction.
	_, _, err = store.UpsertDenyRule(ctx, pgTestTenantA, config.DenyRule{
		ID: "deny_bad", RuleType: denyRuleTypeCIDR, Action: denyRuleActionDeny, Enabled: true,
		RawPattern: "not-a-cidr", NormalizedCIDR: "not-a-cidr",
	}, 0, actor)
	if err == nil {
		t.Fatal("UpsertDenyRule() with invalid CIDR: expected error, got nil")
	}

	after, err := store.CurrentTenantConfigVersion(ctx, pgTestTenantA)
	if err != nil {
		t.Fatalf("CurrentTenantConfigVersion() error = %v", err)
	}

	if after != v1 {
		t.Fatalf("config version = %d after failed write, want %d (write must roll back the version)", after, v1)
	}

	// No audit row for the rolled-back deny rule either.
	var auditCount int

	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM config_audit_source WHERE resource_type = 'deny_rule' AND resource_id = 'deny_bad'`).Scan(&auditCount)
	if err != nil {
		t.Fatalf("count audit rows: %v", err)
	}

	if auditCount != 0 {
		t.Fatalf("audit rows for rolled-back write = %d, want 0", auditCount)
	}
}

// TestPostgresSaveTenantSnapshotOptimisticVersioning proves the SaveTenantSnapshot
// path (used by API-key/worker-credential revocation to force cache
// invalidation) bumps the tenant version under optimistic concurrency.
func TestPostgresSaveTenantSnapshotOptimisticVersioning(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	ctx := context.Background()
	store := NewPostgresConfigStore(pool)
	snap := config.TenantSnapshot{TenantID: pgTestTenantA}

	saved, err := store.SaveTenantSnapshot(ctx, snap, 0)
	if err != nil {
		t.Fatalf("SaveTenantSnapshot(expected 0) error = %v", err)
	}

	if saved.ConfigVersion != 1 {
		t.Fatalf("first save version = %d, want 1", saved.ConfigVersion)
	}

	_, err = store.SaveTenantSnapshot(ctx, snap, 0)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale SaveTenantSnapshot(expected 0) error = %v, want ErrVersionConflict", err)
	}

	saved, err = store.SaveTenantSnapshot(ctx, snap, 1)
	if err != nil {
		t.Fatalf("SaveTenantSnapshot(expected 1) error = %v", err)
	}

	if saved.ConfigVersion != 2 {
		t.Fatalf("second save version = %d, want 2", saved.ConfigVersion)
	}
}

func hasFingerprintProfile(profiles []config.FingerprintProfile, name string) bool {
	for _, p := range profiles {
		if p.Name == name {
			return true
		}
	}

	return false
}
