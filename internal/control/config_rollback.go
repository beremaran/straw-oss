package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/v2/internal/config"
)

const (
	resourceTypeExecutorPool    = "executor_pool"
	resourceTypeDenyRule        = "deny_rule"
	resourceTypeInjectionPolicy = "injection_policy"
	resourceTypeQuotaConfig     = "quota_config"
	resourceTypeRateLimitConfig = "rate_limit_config"
	resourceTypeConfigRollback  = "config_rollback"
	configActionRollback        = "rollback"
)

var (
	// ErrConfigRollbackTargetNotFound means target_config_version is not an
	// earlier tenant config version.
	ErrConfigRollbackTargetNotFound = errors.New("config rollback target not found")
	// ErrConfigRollbackSecretRedacted means rollback would need a secret value
	// that audit history intentionally redacted.
	ErrConfigRollbackSecretRedacted = errors.New("config rollback requires redacted secret")
)

// ConfigRollbackRequest is the POST /api/v1/config/rollback body.
type ConfigRollbackRequest struct {
	ExpectedConfigVersion uint64 `json:"expected_config_version"`
	TargetConfigVersion   uint64 `json:"target_config_version"`
	Reason                string `json:"reason"`
}

type rollbackState struct {
	routes     map[string]config.RoutingRule
	pools      map[string]config.ExecutorPool
	denyRules  map[string]config.DenyRule
	quota      *QuotaConfig
	rateLimits *RateLimitConfig
	seenAudit  bool
}

type rollbackMatchJSON struct {
	Tags        []string `json:"Tags"`
	Country     string   `json:"Country"`
	Region      string   `json:"Region"`
	IPType      string   `json:"IPType"`
	IngressType string   `json:"IngressType"`
	TargetHost  string   `json:"TargetHost"`
}

type rollbackRoutingRuleJSON struct {
	ID                      string            `json:"ID"`
	Priority                int               `json:"Priority"`
	Enabled                 bool              `json:"Enabled"`
	Match                   rollbackMatchJSON `json:"Match"`
	TargetPoolID            string            `json:"TargetPoolID"`
	StickySessionTTLSeconds uint32            `json:"StickySessionTTLSeconds"`
	AllowStickyFallback     bool              `json:"AllowStickyFallback"`
}

type rollbackExecutorPoolJSON struct {
	ID                   string   `json:"ID"`
	ExecutorType         string   `json:"ExecutorType"`
	Tags                 []string `json:"Tags"`
	Enabled              bool     `json:"Enabled"`
	AllowDegradedWorkers bool     `json:"AllowDegradedWorkers"`
	AllowedIPTypes       []string `json:"AllowedIPTypes"`
	AllowedCountries     []string `json:"AllowedCountries"`
	AllowedRegions       []string `json:"AllowedRegions"`
}

type rollbackDenyRuleJSON struct {
	ID             string `json:"ID"`
	RuleType       string `json:"RuleType"`
	Action         string `json:"Action"`
	Enabled        bool   `json:"Enabled"`
	Reason         string `json:"Reason"`
	RawPattern     string `json:"RawPattern"`
	NormalizedHost string `json:"NormalizedHost"`
	NormalizedCIDR string `json:"NormalizedCIDR"`
	NormalizedIP   string `json:"NormalizedIP"`
	NormalizedName string `json:"NormalizedName"`
}

type rollbackQuotaConfigJSON struct {
	TenantID           string `json:"TenantID"`
	Period             string `json:"Period"`
	MaxRequests        int64  `json:"MaxRequests"`
	MaxBandwidthBytes  int64  `json:"MaxBandwidthBytes"`
	RequestCountPolicy string `json:"RequestCountPolicy"`
	RedisFailPolicy    string `json:"RedisFailPolicy"`
	ConfigVersion      uint64 `json:"ConfigVersion"`
}

type rollbackRateLimitRuleJSON struct {
	Dimension     RateLimitDimension  `json:"Dimension"`
	Key           string              `json:"Key"`
	WindowSeconds uint32              `json:"WindowSeconds"`
	MaxRequests   uint32              `json:"MaxRequests"`
	FailPolicy    RateLimitFailPolicy `json:"FailPolicy"`
}

type rollbackRateLimitConfigJSON struct {
	TenantID      string                      `json:"TenantID"`
	Limits        []rollbackRateLimitRuleJSON `json:"Limits"`
	ConfigVersion uint64                      `json:"ConfigVersion"`
}

func newRollbackState() rollbackState {
	return rollbackState{
		routes:    map[string]config.RoutingRule{},
		pools:     map[string]config.ExecutorPool{},
		denyRules: map[string]config.DenyRule{},
	}
}

// RollbackConfig creates a new tenant config version by replaying rollback-safe
// audit-source rows up to target_config_version. Secret-bearing injection
// policies are deliberately not reconstructed from redacted audit JSON.
func (s *PostgresConfigStore) RollbackConfig(ctx context.Context, tenantID string, req ConfigRollbackRequest, actor ConfigActor) (uint64, error) {
	var newVersion uint64

	err := inConfigTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := lockedTenantConfigVersion(ctx, tx, tenantID)
		if err != nil {
			return err
		}

		if current != req.ExpectedConfigVersion {
			return ErrVersionConflict
		}

		if req.TargetConfigVersion >= current {
			return ErrConfigRollbackTargetNotFound
		}

		state, err := loadRollbackState(ctx, tx, tenantID, req.TargetConfigVersion)
		if err != nil {
			return err
		}

		if req.TargetConfigVersion > 0 && !state.seenAudit {
			return ErrConfigRollbackTargetNotFound
		}

		err = rejectRedactedRollbackSecrets(ctx, tx, tenantID, req.TargetConfigVersion, current)
		if err != nil {
			return err
		}

		newVersion, err = bumpTenantConfigVersionOptimistic(ctx, tx, tenantID, current)
		if err != nil {
			return err
		}

		return applyRollbackState(ctx, tx, tenantID, newVersion, state, req, actor)
	})
	if err != nil {
		return 0, err
	}

	return newVersion, nil
}

func lockedTenantConfigVersion(ctx context.Context, tx pgx.Tx, tenantID string) (uint64, error) {
	var v int64

	err := tx.QueryRow(ctx,
		`SELECT config_version FROM tenant_config_versions WHERE tenant_id = $1 FOR UPDATE`,
		tenantID,
	).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}

		return 0, fmt.Errorf("lock tenant config version: %w", err)
	}

	return dbUint64(v, "tenant config version")
}

func loadRollbackState(ctx context.Context, tx pgx.Tx, tenantID string, targetVersion uint64) (rollbackState, error) {
	targetParam, err := configVersionParam(targetVersion)
	if err != nil {
		return rollbackState{}, err
	}

	rows, err := tx.Query(ctx,
		`SELECT resource_type, resource_id, action, new_value_json
		 FROM config_audit_source
		 WHERE tenant_id = $1 AND config_version IS NOT NULL AND config_version <= $2
		   AND resource_type IN ($3, $4, $5, $6, $7)
		 ORDER BY config_version ASC, id ASC`,
		tenantID, targetParam, resourceTypeRoutingRule, resourceTypeExecutorPool, resourceTypeDenyRule,
		resourceTypeQuotaConfig, resourceTypeRateLimitConfig,
	)
	if err != nil {
		return rollbackState{}, fmt.Errorf("load rollback audit rows: %w", err)
	}
	defer rows.Close()

	state := newRollbackState()

	for rows.Next() {
		var (
			resourceType string
			resourceID   string
			action       string
			newJSON      []byte
		)

		err = rows.Scan(&resourceType, &resourceID, &action, &newJSON)
		if err != nil {
			return rollbackState{}, fmt.Errorf("scan rollback audit row: %w", err)
		}

		state.seenAudit = true

		if action == configActionDelete {
			deleteRollbackResource(&state, resourceType, resourceID)

			continue
		}

		if len(newJSON) == 0 {
			continue
		}

		err = setRollbackResource(&state, resourceType, resourceID, newJSON)
		if err != nil {
			return rollbackState{}, err
		}
	}

	return state, checkRows(rows, "rollback audit rows")
}

func setRollbackResource(state *rollbackState, resourceType, resourceID string, newJSON []byte) error {
	setter := rollbackSetters[resourceType]
	if setter == nil {
		return nil
	}

	return setter(state, resourceID, newJSON)
}

type rollbackSetter func(*rollbackState, string, []byte) error

var rollbackSetters = map[string]rollbackSetter{
	resourceTypeRoutingRule:     setRollbackRoutingRule,
	resourceTypeExecutorPool:    setRollbackExecutorPool,
	resourceTypeDenyRule:        setRollbackDenyRule,
	resourceTypeQuotaConfig:     setRollbackQuota,
	resourceTypeRateLimitConfig: setRollbackRateLimits,
}

func setRollbackRoutingRule(state *rollbackState, resourceID string, newJSON []byte) error {
	rule, err := decodeRollbackRoutingRule(newJSON)
	if err != nil {
		return err
	}

	rule.ID = resourceID
	state.routes[resourceID] = rule

	return nil
}

func setRollbackExecutorPool(state *rollbackState, resourceID string, newJSON []byte) error {
	pool, err := decodeRollbackExecutorPool(newJSON)
	if err != nil {
		return err
	}

	pool.ID = resourceID
	state.pools[resourceID] = pool

	return nil
}

func setRollbackDenyRule(state *rollbackState, resourceID string, newJSON []byte) error {
	rule, err := decodeRollbackDenyRule(newJSON)
	if err != nil {
		return err
	}

	rule.ID = resourceID
	state.denyRules[resourceID] = rule

	return nil
}

func setRollbackQuota(state *rollbackState, _ string, newJSON []byte) error {
	quota, err := decodeRollbackQuotaConfig(newJSON)
	if err != nil {
		return err
	}

	state.quota = &quota

	return nil
}

func setRollbackRateLimits(state *rollbackState, _ string, newJSON []byte) error {
	cfg, err := decodeRollbackRateLimitConfig(newJSON)
	if err != nil {
		return err
	}

	state.rateLimits = &cfg

	return nil
}

func decodeRollbackRoutingRule(newJSON []byte) (config.RoutingRule, error) {
	var raw rollbackRoutingRuleJSON

	err := json.Unmarshal(newJSON, &raw)
	if err != nil {
		return config.RoutingRule{}, fmt.Errorf("unmarshal rollback routing rule: %w", err)
	}

	return config.RoutingRule{
		ID: raw.ID, Priority: raw.Priority, Enabled: raw.Enabled,
		Match: config.MatchConditions{
			Tags: raw.Match.Tags, Country: raw.Match.Country, Region: raw.Match.Region,
			IPType: raw.Match.IPType, IngressType: raw.Match.IngressType, TargetHost: raw.Match.TargetHost,
		},
		TargetPoolID: raw.TargetPoolID, StickySessionTTLSeconds: raw.StickySessionTTLSeconds,
		AllowStickyFallback: raw.AllowStickyFallback,
	}, nil
}

func decodeRollbackExecutorPool(newJSON []byte) (config.ExecutorPool, error) {
	var raw rollbackExecutorPoolJSON

	err := json.Unmarshal(newJSON, &raw)
	if err != nil {
		return config.ExecutorPool{}, fmt.Errorf("unmarshal rollback executor pool: %w", err)
	}

	return config.ExecutorPool{
		ID: raw.ID, ExecutorType: raw.ExecutorType, Tags: raw.Tags, Enabled: raw.Enabled,
		AllowDegradedWorkers: raw.AllowDegradedWorkers, AllowedIPTypes: raw.AllowedIPTypes,
		AllowedCountries: raw.AllowedCountries, AllowedRegions: raw.AllowedRegions,
	}, nil
}

func decodeRollbackDenyRule(newJSON []byte) (config.DenyRule, error) {
	var raw rollbackDenyRuleJSON

	err := json.Unmarshal(newJSON, &raw)
	if err != nil {
		return config.DenyRule{}, fmt.Errorf("unmarshal rollback deny rule: %w", err)
	}

	return config.DenyRule{
		ID: raw.ID, RuleType: raw.RuleType, Action: raw.Action, Enabled: raw.Enabled,
		Reason: raw.Reason, RawPattern: raw.RawPattern, NormalizedHost: raw.NormalizedHost,
		NormalizedCIDR: raw.NormalizedCIDR, NormalizedIP: raw.NormalizedIP, NormalizedName: raw.NormalizedName,
	}, nil
}

func decodeRollbackQuotaConfig(newJSON []byte) (QuotaConfig, error) {
	var raw rollbackQuotaConfigJSON

	err := json.Unmarshal(newJSON, &raw)
	if err != nil {
		return QuotaConfig{}, fmt.Errorf("unmarshal rollback quota config: %w", err)
	}

	return QuotaConfig{
		TenantID: raw.TenantID, Period: raw.Period, MaxRequests: raw.MaxRequests,
		MaxBandwidthBytes: raw.MaxBandwidthBytes, RequestCountPolicy: raw.RequestCountPolicy,
		RedisFailPolicy: raw.RedisFailPolicy, ConfigVersion: raw.ConfigVersion,
	}, nil
}

func decodeRollbackRateLimitConfig(newJSON []byte) (RateLimitConfig, error) {
	var raw rollbackRateLimitConfigJSON

	err := json.Unmarshal(newJSON, &raw)
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("unmarshal rollback rate-limit config: %w", err)
	}

	limits := make([]RateLimitRule, 0, len(raw.Limits))
	for _, rule := range raw.Limits {
		limits = append(limits, RateLimitRule(rule))
	}

	return RateLimitConfig{TenantID: raw.TenantID, Limits: limits, ConfigVersion: raw.ConfigVersion}, nil
}

func deleteRollbackResource(state *rollbackState, resourceType, resourceID string) {
	switch resourceType {
	case resourceTypeRoutingRule:
		delete(state.routes, resourceID)
	case resourceTypeExecutorPool:
		delete(state.pools, resourceID)
	case resourceTypeDenyRule:
		delete(state.denyRules, resourceID)
	case resourceTypeQuotaConfig:
		state.quota = nil
	case resourceTypeRateLimitConfig:
		state.rateLimits = nil
	}
}

func rejectRedactedRollbackSecrets(ctx context.Context, tx pgx.Tx, tenantID string, targetVersion, currentVersion uint64) error {
	targetParam, err := configVersionParam(targetVersion)
	if err != nil {
		return err
	}

	currentParam, err := configVersionParam(currentVersion)
	if err != nil {
		return err
	}

	var count int

	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM config_audit_source
		 WHERE tenant_id = $1 AND config_version IS NOT NULL
		   AND config_version > $2 AND config_version <= $3
		   AND resource_type = $4`,
		tenantID, targetParam, currentParam, resourceTypeInjectionPolicy,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check rollback redacted secrets: %w", err)
	}

	if count > 0 {
		return ErrConfigRollbackSecretRedacted
	}

	return nil
}

func applyRollbackState(ctx context.Context, tx pgx.Tx, tenantID string, newVersion uint64, state rollbackState, req ConfigRollbackRequest, actor ConfigActor) error {
	err := replaceRollbackRoutes(ctx, tx, tenantID, newVersion, state.routes, actor)
	if err != nil {
		return err
	}

	err = replaceRollbackPools(ctx, tx, tenantID, newVersion, state.pools, actor)
	if err != nil {
		return err
	}

	err = replaceRollbackDenyRules(ctx, tx, tenantID, newVersion, state.denyRules, actor)
	if err != nil {
		return err
	}

	err = replaceRollbackQuota(ctx, tx, tenantID, newVersion, state.quota, actor)
	if err != nil {
		return err
	}

	err = replaceRollbackRateLimits(ctx, tx, tenantID, newVersion, state.rateLimits, actor)
	if err != nil {
		return err
	}

	return insertConfigAudit(ctx, tx, auditEntry{
		tenantID: tenantID, actor: actor, resourceType: resourceTypeConfigRollback,
		resourceID: strconv.FormatUint(req.TargetConfigVersion, 10), action: configActionRollback,
		configVersion: newVersion, newValue: req,
	})
}

func replaceRollbackRoutes(ctx context.Context, tx pgx.Tx, tenantID string, version uint64, routes map[string]config.RoutingRule, actor ConfigActor) error {
	ids := make([]string, 0, len(routes))
	for id, rule := range routes {
		ids = append(ids, id)

		matchJSON, err := json.Marshal(matchToJSON(rule.Match))
		if err != nil {
			return fmt.Errorf("marshal rollback match: %w", err)
		}

		versionParam, err := configVersionParam(version)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO routing_rules
			  (tenant_id, id, priority, enabled, match_conditions_jsonb, target_pool_id,
			   sticky_session_ttl_seconds, allow_sticky_fallback, created_at, updated_at, config_version, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now(), $9, NULL)
			 ON CONFLICT (tenant_id, id) DO UPDATE SET
			   priority = EXCLUDED.priority, enabled = EXCLUDED.enabled,
			   match_conditions_jsonb = EXCLUDED.match_conditions_jsonb,
			   target_pool_id = EXCLUDED.target_pool_id,
			   sticky_session_ttl_seconds = EXCLUDED.sticky_session_ttl_seconds,
			   allow_sticky_fallback = EXCLUDED.allow_sticky_fallback,
			   updated_at = now(), config_version = EXCLUDED.config_version, deleted_at = NULL`,
			tenantID, id, rule.Priority, rule.Enabled, matchJSON, rule.TargetPoolID,
			int64(rule.StickySessionTTLSeconds), rule.AllowStickyFallback, versionParam,
		)
		if err != nil {
			return fmt.Errorf("rollback routing rule: %w", err)
		}

		err = insertRollbackResourceAudit(ctx, tx, tenantID, actor, resourceTypeRoutingRule, id, version, rule)
		if err != nil {
			return err
		}
	}

	return softDeleteMissingRollbackResources(ctx, tx, tenantID, resourceTypeRoutingRule, ids, version, actor)
}

func replaceRollbackPools(ctx context.Context, tx pgx.Tx, tenantID string, version uint64, pools map[string]config.ExecutorPool, actor ConfigActor) error {
	ids := make([]string, 0, len(pools))
	for id, pool := range pools {
		ids = append(ids, id)

		tagsJSON, ipTypesJSON, countriesJSON, regionsJSON, err := marshalPoolCapabilityFields(pool)
		if err != nil {
			return err
		}

		versionParam, err := configVersionParam(version)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO executor_pools
			  (tenant_id, id, executor_type, tags_jsonb, enabled, allow_degraded_workers,
			   allowed_ip_types_jsonb, allowed_countries_jsonb, allowed_regions_jsonb,
			   created_at, updated_at, config_version, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now(), $10, NULL)
			 ON CONFLICT (tenant_id, id) DO UPDATE SET
			   executor_type = EXCLUDED.executor_type, tags_jsonb = EXCLUDED.tags_jsonb,
			   enabled = EXCLUDED.enabled, allow_degraded_workers = EXCLUDED.allow_degraded_workers,
			   allowed_ip_types_jsonb = EXCLUDED.allowed_ip_types_jsonb,
			   allowed_countries_jsonb = EXCLUDED.allowed_countries_jsonb,
			   allowed_regions_jsonb = EXCLUDED.allowed_regions_jsonb,
			   updated_at = now(), config_version = EXCLUDED.config_version, deleted_at = NULL`,
			tenantID, id, defaultExecutorType(pool.ExecutorType), tagsJSON, pool.Enabled, pool.AllowDegradedWorkers,
			ipTypesJSON, countriesJSON, regionsJSON, versionParam,
		)
		if err != nil {
			return fmt.Errorf("rollback executor pool: %w", err)
		}

		err = insertRollbackResourceAudit(ctx, tx, tenantID, actor, resourceTypeExecutorPool, id, version, pool)
		if err != nil {
			return err
		}
	}

	return softDeleteMissingRollbackResources(ctx, tx, tenantID, resourceTypeExecutorPool, ids, version, actor)
}

func replaceRollbackDenyRules(ctx context.Context, tx pgx.Tx, tenantID string, version uint64, rules map[string]config.DenyRule, actor ConfigActor) error {
	ids := make([]string, 0, len(rules))
	for id, rule := range rules {
		ids = append(ids, id)

		versionParam, err := configVersionParam(version)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO deny_rules
			  (tenant_id, id, rule_type, action, enabled, raw_pattern,
			   normalized_host, normalized_cidr, normalized_ip, normalized_cname, reason,
			   created_at, updated_at, config_version, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $6,
			         nullif($7, ''), nullif($8, '')::cidr, nullif($9, '')::inet, nullif($10, ''), nullif($11, ''),
			         now(), now(), $12, NULL)
			 ON CONFLICT (tenant_id, id) DO UPDATE SET
			   rule_type = EXCLUDED.rule_type, action = EXCLUDED.action,
			   enabled = EXCLUDED.enabled, raw_pattern = EXCLUDED.raw_pattern,
			   normalized_host = EXCLUDED.normalized_host, normalized_cidr = EXCLUDED.normalized_cidr,
			   normalized_ip = EXCLUDED.normalized_ip, normalized_cname = EXCLUDED.normalized_cname,
			   reason = EXCLUDED.reason, updated_at = now(), config_version = EXCLUDED.config_version, deleted_at = NULL`,
			tenantID, id, rule.RuleType, rule.Action, rule.Enabled, rule.RawPattern,
			rule.NormalizedHost, rule.NormalizedCIDR, rule.NormalizedIP, rule.NormalizedName, rule.Reason, versionParam,
		)
		if err != nil {
			return fmt.Errorf("rollback deny rule: %w", err)
		}

		err = insertRollbackResourceAudit(ctx, tx, tenantID, actor, resourceTypeDenyRule, id, version, rule)
		if err != nil {
			return err
		}
	}

	return softDeleteMissingRollbackResources(ctx, tx, tenantID, resourceTypeDenyRule, ids, version, actor)
}

func replaceRollbackQuota(ctx context.Context, tx pgx.Tx, tenantID string, version uint64, quota *QuotaConfig, actor ConfigActor) error {
	versionParam, err := configVersionParam(version)
	if err != nil {
		return err
	}

	if quota == nil {
		_, err = tx.Exec(ctx, `DELETE FROM quota_configs WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return fmt.Errorf("rollback quota delete: %w", err)
		}

		return nil
	}

	quota.TenantID = tenantID
	quota.ConfigVersion = version

	_, err = tx.Exec(ctx,
		`INSERT INTO quota_configs
		  (tenant_id, quota_period, enabled, request_count_limit, bandwidth_bytes_limit,
		   count_on_admission, fail_policy, created_at, updated_at, config_version)
		 VALUES ($1, $2, true, $3, $4, $5, $6, now(), now(), $7)
		 ON CONFLICT (tenant_id, quota_period) DO UPDATE SET
		   enabled = true, request_count_limit = EXCLUDED.request_count_limit,
		   bandwidth_bytes_limit = EXCLUDED.bandwidth_bytes_limit,
		   count_on_admission = EXCLUDED.count_on_admission, fail_policy = EXCLUDED.fail_policy,
		   updated_at = now(), config_version = EXCLUDED.config_version`,
		tenantID, quotaPeriod(quota.Period), quota.MaxRequests, quota.MaxBandwidthBytes,
		requestCountPolicyToBool(quota.RequestCountPolicy), quotaFailPolicy(quota.RedisFailPolicy), versionParam,
	)
	if err != nil {
		return fmt.Errorf("rollback quota config: %w", err)
	}

	return insertRollbackResourceAudit(ctx, tx, tenantID, actor, resourceTypeQuotaConfig, tenantID, version, *quota)
}

func replaceRollbackRateLimits(ctx context.Context, tx pgx.Tx, tenantID string, version uint64, cfg *RateLimitConfig, actor ConfigActor) error {
	_, err := tx.Exec(ctx, `DELETE FROM rate_limit_configs WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return fmt.Errorf("rollback rate limit clear: %w", err)
	}

	if cfg == nil {
		return nil
	}

	versionParam, err := configVersionParam(version)
	if err != nil {
		return err
	}

	cfg.TenantID = tenantID
	cfg.ConfigVersion = version

	for _, rule := range cfg.Limits {
		_, err = tx.Exec(ctx,
			`INSERT INTO rate_limit_configs
			  (tenant_id, dimension, key, enabled, window_ms, fail_policy, limit_count,
			   created_at, updated_at, config_version)
			 VALUES ($1, $2, $3, true, $4, $5, $6, now(), now(), $7)`,
			tenantID, string(rule.Dimension), rule.Key, int64(rule.WindowSeconds)*millisPerSecond,
			rateLimitFailPolicyToDB(rule.FailPolicy), int64(rule.MaxRequests), versionParam,
		)
		if err != nil {
			return fmt.Errorf("rollback rate limit insert: %w", err)
		}
	}

	return insertRollbackResourceAudit(ctx, tx, tenantID, actor, resourceTypeRateLimitConfig, tenantID, version, *cfg)
}

func insertRollbackResourceAudit(ctx context.Context, tx pgx.Tx, tenantID string, actor ConfigActor, resourceType, resourceID string, version uint64, newValue any) error {
	return insertConfigAudit(ctx, tx, auditEntry{
		tenantID: tenantID, actor: actor, resourceType: resourceType,
		resourceID: resourceID, action: configActionUpsert, configVersion: version, newValue: newValue,
	})
}

func softDeleteMissingRollbackResources(ctx context.Context, tx pgx.Tx, tenantID, resourceType string, keepIDs []string, version uint64, actor ConfigActor) error {
	query, ok := softDeleteRollbackQueries[resourceType]
	if !ok {
		return nil
	}

	versionParam, err := configVersionParam(version)
	if err != nil {
		return err
	}

	rows, err := tx.Query(ctx, query, tenantID, keepIDs, versionParam)
	if err != nil {
		return fmt.Errorf("rollback soft delete %s: %w", resourceType, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string

		err := rows.Scan(&id)
		if err != nil {
			return fmt.Errorf("scan rollback deleted %s: %w", resourceType, err)
		}

		err = insertConfigAudit(ctx, tx, auditEntry{
			tenantID: tenantID, actor: actor, resourceType: resourceType,
			resourceID: id, action: configActionDelete, configVersion: version,
		})
		if err != nil {
			return err
		}
	}

	return checkRows(rows, "rollback deleted resources")
}

var softDeleteRollbackQueries = map[string]string{
	resourceTypeRoutingRule: `UPDATE routing_rules
		SET deleted_at = now(), updated_at = now(), config_version = $3
		WHERE tenant_id = $1 AND deleted_at IS NULL AND NOT (id = ANY($2::text[]))
		RETURNING id`,
	resourceTypeExecutorPool: `UPDATE executor_pools
		SET deleted_at = now(), updated_at = now(), config_version = $3
		WHERE tenant_id = $1 AND deleted_at IS NULL AND NOT (id = ANY($2::text[]))
		RETURNING id`,
	resourceTypeDenyRule: `UPDATE deny_rules
		SET deleted_at = now(), updated_at = now(), config_version = $3
		WHERE tenant_id = $1 AND deleted_at IS NULL AND NOT (id = ANY($2::text[]))
		RETURNING id`,
}
