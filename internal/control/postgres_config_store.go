package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/beremaran/straw/v2/internal/config"
)

// maxInjectionOperations bounds an injection policy's operation count, matching
// the injection_policies CHECK constraint (migrations/postgres/0001_init.sql).
const (
	maxInjectionOperations  = 32
	configActorTypeAPIKey   = "api_key"
	configActionDelete      = "delete"
	configActionUpdate      = "update"
	configActionUpsert      = "upsert"
	quotaPeriodMonthly      = "monthly"
	postgresFailClosed      = "fail_closed"
	postgresFailOpen        = "fail_open"
	resourceTypeRoutingRule = "routing_rule"
)

// ErrConfigResourceNotFound is returned when a soft delete targets a resource
// that does not exist (or is already deleted) for the tenant.
var (
	ErrConfigResourceNotFound    = errors.New("config resource not found")
	errConfigVersionOverflow     = errors.New("config version overflows int64")
	errNegativeConfigVersion     = errors.New("negative config version")
	errInjectionPolicyTooLarge   = errors.New("injection policy operation count exceeds maximum")
	errUnknownConfigResource     = errors.New("unknown config resource type")
	errUnsigned32ValueOutOfRange = errors.New("value out of uint32 range")
)

// ErrConfigResourceVersionConflict is returned when a routing rule, deny rule,
// or injection policy write's expected_config_version does not match the
// resource's current per-row config_version (docs/planning/26 "Shared Config
// API Contract").
var ErrConfigResourceVersionConflict = errors.New("config resource version conflict")

// RoutingRuleRecord is a routing rule read from Postgres, carrying the
// per-resource fields the admin API surface needs on top of the config-layer
// config.RoutingRule (docs/tasks/p0/20).
type RoutingRuleRecord struct {
	config.RoutingRule
	TenantID      string
	CreatedAt     time.Time
	ConfigVersion uint64
}

// DenyRuleRecord is a deny rule read from Postgres with admin-API fields.
type DenyRuleRecord struct {
	config.DenyRule
	TenantID      string
	CreatedAt     time.Time
	ConfigVersion uint64
}

// InjectionPolicyRecord is an injection policy read from Postgres with
// admin-API fields. Operations carry real (non-redacted) values, matching
// snapshot assembly; handlers must not echo these back over the wire for
// sensitive operations without the same role check applied on write.
type InjectionPolicyRecord struct {
	config.InjectionPolicy
	TenantID      string
	CreatedAt     time.Time
	ConfigVersion uint64
}

// FingerprintProfileRecord is a fingerprint profile visible to a tenant
// (global built-ins plus, if ever added, tenant-scoped rows).
type FingerprintProfileRecord struct {
	config.FingerprintProfile
	CreatedAt     time.Time
	ConfigVersion uint64
}

// ExecutorPoolRecord is an executor pool read from Postgres with admin-API
// fields (docs/tasks/p0/30).
type ExecutorPoolRecord struct {
	config.ExecutorPool
	TenantID      string
	CreatedAt     time.Time
	ConfigVersion uint64
}

// ConfigActor identifies the API-key actor behind a config write, recorded in
// config_audit_source (docs/planning/21). ActorID is the API key ID in P0.
type ConfigActor struct {
	ActorType string
	ActorID   string
	RequestID string
}

// PostgresConfigStore is the Postgres-backed durable config store. Its write
// methods run each resource change, the tenant config-version increment, and
// the redacted audit-source append in a single transaction
// (docs/planning/21-state-and-storage.md). It also implements SnapshotStore
// (see postgres_snapshot_store.go) so ConfigCache assembles immutable tenant
// snapshots straight from these tables instead of process-local config state.
type PostgresConfigStore struct {
	pool *pgxpool.Pool
}

// NewPostgresConfigStore builds a config store over the given pool.
func NewPostgresConfigStore(pool *pgxpool.Pool) *PostgresConfigStore {
	return &PostgresConfigStore{pool: pool}
}

func dbUint64(v int64, field string) (uint64, error) {
	if v < 0 {
		return 0, fmt.Errorf("%s: %w", field, errNegativeConfigVersion)
	}

	return uint64(v), nil
}

func dbUint32(v int64, field string) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, fmt.Errorf("%s: %w", field, errUnsigned32ValueOutOfRange)
	}

	return uint32(v), nil
}

func dbWindowSeconds(windowMS int64) (uint32, error) {
	return dbUint32(windowMS/millisPerSecond, "window seconds")
}

func configVersionParam(v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("config version %d: %w", v, errConfigVersionOverflow)
	}

	return int64(v), nil
}

func nextConfigVersionParam(expected uint64) (uint64, int64, error) {
	if expected >= math.MaxInt64 {
		return 0, 0, fmt.Errorf("config version %d: %w", expected, errConfigVersionOverflow)
	}

	next := expected + 1

	return next, int64(next), nil
}

// auditEntry is the redacted row appended to config_audit_source alongside a
// config write. NewValue/OldValue must already have secret fields redacted.
type auditEntry struct {
	tenantID     string // "" → NULL (platform-scoped)
	actor        ConfigActor
	resourceType string
	resourceID   string
	action       string
	newValue     any
	oldValue     any
}

// writeTenantConfig runs write inside a transaction that also increments the
// tenant's config version and appends the audit row, so a partial config write
// can never leave the version or audit trail inconsistent. It returns the new
// tenant config version.
func writeTenantConfig(ctx context.Context, pool *pgxpool.Pool, tenantID string, entry auditEntry, write func(context.Context, pgx.Tx) error) (uint64, error) {
	entry.tenantID = tenantID

	var version uint64

	err := inConfigTx(ctx, pool, func(tx pgx.Tx) error {
		err := write(ctx, tx)
		if err != nil {
			return err
		}

		v, err := bumpTenantConfigVersion(ctx, tx, tenantID)
		if err != nil {
			return err
		}

		version = v

		return insertConfigAudit(ctx, tx, entry)
	})
	if err != nil {
		return 0, err
	}

	return version, nil
}

// inConfigTx runs fn in a transaction, rolling back on any error.
func inConfigTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin config tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	err = fn(tx)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit config tx: %w", err)
	}

	return nil
}

// bumpTenantConfigVersion increments (or initializes) the tenant's monotonic
// snapshot version and returns the new value (docs/planning/25).
func bumpTenantConfigVersion(ctx context.Context, tx pgx.Tx, tenantID string) (uint64, error) {
	var v int64

	err := tx.QueryRow(
		ctx,
		`INSERT INTO tenant_config_versions (tenant_id, config_version, updated_at)
		 VALUES ($1, 1, now())
		 ON CONFLICT (tenant_id) DO UPDATE
		 SET config_version = tenant_config_versions.config_version + 1, updated_at = now()
		 RETURNING config_version`,
		tenantID,
	).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("bump tenant config version: %w", err)
	}

	version, err := dbUint64(v, "tenant config version")
	if err != nil {
		return 0, err
	}

	return version, nil
}

// insertConfigAudit appends a redacted config_audit_source row.
func insertConfigAudit(ctx context.Context, tx pgx.Tx, entry auditEntry) error {
	newJSON, err := marshalAuditValue(entry.newValue)
	if err != nil {
		return fmt.Errorf("marshal audit new value: %w", err)
	}

	oldJSON, err := marshalAuditValue(entry.oldValue)
	if err != nil {
		return fmt.Errorf("marshal audit old value: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO config_audit_source
		  (tenant_id, actor_type, actor_id, resource_type, resource_id, action,
		   request_id, old_value_json, new_value_json, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())`,
		nullString(entry.tenantID),
		entry.actor.ActorType,
		entry.actor.ActorID,
		entry.resourceType,
		entry.resourceID,
		entry.action,
		nullString(entry.actor.RequestID),
		oldJSON,
		newJSON,
	)
	if err != nil {
		return fmt.Errorf("insert config audit: %w", err)
	}

	return nil
}

// marshalAuditValue returns nil for a nil value so the audit column stays NULL,
// otherwise the JSON encoding.
func marshalAuditValue(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal audit value: %w", err)
	}

	return b, nil
}

// ---- Routing rules ----

// matchConditionsJSON is the stored/loaded shape of routing_rules.match_conditions_jsonb.
type matchConditionsJSON struct {
	Tags        []string `json:"tags,omitempty"`
	Country     string   `json:"country,omitempty"`
	Region      string   `json:"region,omitempty"`
	IPType      string   `json:"ip_type,omitempty"`
	IngressType string   `json:"ingress_type,omitempty"`
	TargetHost  string   `json:"target_host,omitempty"`
}

func matchToJSON(m config.MatchConditions) matchConditionsJSON {
	return matchConditionsJSON{
		Tags: m.Tags, Country: m.Country, Region: m.Region,
		IPType: m.IPType, IngressType: m.IngressType, TargetHost: m.TargetHost,
	}
}

func matchFromJSON(m matchConditionsJSON) config.MatchConditions {
	return config.MatchConditions{
		Tags: m.Tags, Country: m.Country, Region: m.Region,
		IPType: m.IPType, IngressType: m.IngressType, TargetHost: m.TargetHost,
	}
}

// UpsertRoutingRule inserts or updates a routing rule by its stable
// (tenant_id, id), clearing any prior soft delete (docs/planning/10, 26).
// expectedVersion is checked against the resource's own config_version
// (0 for a not-yet-existing or soft-deleted row) under optimistic
// concurrency; a mismatch returns ErrConfigResourceVersionConflict. It
// returns the saved record and the bumped tenant config version (for cache
// invalidation).
func (s *PostgresConfigStore) UpsertRoutingRule(ctx context.Context, tenantID string, rule config.RoutingRule, expectedVersion uint64, actor ConfigActor) (RoutingRuleRecord, uint64, error) {
	matchJSON, err := json.Marshal(matchToJSON(rule.Match))
	if err != nil {
		return RoutingRuleRecord{}, 0, fmt.Errorf("marshal match conditions: %w", err)
	}

	var record RoutingRuleRecord

	tenantVersion, err := writeTenantConfig(ctx, s.pool, tenantID, auditEntry{
		actor: actor, resourceType: resourceTypeRoutingRule, resourceID: rule.ID, action: configActionUpsert,
		newValue: rule,
	}, func(ctx context.Context, tx pgx.Tx) error {
		nextVersion, nextVersionParam, verErr := checkResourceVersion(ctx, tx,
			`SELECT config_version FROM routing_rules WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
			[]any{tenantID, rule.ID}, expectedVersion)
		if verErr != nil {
			return verErr
		}

		var createdAt time.Time

		execErr := tx.QueryRow(
			ctx,
			`INSERT INTO routing_rules
			  (tenant_id, id, priority, enabled, match_conditions_jsonb, target_pool_id,
			   sticky_session_ttl_seconds, allow_sticky_fallback, created_at, updated_at, config_version, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now(), $9, NULL)
			 ON CONFLICT (tenant_id, id) DO UPDATE SET
			   priority = EXCLUDED.priority,
			   enabled = EXCLUDED.enabled,
			   match_conditions_jsonb = EXCLUDED.match_conditions_jsonb,
			   target_pool_id = EXCLUDED.target_pool_id,
			   sticky_session_ttl_seconds = EXCLUDED.sticky_session_ttl_seconds,
			   allow_sticky_fallback = EXCLUDED.allow_sticky_fallback,
			   updated_at = now(),
			   config_version = $9,
			   deleted_at = NULL
			 RETURNING created_at`,
			tenantID, rule.ID, rule.Priority, rule.Enabled, matchJSON, rule.TargetPoolID,
			int64(rule.StickySessionTTLSeconds), rule.AllowStickyFallback, nextVersionParam,
		).Scan(&createdAt)
		if execErr != nil {
			return fmt.Errorf("upsert routing rule: %w", execErr)
		}

		record = RoutingRuleRecord{RoutingRule: rule, TenantID: tenantID, CreatedAt: createdAt, ConfigVersion: nextVersion}

		return nil
	})
	if err != nil {
		return RoutingRuleRecord{}, 0, err
	}

	return record, tenantVersion, nil
}

// checkResourceVersion reads a resource's current config_version (0 when
// missing or soft-deleted, matching what a GET/List would show) and confirms
// it equals expectedVersion, returning ErrConfigResourceVersionConflict
// otherwise. On success it returns the next version both as a uint64 and as
// the int64 SQL parameter to write.
func checkResourceVersion(ctx context.Context, tx pgx.Tx, currentVersionQuery string, args []any, expectedVersion uint64) (uint64, int64, error) {
	current, err := currentResourceVersion(ctx, tx, currentVersionQuery, args...)
	if err != nil {
		return 0, 0, err
	}

	if current != expectedVersion {
		return 0, 0, ErrConfigResourceVersionConflict
	}

	nextVersion, nextVersionParam, err := nextConfigVersionParam(expectedVersion)
	if err != nil {
		return 0, 0, err
	}

	return nextVersion, nextVersionParam, nil
}

// DeleteRoutingRule soft-deletes a routing rule so it drops out of assembled
// snapshots while retaining version history.
func (s *PostgresConfigStore) DeleteRoutingRule(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error) {
	return s.softDelete(ctx, tenantID, resourceTypeRoutingRule, id, actor)
}

// ---- Executor pools ----

// UpsertExecutorPool inserts or updates a tenant-visible executor pool by its
// stable (tenant_id, id), clearing any prior soft delete. See
// UpsertRoutingRule for expectedVersion/return semantics.
func (s *PostgresConfigStore) UpsertExecutorPool(ctx context.Context, tenantID string, pool config.ExecutorPool, expectedVersion uint64, actor ConfigActor) (ExecutorPoolRecord, uint64, error) {
	tagsJSON, ipTypesJSON, countriesJSON, regionsJSON, err := marshalPoolCapabilityFields(pool)
	if err != nil {
		return ExecutorPoolRecord{}, 0, err
	}

	var record ExecutorPoolRecord

	tenantVersion, err := writeTenantConfig(ctx, s.pool, tenantID, auditEntry{
		actor: actor, resourceType: "executor_pool", resourceID: pool.ID, action: configActionUpsert,
		newValue: pool,
	}, func(ctx context.Context, tx pgx.Tx) error {
		nextVersion, nextVersionParam, verErr := checkResourceVersion(ctx, tx,
			`SELECT config_version FROM executor_pools WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
			[]any{tenantID, pool.ID}, expectedVersion)
		if verErr != nil {
			return verErr
		}

		var createdAt time.Time

		execErr := tx.QueryRow(
			ctx,
			`INSERT INTO executor_pools
			  (tenant_id, id, executor_type, tags_jsonb, enabled, allow_degraded_workers,
			   allowed_ip_types_jsonb, allowed_countries_jsonb, allowed_regions_jsonb,
			   created_at, updated_at, config_version, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now(), $10, NULL)
			 ON CONFLICT (tenant_id, id) DO UPDATE SET
			   executor_type = EXCLUDED.executor_type,
			   tags_jsonb = EXCLUDED.tags_jsonb,
			   enabled = EXCLUDED.enabled,
			   allow_degraded_workers = EXCLUDED.allow_degraded_workers,
			   allowed_ip_types_jsonb = EXCLUDED.allowed_ip_types_jsonb,
			   allowed_countries_jsonb = EXCLUDED.allowed_countries_jsonb,
			   allowed_regions_jsonb = EXCLUDED.allowed_regions_jsonb,
			   updated_at = now(),
			   config_version = $10,
			   deleted_at = NULL
			 RETURNING created_at`,
			tenantID, pool.ID, defaultExecutorType(pool.ExecutorType), tagsJSON, pool.Enabled,
			pool.AllowDegradedWorkers, ipTypesJSON, countriesJSON, regionsJSON, nextVersionParam,
		).Scan(&createdAt)
		if execErr != nil {
			return fmt.Errorf("upsert executor pool: %w", execErr)
		}

		record = ExecutorPoolRecord{ExecutorPool: pool, TenantID: tenantID, CreatedAt: createdAt, ConfigVersion: nextVersion}

		return nil
	})
	if err != nil {
		return ExecutorPoolRecord{}, 0, err
	}

	return record, tenantVersion, nil
}

// marshalPoolCapabilityFields marshals an executor pool's tag/capability-
// restriction list fields to jsonb for UpsertExecutorPool.
func marshalPoolCapabilityFields(pool config.ExecutorPool) ([]byte, []byte, []byte, []byte, error) {
	tagsJSON, err := json.Marshal(nonNilStrings(pool.Tags))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal pool tags: %w", err)
	}

	ipTypesJSON, err := json.Marshal(nonNilStrings(pool.AllowedIPTypes))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal pool allowed ip types: %w", err)
	}

	countriesJSON, err := json.Marshal(nonNilStrings(pool.AllowedCountries))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal pool allowed countries: %w", err)
	}

	regionsJSON, err := json.Marshal(nonNilStrings(pool.AllowedRegions))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("marshal pool allowed regions: %w", err)
	}

	return tagsJSON, ipTypesJSON, countriesJSON, regionsJSON, nil
}

// DeleteExecutorPool soft-deletes an executor pool.
func (s *PostgresConfigStore) DeleteExecutorPool(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error) {
	return s.softDelete(ctx, tenantID, "executor_pool", id, actor)
}

// ---- Deny rules ----

// UpsertDenyRule inserts or updates a host/CIDR/CNAME/IP deny or allow rule.
// See UpsertRoutingRule for expectedVersion/return semantics.
func (s *PostgresConfigStore) UpsertDenyRule(ctx context.Context, tenantID string, rule config.DenyRule, expectedVersion uint64, actor ConfigActor) (DenyRuleRecord, uint64, error) {
	var record DenyRuleRecord

	tenantVersion, err := writeTenantConfig(ctx, s.pool, tenantID, auditEntry{
		actor: actor, resourceType: "deny_rule", resourceID: rule.ID, action: configActionUpsert,
		newValue: rule,
	}, func(ctx context.Context, tx pgx.Tx) error {
		nextVersion, nextVersionParam, verErr := checkResourceVersion(ctx, tx,
			`SELECT config_version FROM deny_rules WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
			[]any{tenantID, rule.ID}, expectedVersion)
		if verErr != nil {
			return verErr
		}

		var createdAt time.Time

		execErr := tx.QueryRow(
			ctx,
			`INSERT INTO deny_rules
			  (tenant_id, id, rule_type, action, enabled, raw_pattern,
			   normalized_host, normalized_cidr, normalized_ip, normalized_cname, reason,
			   created_at, updated_at, config_version, deleted_at)
			 VALUES ($1, $2, $3, $4, $5, $6,
			         nullif($7, ''), nullif($8, '')::cidr, nullif($9, '')::inet, nullif($10, ''), nullif($11, ''),
			         now(), now(), $12, NULL)
			 ON CONFLICT (tenant_id, id) DO UPDATE SET
			   rule_type = EXCLUDED.rule_type,
			   action = EXCLUDED.action,
			   enabled = EXCLUDED.enabled,
			   raw_pattern = EXCLUDED.raw_pattern,
			   normalized_host = EXCLUDED.normalized_host,
			   normalized_cidr = EXCLUDED.normalized_cidr,
			   normalized_ip = EXCLUDED.normalized_ip,
			   normalized_cname = EXCLUDED.normalized_cname,
			   reason = EXCLUDED.reason,
			   updated_at = now(),
			   config_version = $12,
			   deleted_at = NULL
			 RETURNING created_at`,
			tenantID, rule.ID, rule.RuleType, rule.Action, rule.Enabled, rule.RawPattern,
			rule.NormalizedHost, rule.NormalizedCIDR, rule.NormalizedIP, rule.NormalizedName, rule.Reason, nextVersionParam,
		).Scan(&createdAt)
		if execErr != nil {
			return fmt.Errorf("upsert deny rule: %w", execErr)
		}

		record = DenyRuleRecord{DenyRule: rule, TenantID: tenantID, CreatedAt: createdAt, ConfigVersion: nextVersion}

		return nil
	})
	if err != nil {
		return DenyRuleRecord{}, 0, err
	}

	return record, tenantVersion, nil
}

// DeleteDenyRule soft-deletes a deny rule.
func (s *PostgresConfigStore) DeleteDenyRule(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error) {
	return s.softDelete(ctx, tenantID, "deny_rule", id, actor)
}

// ---- Injection policies ----

// injectionOperationJSON is the stored/loaded shape of one operation in
// injection_policies.operations.
type injectionOperationJSON struct {
	Op          string `json:"op"`
	HeaderName  string `json:"header_name"`
	ValueBase64 string `json:"value_base64,omitempty"`
}

// UpsertInjectionPolicy inserts or updates an ordered header-injection policy.
// The audit record redacts each operation's value_base64 (secret classification,
// docs/planning/21 Config Secret Classification). See UpsertRoutingRule for
// expectedVersion/return semantics.
func (s *PostgresConfigStore) UpsertInjectionPolicy(ctx context.Context, tenantID string, pol config.InjectionPolicy, expectedVersion uint64, actor ConfigActor) (InjectionPolicyRecord, uint64, error) {
	if len(pol.Operations) > maxInjectionOperations {
		return InjectionPolicyRecord{}, 0, fmt.Errorf("%w: %s has %d operations, max %d", errInjectionPolicyTooLarge, pol.ID, len(pol.Operations), maxInjectionOperations)
	}

	opsJSON, err := json.Marshal(injectionOpsToJSON(pol.Operations))
	if err != nil {
		return InjectionPolicyRecord{}, 0, fmt.Errorf("marshal injection operations: %w", err)
	}

	var record InjectionPolicyRecord

	tenantVersion, err := writeTenantConfig(ctx, s.pool, tenantID, auditEntry{
		actor: actor, resourceType: "injection_policy", resourceID: pol.ID, action: configActionUpsert,
		newValue: redactInjectionPolicy(pol),
	}, func(ctx context.Context, tx pgx.Tx) error {
		nextVersion, nextVersionParam, verErr := checkResourceVersion(ctx, tx,
			`SELECT config_version FROM injection_policies WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
			[]any{tenantID, pol.ID}, expectedVersion)
		if verErr != nil {
			return verErr
		}

		var createdAt time.Time

		execErr := tx.QueryRow(
			ctx,
			`INSERT INTO injection_policies
			  (tenant_id, id, enabled, operations, audit_redacted, created_at, updated_at, config_version, deleted_at)
			 VALUES ($1, $2, $3, $4, true, now(), now(), $5, NULL)
			 ON CONFLICT (tenant_id, id) DO UPDATE SET
			   enabled = EXCLUDED.enabled,
			   operations = EXCLUDED.operations,
			   updated_at = now(),
			   config_version = $5,
			   deleted_at = NULL
			 RETURNING created_at`,
			tenantID, pol.ID, pol.Enabled, opsJSON, nextVersionParam,
		).Scan(&createdAt)
		if execErr != nil {
			return fmt.Errorf("upsert injection policy: %w", execErr)
		}

		record = InjectionPolicyRecord{InjectionPolicy: pol, TenantID: tenantID, CreatedAt: createdAt, ConfigVersion: nextVersion}

		return nil
	})
	if err != nil {
		return InjectionPolicyRecord{}, 0, err
	}

	return record, tenantVersion, nil
}

// DeleteInjectionPolicy soft-deletes an injection policy.
func (s *PostgresConfigStore) DeleteInjectionPolicy(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error) {
	return s.softDelete(ctx, tenantID, "injection_policy", id, actor)
}

// ---- Worker admin state (durable disable; drain stays runtime per
// docs/planning/21 "Durable global worker disable state") ----
//
// These are pure durable upserts. The worker-admin HTTP handlers own the
// audit-source record and (for the tenant path) the tenant config-version bump,
// matching the two-step flow the existing quota/rate-limit/API-key handlers use.

// SetTenantWorkerOverride persists a tenant's durable worker routing override
// (disable only).
func (s *PostgresConfigStore) SetTenantWorkerOverride(ctx context.Context, tenantID, workerID string, disabled bool, reason string) error {
	return setTenantWorkerOverride(ctx, s.pool, tenantID, workerID, disabled, reason)
}

// SetTenantWorkerOverrideConfig persists a tenant worker override, bumps the
// tenant snapshot version, and records config audit in one transaction.
func (s *PostgresConfigStore) SetTenantWorkerOverrideConfig(ctx context.Context, tenantID, workerID string, disabled bool, reason string, actor ConfigActor) error {
	_, err := writeTenantConfig(ctx, s.pool, tenantID, auditEntry{
		actor: actor, resourceType: "tenant_worker_admin_state", resourceID: workerID, action: configActionUpdate,
		newValue: config.TenantWorkerOverride{WorkerID: workerID, Disabled: disabled, DisabledReason: reason},
	}, func(ctx context.Context, tx pgx.Tx) error {
		return setTenantWorkerOverride(ctx, tx, tenantID, workerID, disabled, reason)
	})

	return err
}

func setTenantWorkerOverride(ctx context.Context, exec pgxExecutor, tenantID, workerID string, disabled bool, reason string) error {
	_, err := exec.Exec(
		ctx,
		`INSERT INTO tenant_worker_admin_state
		  (tenant_id, worker_id, disabled, disabled_reason, created_at, updated_at, config_version)
		 VALUES ($1, $2, $3, nullif($4, ''), now(), now(), 1)
		 ON CONFLICT (tenant_id, worker_id) DO UPDATE SET
		   disabled = EXCLUDED.disabled,
		   disabled_reason = EXCLUDED.disabled_reason,
		   updated_at = now(),
		   config_version = tenant_worker_admin_state.config_version + 1`,
		tenantID, workerID, disabled, reason,
	)
	if err != nil {
		return fmt.Errorf("set tenant worker override: %w", err)
	}

	return nil
}

// SetGlobalWorkerAdmin persists a worker's durable global disable state. Global
// worker admin state is platform-scoped, so it is not tied to any tenant
// config version.
func (s *PostgresConfigStore) SetGlobalWorkerAdmin(ctx context.Context, workerID string, disabled bool, reason string) error {
	return setGlobalWorkerAdmin(ctx, s.pool, workerID, disabled, reason)
}

// SetGlobalWorkerAdminConfig persists platform-scoped worker admin state and
// records config audit in one transaction. It does not bump a tenant version
// because the row is global.
func (s *PostgresConfigStore) SetGlobalWorkerAdminConfig(ctx context.Context, workerID string, disabled bool, reason string, actor ConfigActor) error {
	return inConfigTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := setGlobalWorkerAdmin(ctx, tx, workerID, disabled, reason)
		if err != nil {
			return err
		}

		return insertConfigAudit(ctx, tx, auditEntry{
			actor: actor, resourceType: "worker_admin_state", resourceID: workerID, action: configActionUpdate,
			newValue: config.WorkerAdminState{WorkerID: workerID, Disabled: disabled, DisabledReason: reason},
		})
	})
}

func setGlobalWorkerAdmin(ctx context.Context, exec pgxExecutor, workerID string, disabled bool, reason string) error {
	_, err := exec.Exec(
		ctx,
		`INSERT INTO worker_admin_state
		  (worker_id, disabled, disabled_reason, created_at, updated_at, config_version)
		 VALUES ($1, $2, nullif($3, ''), now(), now(), 1)
		 ON CONFLICT (worker_id) DO UPDATE SET
		   disabled = EXCLUDED.disabled,
		   disabled_reason = EXCLUDED.disabled_reason,
		   updated_at = now(),
		   config_version = worker_admin_state.config_version + 1`,
		workerID, disabled, reason,
	)
	if err != nil {
		return fmt.Errorf("set global worker admin: %w", err)
	}

	return nil
}

type pgxExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// ListWorkerAdminStates returns all persisted worker disable states so Control
// can rehydrate durable admin decisions into its runtime registry at startup.
func (s *PostgresConfigStore) ListWorkerAdminStates(ctx context.Context) ([]config.WorkerAdminState, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT worker_id, disabled, COALESCE(disabled_reason, '') FROM worker_admin_state ORDER BY worker_id`)
	if err != nil {
		return nil, fmt.Errorf("list worker admin states: %w", err)
	}

	defer rows.Close()

	var out []config.WorkerAdminState

	for rows.Next() {
		var state config.WorkerAdminState

		scanErr := rows.Scan(&state.WorkerID, &state.Disabled, &state.DisabledReason)
		if scanErr != nil {
			return nil, fmt.Errorf("scan worker admin state: %w", scanErr)
		}

		out = append(out, state)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("worker admin state rows: %w", err)
	}

	return out, nil
}

// ListTenantWorkerOverrides returns all persisted tenant worker overrides so
// Control can rehydrate them into its runtime registry at startup.
func (s *PostgresConfigStore) ListTenantWorkerOverrides(ctx context.Context) ([]TenantWorkerOverrideRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT tenant_id, worker_id, disabled FROM tenant_worker_admin_state ORDER BY tenant_id, worker_id`)
	if err != nil {
		return nil, fmt.Errorf("list tenant worker overrides: %w", err)
	}

	defer rows.Close()

	var out []TenantWorkerOverrideRow

	for rows.Next() {
		var row TenantWorkerOverrideRow

		scanErr := rows.Scan(&row.TenantID, &row.WorkerID, &row.Disabled)
		if scanErr != nil {
			return nil, fmt.Errorf("scan tenant worker override: %w", scanErr)
		}

		out = append(out, row)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("tenant worker override rows: %w", err)
	}

	return out, nil
}

// TenantWorkerOverrideRow is one durable tenant worker override, carrying the
// tenant scope the config-layer TenantWorkerOverride omits.
type TenantWorkerOverrideRow struct {
	TenantID string
	WorkerID string
	Disabled bool
}

// WorkerAdminStore persists durable worker disable state (docs/planning/21). It
// is optional on AdminHandlers: nil keeps the pre-existing runtime-only
// (single-Control) behavior used by unit tests, while the running binary wires
// the Postgres implementation so disable decisions survive restarts and appear
// in tenant snapshots.
type WorkerAdminStore interface {
	SetGlobalWorkerAdmin(ctx context.Context, workerID string, disabled bool, reason string) error
	SetTenantWorkerOverride(ctx context.Context, tenantID, workerID string, disabled bool, reason string) error
}

// ---- shared helpers ----

// softDeleteQueries maps a resource type to its fixed soft-delete statement.
// Using constant statements (rather than concatenating a table name into SQL)
// keeps the query free of any dynamic-SQL construction.
var softDeleteQueries = map[string]string{
	resourceTypeRoutingRule: `UPDATE routing_rules SET deleted_at = now(), updated_at = now(), config_version = config_version + 1
	                 WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
	"executor_pool": `UPDATE executor_pools SET deleted_at = now(), updated_at = now(), config_version = config_version + 1
	                  WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
	"deny_rule": `UPDATE deny_rules SET deleted_at = now(), updated_at = now(), config_version = config_version + 1
	              WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
	"injection_policy": `UPDATE injection_policies SET deleted_at = now(), updated_at = now(), config_version = config_version + 1
	                     WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`,
}

// softDelete marks a resource deleted for the tenant, bumping the tenant config
// version and recording an audit row. Returns ErrConfigResourceNotFound if no
// live resource matched.
func (s *PostgresConfigStore) softDelete(ctx context.Context, tenantID, resourceType, id string, actor ConfigActor) (uint64, error) {
	query, ok := softDeleteQueries[resourceType]
	if !ok {
		return 0, fmt.Errorf("%w: %s", errUnknownConfigResource, resourceType)
	}

	return writeTenantConfig(ctx, s.pool, tenantID, auditEntry{
		actor: actor, resourceType: resourceType, resourceID: id, action: configActionDelete,
	}, func(ctx context.Context, tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, query, tenantID, id)
		if execErr != nil {
			return fmt.Errorf("soft delete %s: %w", resourceType, execErr)
		}

		if tag.RowsAffected() == 0 {
			return ErrConfigResourceNotFound
		}

		return nil
	})
}

func injectionOpsToJSON(ops []config.InjectionOperation) []injectionOperationJSON {
	out := make([]injectionOperationJSON, 0, len(ops))
	for _, op := range ops {
		out = append(out, injectionOperationJSON{Op: op.Op, HeaderName: op.HeaderName, ValueBase64: op.ValueBase64})
	}

	return out
}

// redactInjectionPolicy replaces each operation's secret value_base64 with a
// redaction marker for audit storage (docs/planning/21).
func redactInjectionPolicy(pol config.InjectionPolicy) config.InjectionPolicy {
	out := pol
	out.Operations = make([]config.InjectionOperation, len(pol.Operations))

	for i, op := range pol.Operations {
		if op.ValueBase64 != "" {
			op.ValueBase64 = "[redacted]"
		}

		out.Operations[i] = op
	}

	return out
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}

	return in
}

func defaultExecutorType(t string) string {
	if t == "" {
		return "egress"
	}

	return t
}
