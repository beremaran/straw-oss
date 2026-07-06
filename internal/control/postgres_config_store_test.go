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
	pgConfigFastTag      = pgTestFastTag
	pgConfigRegionAUEast = pgTestRegionAUEast
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
		Tags:                 []string{pgConfigFastTag, "au"},
		Enabled:              true,
		AllowDegradedWorkers: true,
		AllowedIPTypes:       []string{ipTypeDatacenter},
		AllowedCountries:     []string{"AU"},
		AllowedRegions:       []string{pgConfigRegionAUEast},
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertExecutorPool() error = %v", err)
	}

	_, _, err = store.UpsertRoutingRule(ctx, pgTestTenantA, config.RoutingRule{
		ID:           "route_keep",
		Priority:     20,
		Enabled:      true,
		Match:        config.MatchConditions{TargetHost: "*.example.com", Tags: []string{pgConfigFastTag}},
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
	if gotPool := snap.ExecutorPools[0]; len(gotPool.AllowedIPTypes) != 1 || gotPool.AllowedIPTypes[0] != ipTypeDatacenter ||
		len(gotPool.AllowedCountries) != 1 || gotPool.AllowedCountries[0] != "AU" ||
		len(gotPool.AllowedRegions) != 1 || gotPool.AllowedRegions[0] != pgConfigRegionAUEast {
		t.Fatalf("executor pool capability fields = %+v, want datacenter/AU/au-east-1", gotPool)
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

func TestPostgresConfigRollbackRestoresAuditBackedResources(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	ctx := context.Background()
	store := NewPostgresConfigStore(pool)
	actor := ConfigActor{ActorType: configActorTypeAPIKey, ActorID: pgConfigActorID}

	_, targetVersion, err := store.UpsertRoutingRule(ctx, pgTestTenantA, config.RoutingRule{
		ID: "route_rollback", Priority: 10, Enabled: true, TargetPoolID: pgConfigPoolA,
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertRoutingRule(initial) error = %v", err)
	}

	_, currentVersion, err := store.UpsertRoutingRule(ctx, pgTestTenantA, config.RoutingRule{
		ID: "route_rollback", Priority: 99, Enabled: true, TargetPoolID: pgConfigPoolA,
	}, 1, actor)
	if err != nil {
		t.Fatalf("UpsertRoutingRule(update) error = %v", err)
	}

	newVersion, err := store.RollbackConfig(ctx, pgTestTenantA, ConfigRollbackRequest{
		ExpectedConfigVersion: currentVersion,
		TargetConfigVersion:   targetVersion,
		Reason:                "restore route",
	}, actor)
	if err != nil {
		t.Fatalf("RollbackConfig() error = %v", err)
	}
	if newVersion != currentVersion+1 {
		t.Fatalf("rollback version = %d, want %d", newVersion, currentVersion+1)
	}

	restored, err := store.GetRoutingRule(ctx, pgTestTenantA, "route_rollback")
	if err != nil {
		t.Fatalf("GetRoutingRule() error = %v", err)
	}
	if restored.Priority != 10 || restored.ConfigVersion != newVersion {
		t.Fatalf("restored route = %+v, want priority 10 version %d", restored, newVersion)
	}

	var auditVersion int64
	err = pool.QueryRow(ctx,
		`SELECT config_version FROM config_audit_source
		 WHERE tenant_id = $1 AND resource_type = $2 AND action = $3
		 ORDER BY id DESC LIMIT 1`,
		pgTestTenantA, resourceTypeConfigRollback, configActionRollback,
	).Scan(&auditVersion)
	if err != nil {
		t.Fatalf("read rollback audit: %v", err)
	}
	wantVersion, err := dbUint64(auditVersion, "rollback audit version")
	if err != nil {
		t.Fatalf("convert rollback audit version: %v", err)
	}
	if wantVersion != newVersion {
		t.Fatalf("rollback audit version = %d, want %d", auditVersion, newVersion)
	}
}

func TestPostgresConfigRollbackRejectsRedactedInjectionPolicy(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	ctx := context.Background()
	store := NewPostgresConfigStore(pool)
	actor := ConfigActor{ActorType: configActorTypeAPIKey, ActorID: pgConfigActorID}

	_, targetVersion, err := store.UpsertRoutingRule(ctx, pgTestTenantA, config.RoutingRule{
		ID: "route_before_secret", Priority: 1, Enabled: true, TargetPoolID: pgConfigPoolA,
	}, 0, actor)
	if err != nil {
		t.Fatalf("UpsertRoutingRule() error = %v", err)
	}

	_, currentVersion, err := store.UpsertInjectionPolicy(ctx, pgTestTenantA, config.InjectionPolicy{
		ID:      "inject_secret_rollback",
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

	_, err = store.RollbackConfig(ctx, pgTestTenantA, ConfigRollbackRequest{
		ExpectedConfigVersion: currentVersion,
		TargetConfigVersion:   targetVersion,
		Reason:                "crosses secret",
	}, actor)
	if !errors.Is(err, ErrConfigRollbackSecretRedacted) {
		t.Fatalf("RollbackConfig() error = %v, want ErrConfigRollbackSecretRedacted", err)
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

func TestPostgresDenyRuleMigrationMapping(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)
	ctx := context.Background()

	// Temporarily drop constraints to seed old schema values
	_, err := pool.Exec(ctx, `
		ALTER TABLE deny_rules DROP CONSTRAINT IF EXISTS deny_rules_rule_type_check;
		ALTER TABLE deny_rules DROP CONSTRAINT IF EXISTS deny_rules_action_check;
	`)
	if err != nil {
		t.Fatalf("failed to drop constraints: %v", err)
	}

	// Seed pre-migration shapes using raw SQL
	_, err = pool.Exec(ctx, `
		INSERT INTO deny_rules (tenant_id, id, rule_type, action, raw_pattern, normalized_ip, normalized_cname)
		VALUES
			($1, 'old_ipv4_allow', 'ip', 'allow', '10.0.0.1', '10.0.0.1'::inet, NULL),
			($1, 'old_ipv6_deny', 'ip', 'deny', '2001:db8::1', '2001:db8::1'::inet, NULL),
			($1, 'old_cname_deny', 'cname', 'deny', 'cname.example', NULL, 'cname.example')
	`, pgTestTenantA)
	if err != nil {
		t.Fatalf("failed to seed test rules: %v", err)
	}

	// Execute migration queries manually to check their logic
	_, err = pool.Exec(ctx, `
		UPDATE deny_rules
		   SET normalized_cidr = (host(normalized_ip) || '/' || CASE WHEN family(normalized_ip) = 6 THEN '128' ELSE '32' END)::cidr
		 WHERE rule_type = 'ip' AND normalized_ip IS NOT NULL AND normalized_cidr IS NULL
	`)
	if err != nil {
		t.Fatalf("failed to update normalized_cidr: %v", err)
	}

	_, err = pool.Exec(ctx, "UPDATE deny_rules SET rule_type = 'cidr' WHERE rule_type = 'ip'")
	if err != nil {
		t.Fatalf("failed to update rule_type ip -> cidr: %v", err)
	}

	_, err = pool.Exec(ctx, "UPDATE deny_rules SET rule_type = 'cname_suffix' WHERE rule_type = 'cname'")
	if err != nil {
		t.Fatalf("failed to update rule_type cname -> cname_suffix: %v", err)
	}

	_, err = pool.Exec(ctx, "UPDATE deny_rules SET action = 'allow_override' WHERE action = 'allow'")
	if err != nil {
		t.Fatalf("failed to update action allow -> allow_override: %v", err)
	}

	// Re-add constraints to verify they succeed on the updated data
	_, err = pool.Exec(ctx, `
		ALTER TABLE deny_rules ADD CONSTRAINT deny_rules_rule_type_check
		  CHECK (rule_type IN ('cidr', 'host', 'host_suffix', 'cname_suffix', 'metadata_ip', 'private_range'));
		ALTER TABLE deny_rules ADD CONSTRAINT deny_rules_action_check
		  CHECK (action IN ('deny', 'allow_override'));
	`)
	if err != nil {
		t.Fatalf("failed to re-add constraints: %v", err)
	}

	// Verify mappings and check results
	type resultRow struct {
		ID             string
		RuleType       string
		Action         string
		NormalizedCIDR string
	}

	rows, err := pool.Query(ctx, `
		SELECT id, rule_type, action, COALESCE(normalized_cidr::text, '')
		FROM deny_rules
		WHERE tenant_id = $1
		ORDER BY id
	`, pgTestTenantA)
	if err != nil {
		t.Fatalf("failed to query results: %v", err)
	}
	defer rows.Close()

	results := make(map[string]resultRow)
	for rows.Next() {
		var r resultRow
		err = rows.Scan(&r.ID, &r.RuleType, &r.Action, &r.NormalizedCIDR)
		if err != nil {
			t.Fatalf("scan row: %v", err)
		}
		results[r.ID] = r
	}

	// Verify old_ipv4_allow -> cidr + allow_override + /32
	ipv4, ok := results["old_ipv4_allow"]
	if !ok {
		t.Fatal("missing old_ipv4_allow")
	}
	if ipv4.RuleType != denyRuleTypeCIDR || ipv4.Action != "allow_override" || ipv4.NormalizedCIDR != "10.0.0.1/32" {
		t.Errorf("ipv4 mapped wrong: %+v", ipv4)
	}

	// Verify old_ipv6_deny -> cidr + deny + /128
	ipv6, ok := results["old_ipv6_deny"]
	if !ok {
		t.Fatal("missing old_ipv6_deny")
	}
	if ipv6.RuleType != denyRuleTypeCIDR || ipv6.Action != "deny" || ipv6.NormalizedCIDR != "2001:db8::1/128" {
		t.Errorf("ipv6 mapped wrong: %+v", ipv6)
	}

	// Verify old_cname_deny -> cname_suffix + deny
	cname, ok := results["old_cname_deny"]
	if !ok {
		t.Fatal("missing old_cname_deny")
	}
	if cname.RuleType != "cname_suffix" || cname.Action != "deny" {
		t.Errorf("cname mapped wrong: %+v", cname)
	}
}
