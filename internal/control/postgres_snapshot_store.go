package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/v2/internal/config"
)

// This file makes PostgresConfigStore satisfy SnapshotStore: it reads the
// current tenant config version and assembles the immutable tenant snapshot
// straight from the durable config tables (docs/planning/21, 25). Assembly runs
// in a read-only repeatable-read transaction so every resource in one snapshot
// reflects a single consistent point in time.

// CurrentTenantConfigVersion returns the tenant's latest config version, or 0
// when the tenant has no version row yet.
func (s *PostgresConfigStore) CurrentTenantConfigVersion(ctx context.Context, tenantID string) (uint64, error) {
	var v int64

	err := s.pool.QueryRow(ctx,
		`SELECT config_version FROM tenant_config_versions WHERE tenant_id = $1`,
		tenantID,
	).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}

		return 0, fmt.Errorf("current tenant config version: %w", err)
	}

	version, err := dbUint64(v, "tenant config version")
	if err != nil {
		return 0, err
	}

	return version, nil
}

// LoadTenantSnapshot assembles the full immutable snapshot for a tenant at the
// requested current config version. Older versions are retained by ConfigCache
// clones for in-flight requests; Postgres rejects stale requested versions
// rather than silently returning different state under the wrong key.
func (s *PostgresConfigStore) LoadTenantSnapshot(ctx context.Context, tenantID string, version uint64) (config.TenantSnapshot, error) {
	var snapshot config.TenantSnapshot

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return config.TenantSnapshot{}, fmt.Errorf("begin snapshot read tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	snapshot, err = assembleTenantSnapshot(ctx, tx, tenantID, version)
	if err != nil {
		return config.TenantSnapshot{}, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return config.TenantSnapshot{}, fmt.Errorf("commit snapshot read tx: %w", err)
	}

	return snapshot, nil
}

// SaveTenantSnapshot bumps the tenant config version under optimistic
// concurrency, then returns the freshly assembled snapshot at the new version.
// In the Postgres model the resource tables are the source of truth, so this
// path exists only to advance the aggregate version (e.g. after an API-key
// revocation) and re-read the derived snapshot; the passed snapshot's derived
// fields are ignored.
func (s *PostgresConfigStore) SaveTenantSnapshot(ctx context.Context, snapshot config.TenantSnapshot, expectedVersion uint64) (config.TenantSnapshot, error) {
	var newVersion uint64

	err := inConfigTx(ctx, s.pool, func(tx pgx.Tx) error {
		v, bumpErr := bumpTenantConfigVersionOptimistic(ctx, tx, snapshot.TenantID, expectedVersion)
		if bumpErr != nil {
			return bumpErr
		}

		newVersion = v

		return nil
	})
	if err != nil {
		return config.TenantSnapshot{}, err
	}

	return s.LoadTenantSnapshot(ctx, snapshot.TenantID, newVersion)
}

// bumpTenantConfigVersionOptimistic increments the tenant version only when the
// stored version equals expected, initializing the row for a first write at
// expected 0. Returns ErrVersionConflict on mismatch.
func bumpTenantConfigVersionOptimistic(ctx context.Context, tx pgx.Tx, tenantID string, expected uint64) (uint64, error) {
	var v int64

	expectedParam, err := configVersionParam(expected)
	if err != nil {
		return 0, err
	}

	err = tx.QueryRow(ctx,
		`UPDATE tenant_config_versions
		 SET config_version = config_version + 1, updated_at = now()
		 WHERE tenant_id = $1 AND config_version = $2
		 RETURNING config_version`,
		tenantID, expectedParam,
	).Scan(&v)
	if err == nil {
		version, convErr := dbUint64(v, "tenant config version")
		if convErr != nil {
			return 0, convErr
		}

		return version, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("optimistic bump tenant config version: %w", err)
	}

	// No row matched the expected version. Only a brand-new tenant (expected 0
	// with no row yet) may create one; anything else is a version conflict.
	if expected != 0 {
		return 0, ErrVersionConflict
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO tenant_config_versions (tenant_id, config_version, updated_at)
		 VALUES ($1, 1, now())
		 ON CONFLICT (tenant_id) DO NOTHING
		 RETURNING config_version`,
		tenantID,
	).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrVersionConflict
		}

		return 0, fmt.Errorf("init tenant config version: %w", err)
	}

	version, err := dbUint64(v, "tenant config version")
	if err != nil {
		return 0, err
	}

	return version, nil
}

// assembleTenantSnapshot reads every config resource for a tenant into one
// snapshot. Deleted resources are excluded; the version is read in the same
// transaction as the data.
func assembleTenantSnapshot(ctx context.Context, tx pgx.Tx, tenantID string, requestedVersion uint64) (config.TenantSnapshot, error) {
	snapshot := config.TenantSnapshot{TenantID: tenantID}

	version, err := readTenantVersion(ctx, tx, tenantID)
	if err != nil {
		return config.TenantSnapshot{}, err
	}

	if version != requestedVersion {
		return config.TenantSnapshot{}, ErrVersionConflict
	}

	snapshot.ConfigVersion = version

	readers := []func(context.Context, pgx.Tx, string, *config.TenantSnapshot) error{
		readRevokedAPIKeys,
		readRoutingRules,
		readExecutorPools,
		readDenyRules,
		readInjectionPolicies,
		readFingerprintProfiles,
		readRateLimits,
		readQuota,
		readWorkerAdminStates,
		readTenantWorkerOverrides,
	}

	for _, read := range readers {
		err := read(ctx, tx, tenantID, &snapshot)
		if err != nil {
			return config.TenantSnapshot{}, err
		}
	}

	return snapshot, nil
}

func readTenantVersion(ctx context.Context, tx pgx.Tx, tenantID string) (uint64, error) {
	var v int64

	err := tx.QueryRow(ctx,
		`SELECT config_version FROM tenant_config_versions WHERE tenant_id = $1`,
		tenantID,
	).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}

		return 0, fmt.Errorf("read tenant version: %w", err)
	}

	version, err := dbUint64(v, "tenant config version")
	if err != nil {
		return 0, err
	}

	return version, nil
}

func checkRows(rows pgx.Rows, label string) error {
	err := rows.Err()
	if err != nil {
		return fmt.Errorf("%s rows: %w", label, err)
	}

	return nil
}

func readRevokedAPIKeys(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT id FROM api_keys WHERE tenant_id = $1 AND status = 'revoked' ORDER BY id`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read revoked api keys: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var id string

		err := rows.Scan(&id)
		if err != nil {
			return fmt.Errorf("scan revoked api key: %w", err)
		}

		snap.RevokedAPIKeyIDs = append(snap.RevokedAPIKeyIDs, id)
	}

	return checkRows(rows, "revoked api key")
}

func readRoutingRules(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT id, priority, enabled, match_conditions_jsonb, target_pool_id,
		        COALESCE(sticky_session_ttl_seconds, 0), allow_sticky_fallback
		 FROM routing_rules
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY priority, id`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read routing rules: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			rule      config.RoutingRule
			matchJSON []byte
			ttl       int64
		)

		err := rows.Scan(&rule.ID, &rule.Priority, &rule.Enabled, &matchJSON,
			&rule.TargetPoolID, &ttl, &rule.AllowStickyFallback)
		if err != nil {
			return fmt.Errorf("scan routing rule: %w", err)
		}

		var mc matchConditionsJSON
		if len(matchJSON) > 0 {
			err = json.Unmarshal(matchJSON, &mc)
			if err != nil {
				return fmt.Errorf("unmarshal match conditions: %w", err)
			}
		}

		rule.Match = matchFromJSON(mc)

		rule.StickySessionTTLSeconds, err = dbUint32(ttl, "sticky session ttl seconds")
		if err != nil {
			return err
		}

		snap.RoutingRules = append(snap.RoutingRules, rule)
	}

	return checkRows(rows, "routing rule")
}

func readExecutorPools(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT id, executor_type, tags_jsonb, enabled
		 FROM executor_pools
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY id`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read executor pools: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			pool     config.ExecutorPool
			tagsJSON []byte
		)

		err := rows.Scan(&pool.ID, &pool.ExecutorType, &tagsJSON, &pool.Enabled)
		if err != nil {
			return fmt.Errorf("scan executor pool: %w", err)
		}

		if len(tagsJSON) > 0 {
			err = json.Unmarshal(tagsJSON, &pool.Tags)
			if err != nil {
				return fmt.Errorf("unmarshal pool tags: %w", err)
			}
		}

		snap.ExecutorPools = append(snap.ExecutorPools, pool)
	}

	return checkRows(rows, "executor pool")
}

func readDenyRules(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT id, rule_type, action, enabled, raw_pattern,
		        COALESCE(normalized_host, ''), COALESCE(normalized_cidr::text, ''),
		        COALESCE(normalized_ip::text, ''), COALESCE(normalized_cname, '')
		 FROM deny_rules
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY id`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read deny rules: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var rule config.DenyRule

		err := rows.Scan(&rule.ID, &rule.RuleType, &rule.Action, &rule.Enabled, &rule.RawPattern,
			&rule.NormalizedHost, &rule.NormalizedCIDR, &rule.NormalizedIP, &rule.NormalizedName)
		if err != nil {
			return fmt.Errorf("scan deny rule: %w", err)
		}

		snap.DenyRules = append(snap.DenyRules, rule)
	}

	return checkRows(rows, "deny rule")
}

func readInjectionPolicies(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT id, enabled, operations
		 FROM injection_policies
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY id`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read injection policies: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			pol     config.InjectionPolicy
			opsJSON []byte
		)

		err := rows.Scan(&pol.ID, &pol.Enabled, &opsJSON)
		if err != nil {
			return fmt.Errorf("scan injection policy: %w", err)
		}

		var ops []injectionOperationJSON
		if len(opsJSON) > 0 {
			err = json.Unmarshal(opsJSON, &ops)
			if err != nil {
				return fmt.Errorf("unmarshal injection operations: %w", err)
			}
		}

		for _, op := range ops {
			pol.Operations = append(pol.Operations, config.InjectionOperation{
				Op: op.Op, HeaderName: op.HeaderName, ValueBase64: op.ValueBase64,
			})
		}

		snap.InjectionPolicies = append(snap.InjectionPolicies, pol)
	}

	return checkRows(rows, "injection policy")
}

func readFingerprintProfiles(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT name, scope_type, supported_by_worker, enabled
		 FROM fingerprint_profiles
		 WHERE (scope_type = 'global' OR tenant_id = $1) AND enabled = true
		 ORDER BY name`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read fingerprint profiles: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var fp config.FingerprintProfile

		err := rows.Scan(&fp.Name, &fp.ScopeType, &fp.SupportedByWorker, &fp.Enabled)
		if err != nil {
			return fmt.Errorf("scan fingerprint profile: %w", err)
		}

		snap.FingerprintProfiles = append(snap.FingerprintProfiles, fp)
	}

	return checkRows(rows, "fingerprint profile")
}

func readRateLimits(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT dimension, key, window_ms, limit_count, fail_policy
		 FROM rate_limit_configs
		 WHERE tenant_id = $1 AND enabled = true
		 ORDER BY dimension, key`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read rate limits: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			rule       config.RateLimitRule
			windowMS   int64
			limitCount int64
		)

		err := rows.Scan(&rule.Dimension, &rule.Key, &windowMS, &limitCount, &rule.FailPolicy)
		if err != nil {
			return fmt.Errorf("scan rate limit: %w", err)
		}

		rule.WindowSeconds, err = dbWindowSeconds(windowMS)
		if err != nil {
			return err
		}

		rule.MaxRequests, err = dbUint32(limitCount, "rate limit count")
		if err != nil {
			return err
		}

		snap.RateLimits = append(snap.RateLimits, rule)
	}

	return checkRows(rows, "rate limit")
}

func readQuota(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	var (
		requestLimit   *int64
		bandwidthLimit *int64
		quota          config.QuotaConfig
	)

	err := tx.QueryRow(ctx,
		`SELECT quota_period, request_count_limit, bandwidth_bytes_limit,
		        count_on_admission, fail_policy, enabled
		 FROM quota_configs
		 WHERE tenant_id = $1 AND quota_period = 'monthly'`,
		tenantID,
	).Scan(&quota.Period, &requestLimit, &bandwidthLimit, &quota.CountOnAdmission, &quota.FailPolicy, &quota.Enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}

		return fmt.Errorf("read quota: %w", err)
	}

	quota.RequestCountLimit = derefInt64(requestLimit)
	quota.BandwidthBytesLimit = derefInt64(bandwidthLimit)
	snap.Quota = quota

	return nil
}

func readWorkerAdminStates(ctx context.Context, tx pgx.Tx, _ string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT worker_id, disabled, COALESCE(disabled_reason, '')
		 FROM worker_admin_state
		 WHERE disabled = true
		 ORDER BY worker_id`,
	)
	if err != nil {
		return fmt.Errorf("read worker admin state: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var state config.WorkerAdminState

		err := rows.Scan(&state.WorkerID, &state.Disabled, &state.DisabledReason)
		if err != nil {
			return fmt.Errorf("scan worker admin state: %w", err)
		}

		snap.WorkerAdminStates = append(snap.WorkerAdminStates, state)
	}

	return checkRows(rows, "worker admin state")
}

func readTenantWorkerOverrides(ctx context.Context, tx pgx.Tx, tenantID string, snap *config.TenantSnapshot) error {
	rows, err := tx.Query(ctx,
		`SELECT worker_id, disabled, COALESCE(disabled_reason, '')
		 FROM tenant_worker_admin_state
		 WHERE tenant_id = $1
		 ORDER BY worker_id`,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("read tenant worker overrides: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var override config.TenantWorkerOverride

		err := rows.Scan(&override.WorkerID, &override.Disabled, &override.DisabledReason)
		if err != nil {
			return fmt.Errorf("scan tenant worker override: %w", err)
		}

		snap.TenantWorkerOverrides = append(snap.TenantWorkerOverrides, override)
	}

	return checkRows(rows, "tenant worker override")
}
