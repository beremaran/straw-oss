package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/v2/internal/config"
)

// This file adds the read paths (single-resource Get and paginated List) the
// config admin API surface needs on top of the write paths in
// postgres_config_store.go (docs/tasks/p0/20). Sorting matches the shared
// contract in docs/planning/26: created_at descending, then id ascending.

const (
	defaultConfigListLimit = 50
	maxConfigListLimit     = 200
)

// clampConfigListLimit applies the shared pagination defaults/bounds from
// docs/planning/26 ("Shared Config API Contract").
func clampConfigListLimit(limit int) int {
	if limit <= 0 {
		return defaultConfigListLimit
	}

	if limit > maxConfigListLimit {
		return maxConfigListLimit
	}

	return limit
}

// ---- Routing rules ----

// GetRoutingRule returns a live (non-deleted) routing rule, or
// ErrConfigResourceNotFound.
func (s *PostgresConfigStore) GetRoutingRule(ctx context.Context, tenantID, id string) (RoutingRuleRecord, error) {
	var (
		record    RoutingRuleRecord
		matchJSON []byte
		ttl       int64
		version   int64
	)

	record.TenantID = tenantID
	record.ID = id

	err := s.pool.QueryRow(
		ctx,
		`SELECT priority, enabled, match_conditions_jsonb, target_pool_id,
		        COALESCE(sticky_session_ttl_seconds, 0), allow_sticky_fallback, created_at, config_version
		 FROM routing_rules WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
		tenantID, id,
	).Scan(&record.Priority, &record.Enabled, &matchJSON, &record.TargetPoolID, &ttl,
		&record.AllowStickyFallback, &record.CreatedAt, &version)
	if err != nil {
		return RoutingRuleRecord{}, mapConfigResourceNotFound(err)
	}

	record.Match, err = unmarshalMatchConditions(matchJSON)
	if err != nil {
		return RoutingRuleRecord{}, err
	}

	record.StickySessionTTLSeconds, err = dbUint32(ttl, "sticky session ttl seconds")
	if err != nil {
		return RoutingRuleRecord{}, err
	}

	record.ConfigVersion, err = dbUint64(version, "routing rule config version")
	if err != nil {
		return RoutingRuleRecord{}, err
	}

	return record, nil
}

// ListRoutingRules returns a page of live routing rules for a tenant.
func (s *PostgresConfigStore) ListRoutingRules(ctx context.Context, tenantID string, limit, offset int) ([]RoutingRuleRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, priority, enabled, match_conditions_jsonb, target_pool_id,
		        COALESCE(sticky_session_ttl_seconds, 0), allow_sticky_fallback, created_at, config_version
		 FROM routing_rules
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC, id ASC
		 LIMIT $2 OFFSET $3`,
		tenantID, clampConfigListLimit(limit), offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list routing rules: %w", err)
	}

	defer rows.Close()

	var out []RoutingRuleRecord

	for rows.Next() {
		var (
			record    RoutingRuleRecord
			matchJSON []byte
			ttl       int64
			version   int64
		)

		record.TenantID = tenantID

		scanErr := rows.Scan(&record.ID, &record.Priority, &record.Enabled, &matchJSON, &record.TargetPoolID,
			&ttl, &record.AllowStickyFallback, &record.CreatedAt, &version)
		if scanErr != nil {
			return nil, fmt.Errorf("scan routing rule: %w", scanErr)
		}

		record.Match, err = unmarshalMatchConditions(matchJSON)
		if err != nil {
			return nil, err
		}

		record.StickySessionTTLSeconds, err = dbUint32(ttl, "sticky session ttl seconds")
		if err != nil {
			return nil, err
		}

		record.ConfigVersion, err = dbUint64(version, "routing rule config version")
		if err != nil {
			return nil, err
		}

		out = append(out, record)
	}

	return out, checkRows(rows, "routing rule list")
}

func unmarshalMatchConditions(matchJSON []byte) (config.MatchConditions, error) {
	var mc matchConditionsJSON

	if len(matchJSON) > 0 {
		err := json.Unmarshal(matchJSON, &mc)
		if err != nil {
			return config.MatchConditions{}, fmt.Errorf("unmarshal match conditions: %w", err)
		}
	}

	return matchFromJSON(mc), nil
}

// ---- Executor pools ----

// GetExecutorPool returns a live (non-deleted) executor pool, or
// ErrConfigResourceNotFound.
func (s *PostgresConfigStore) GetExecutorPool(ctx context.Context, tenantID, id string) (ExecutorPoolRecord, error) {
	var (
		record        ExecutorPoolRecord
		tagsJSON      []byte
		ipTypesJSON   []byte
		countriesJSON []byte
		regionsJSON   []byte
		version       int64
	)

	record.TenantID = tenantID
	record.ID = id

	err := s.pool.QueryRow(
		ctx,
		`SELECT executor_type, tags_jsonb, enabled, allow_degraded_workers,
		        allowed_ip_types_jsonb, allowed_countries_jsonb, allowed_regions_jsonb,
		        created_at, config_version
		 FROM executor_pools WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
		tenantID, id,
	).Scan(&record.ExecutorType, &tagsJSON, &record.Enabled, &record.AllowDegradedWorkers,
		&ipTypesJSON, &countriesJSON, &regionsJSON, &record.CreatedAt, &version)
	if err != nil {
		return ExecutorPoolRecord{}, mapConfigResourceNotFound(err)
	}

	err = unmarshalPoolCapabilityFields(&record.ExecutorPool, tagsJSON, ipTypesJSON, countriesJSON, regionsJSON)
	if err != nil {
		return ExecutorPoolRecord{}, err
	}

	record.ConfigVersion, err = dbUint64(version, "executor pool config version")
	if err != nil {
		return ExecutorPoolRecord{}, err
	}

	return record, nil
}

// unmarshalPoolCapabilityFields decodes the jsonb tag/capability-restriction
// columns shared by GetExecutorPool, ListExecutorPools, and snapshot assembly.
func unmarshalPoolCapabilityFields(pool *config.ExecutorPool, tagsJSON, ipTypesJSON, countriesJSON, regionsJSON []byte) error {
	fields := []struct {
		name string
		json []byte
		out  *[]string
	}{
		{"tags", tagsJSON, &pool.Tags},
		{"allowed ip types", ipTypesJSON, &pool.AllowedIPTypes},
		{"allowed countries", countriesJSON, &pool.AllowedCountries},
		{"allowed regions", regionsJSON, &pool.AllowedRegions},
	}

	for _, f := range fields {
		if len(f.json) == 0 {
			continue
		}

		err := json.Unmarshal(f.json, f.out)
		if err != nil {
			return fmt.Errorf("unmarshal pool %s: %w", f.name, err)
		}
	}

	return nil
}

// ListExecutorPools returns a page of live executor pools for a tenant.
func (s *PostgresConfigStore) ListExecutorPools(ctx context.Context, tenantID string, limit, offset int) ([]ExecutorPoolRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, executor_type, tags_jsonb, enabled, allow_degraded_workers,
		        allowed_ip_types_jsonb, allowed_countries_jsonb, allowed_regions_jsonb,
		        created_at, config_version
		 FROM executor_pools
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC, id ASC
		 LIMIT $2 OFFSET $3`,
		tenantID, clampConfigListLimit(limit), offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list executor pools: %w", err)
	}

	defer rows.Close()

	var out []ExecutorPoolRecord

	for rows.Next() {
		var (
			record        ExecutorPoolRecord
			tagsJSON      []byte
			ipTypesJSON   []byte
			countriesJSON []byte
			regionsJSON   []byte
			version       int64
		)

		record.TenantID = tenantID

		scanErr := rows.Scan(&record.ID, &record.ExecutorType, &tagsJSON, &record.Enabled,
			&record.AllowDegradedWorkers, &ipTypesJSON, &countriesJSON, &regionsJSON,
			&record.CreatedAt, &version)
		if scanErr != nil {
			return nil, fmt.Errorf("scan executor pool: %w", scanErr)
		}

		err = unmarshalPoolCapabilityFields(&record.ExecutorPool, tagsJSON, ipTypesJSON, countriesJSON, regionsJSON)
		if err != nil {
			return nil, err
		}

		record.ConfigVersion, err = dbUint64(version, "executor pool config version")
		if err != nil {
			return nil, err
		}

		out = append(out, record)
	}

	return out, checkRows(rows, "executor pool list")
}

// ---- Deny rules ----

// GetDenyRule returns a live deny rule, or ErrConfigResourceNotFound.
func (s *PostgresConfigStore) GetDenyRule(ctx context.Context, tenantID, id string) (DenyRuleRecord, error) {
	var (
		record  DenyRuleRecord
		version int64
	)

	record.TenantID = tenantID
	record.ID = id

	err := s.pool.QueryRow(
		ctx,
		`SELECT rule_type, action, enabled, raw_pattern,
		        COALESCE(normalized_host, ''), COALESCE(normalized_cidr::text, ''),
		        COALESCE(normalized_ip::text, ''), COALESCE(normalized_cname, ''),
		        created_at, config_version
		 FROM deny_rules WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
		tenantID, id,
	).Scan(&record.RuleType, &record.Action, &record.Enabled, &record.RawPattern,
		&record.NormalizedHost, &record.NormalizedCIDR, &record.NormalizedIP, &record.NormalizedName,
		&record.CreatedAt, &version)
	if err != nil {
		return DenyRuleRecord{}, mapConfigResourceNotFound(err)
	}

	record.ConfigVersion, err = dbUint64(version, "deny rule config version")
	if err != nil {
		return DenyRuleRecord{}, err
	}

	return record, nil
}

// ListDenyRules returns a page of live deny rules for a tenant.
func (s *PostgresConfigStore) ListDenyRules(ctx context.Context, tenantID string, limit, offset int) ([]DenyRuleRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, rule_type, action, enabled, raw_pattern,
		        COALESCE(normalized_host, ''), COALESCE(normalized_cidr::text, ''),
		        COALESCE(normalized_ip::text, ''), COALESCE(normalized_cname, ''),
		        created_at, config_version
		 FROM deny_rules
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC, id ASC
		 LIMIT $2 OFFSET $3`,
		tenantID, clampConfigListLimit(limit), offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list deny rules: %w", err)
	}

	defer rows.Close()

	var out []DenyRuleRecord

	for rows.Next() {
		var (
			record  DenyRuleRecord
			version int64
		)

		record.TenantID = tenantID

		scanErr := rows.Scan(&record.ID, &record.RuleType, &record.Action, &record.Enabled, &record.RawPattern,
			&record.NormalizedHost, &record.NormalizedCIDR, &record.NormalizedIP, &record.NormalizedName,
			&record.CreatedAt, &version)
		if scanErr != nil {
			return nil, fmt.Errorf("scan deny rule: %w", scanErr)
		}

		record.ConfigVersion, err = dbUint64(version, "deny rule config version")
		if err != nil {
			return nil, err
		}

		out = append(out, record)
	}

	return out, checkRows(rows, "deny rule list")
}

// ---- Injection policies ----

// GetInjectionPolicy returns a live injection policy, or
// ErrConfigResourceNotFound.
func (s *PostgresConfigStore) GetInjectionPolicy(ctx context.Context, tenantID, id string) (InjectionPolicyRecord, error) {
	var (
		record  InjectionPolicyRecord
		opsJSON []byte
		version int64
	)

	record.TenantID = tenantID
	record.ID = id

	err := s.pool.QueryRow(
		ctx,
		`SELECT enabled, operations, created_at, config_version
		 FROM injection_policies WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
		tenantID, id,
	).Scan(&record.Enabled, &opsJSON, &record.CreatedAt, &version)
	if err != nil {
		return InjectionPolicyRecord{}, mapConfigResourceNotFound(err)
	}

	record.Operations, err = unmarshalInjectionOperations(opsJSON)
	if err != nil {
		return InjectionPolicyRecord{}, err
	}

	record.ConfigVersion, err = dbUint64(version, "injection policy config version")
	if err != nil {
		return InjectionPolicyRecord{}, err
	}

	return record, nil
}

// ListInjectionPolicies returns a page of live injection policies for a tenant.
func (s *PostgresConfigStore) ListInjectionPolicies(ctx context.Context, tenantID string, limit, offset int) ([]InjectionPolicyRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, enabled, operations, created_at, config_version
		 FROM injection_policies
		 WHERE tenant_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC, id ASC
		 LIMIT $2 OFFSET $3`,
		tenantID, clampConfigListLimit(limit), offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list injection policies: %w", err)
	}

	defer rows.Close()

	var out []InjectionPolicyRecord

	for rows.Next() {
		var (
			record  InjectionPolicyRecord
			opsJSON []byte
			version int64
		)

		record.TenantID = tenantID

		scanErr := rows.Scan(&record.ID, &record.Enabled, &opsJSON, &record.CreatedAt, &version)
		if scanErr != nil {
			return nil, fmt.Errorf("scan injection policy: %w", scanErr)
		}

		record.Operations, err = unmarshalInjectionOperations(opsJSON)
		if err != nil {
			return nil, err
		}

		record.ConfigVersion, err = dbUint64(version, "injection policy config version")
		if err != nil {
			return nil, err
		}

		out = append(out, record)
	}

	return out, checkRows(rows, "injection policy list")
}

func unmarshalInjectionOperations(opsJSON []byte) ([]config.InjectionOperation, error) {
	if len(opsJSON) == 0 {
		return nil, nil
	}

	var ops []injectionOperationJSON

	err := json.Unmarshal(opsJSON, &ops)
	if err != nil {
		return nil, fmt.Errorf("unmarshal injection operations: %w", err)
	}

	out := make([]config.InjectionOperation, 0, len(ops))
	for _, op := range ops {
		out = append(out, config.InjectionOperation{Op: op.Op, HeaderName: op.HeaderName, ValueBase64: op.ValueBase64})
	}

	return out, nil
}

// ---- Fingerprint profiles (read-only in P0) ----

// ListFingerprintProfiles returns every enabled fingerprint profile visible to
// a tenant: global built-ins plus any tenant-scoped rows. P0 seeds only global
// profiles and exposes no write path (docs/planning/26).
func (s *PostgresConfigStore) ListFingerprintProfiles(ctx context.Context, tenantID string) ([]FingerprintProfileRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT name, scope_type, supported_by_worker, enabled, created_at, config_version
		 FROM fingerprint_profiles
		 WHERE (scope_type = 'global' OR tenant_id = $1) AND enabled = true
		 ORDER BY name`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list fingerprint profiles: %w", err)
	}

	defer rows.Close()

	var out []FingerprintProfileRecord

	for rows.Next() {
		var (
			record  FingerprintProfileRecord
			version int64
		)

		scanErr := rows.Scan(&record.Name, &record.ScopeType, &record.SupportedByWorker, &record.Enabled,
			&record.CreatedAt, &version)
		if scanErr != nil {
			return nil, fmt.Errorf("scan fingerprint profile: %w", scanErr)
		}

		record.ConfigVersion, err = dbUint64(version, "fingerprint profile config version")
		if err != nil {
			return nil, err
		}

		out = append(out, record)
	}

	return out, checkRows(rows, "fingerprint profile list")
}

// mapConfigResourceNotFound maps "no rows" to the shared not-found sentinel;
// any other error is wrapped as-is so callers see the real failure.
func mapConfigResourceNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConfigResourceNotFound
	}

	return fmt.Errorf("get config resource: %w", err)
}
