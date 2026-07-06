package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This file holds the Postgres-backed durable stores for the two
// admission-config resources that already have P0 admin handlers: quota configs
// (platform-managed) and rate-limit configs (tenant-managed, ceiling-bounded).
// They implement the existing QuotaStore / RateLimitConfigStore interfaces so
// the admin handlers persist to Postgres instead of process memory
// (docs/planning/20, docs/planning/21). Each resource keeps its own
// config_version for optimistic concurrency. PostgresConfigStore wraps those
// same row writes with tenant-version and audit updates for runtime Control.

// ---- Quota configs ----

// postgresQuotaStore implements QuotaStore over quota_configs.
type postgresQuotaStore struct {
	pool *pgxpool.Pool
}

// ---- Payload capture policies ----

type postgresPayloadCapturePolicyStore struct {
	pool *pgxpool.Pool
}

// NewPostgresPayloadCapturePolicyStore builds a store over payload_capture_policies.
func NewPostgresPayloadCapturePolicyStore(pool *pgxpool.Pool) PayloadCapturePolicyStore {
	return &postgresPayloadCapturePolicyStore{pool: pool}
}

func (s *postgresPayloadCapturePolicyStore) Get(ctx context.Context, tenantID string) (PayloadCapturePolicy, error) {
	var (
		enabled bool
		raw     []byte
		version int64
	)

	err := s.pool.QueryRow(ctx,
		`SELECT enabled, allowed_decisions_jsonb, config_version
		 FROM payload_capture_policies
		 WHERE tenant_id = $1`,
		tenantID,
	).Scan(&enabled, &raw, &version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultPayloadCapturePolicy(tenantID), nil
		}

		return PayloadCapturePolicy{}, fmt.Errorf("postgres payload capture get: %w", err)
	}

	configVersion, err := dbUint64(version, "payload capture config version")
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	allowed, err := decodeCaptureDecisions(raw)
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	return PayloadCapturePolicy{TenantID: tenantID, Enabled: enabled, AllowedDecisions: allowed, ConfigVersion: configVersion}, nil
}

func (s *postgresPayloadCapturePolicyStore) Put(ctx context.Context, policy PayloadCapturePolicy, expectedVersion uint64) (PayloadCapturePolicy, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PayloadCapturePolicy{}, fmt.Errorf("postgres payload capture put begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := putPayloadCapturePolicyTx(ctx, tx, policy, expectedVersion)
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return PayloadCapturePolicy{}, fmt.Errorf("postgres payload capture put commit: %w", err)
	}

	return saved, nil
}

// PutPayloadCapturePolicy writes policy, bumps tenant version, and audits it.
func (s *PostgresConfigStore) PutPayloadCapturePolicy(ctx context.Context, policy PayloadCapturePolicy, expectedVersion uint64, actor ConfigActor) (PayloadCapturePolicy, error) {
	nextVersion, _, err := nextConfigVersionParam(expectedVersion)
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	saved := policy
	saved.ConfigVersion = nextVersion

	_, err = writeTenantConfig(ctx, s.pool, policy.TenantID, auditEntry{
		actor: actor, resourceType: "payload_capture_policy", resourceID: policy.TenantID, action: configActionUpdate,
		newValue: saved,
	}, func(ctx context.Context, tx pgx.Tx) error {
		var writeErr error

		saved, writeErr = putPayloadCapturePolicyTx(ctx, tx, policy, expectedVersion)

		return writeErr
	})
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	return saved, nil
}

func putPayloadCapturePolicyTx(ctx context.Context, tx pgx.Tx, policy PayloadCapturePolicy, expectedVersion uint64) (PayloadCapturePolicy, error) {
	current, err := currentResourceVersion(ctx, tx,
		`SELECT config_version FROM payload_capture_policies WHERE tenant_id = $1`,
		policy.TenantID)
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	if current != expectedVersion {
		return PayloadCapturePolicy{}, ErrPayloadCaptureVersionConflict
	}

	nextVersion, newVersion, err := nextConfigVersionParam(expectedVersion)
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	raw, err := encodeCaptureDecisions(policy.AllowedDecisions)
	if err != nil {
		return PayloadCapturePolicy{}, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO payload_capture_policies
		  (tenant_id, enabled, allowed_decisions_jsonb, created_at, updated_at, config_version)
		 VALUES ($1, $2, $3, now(), now(), $4)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   enabled = EXCLUDED.enabled,
		   allowed_decisions_jsonb = EXCLUDED.allowed_decisions_jsonb,
		   updated_at = now(),
		   config_version = EXCLUDED.config_version`,
		policy.TenantID, policy.Enabled, raw, newVersion,
	)
	if err != nil {
		return PayloadCapturePolicy{}, fmt.Errorf("postgres payload capture put: %w", err)
	}

	policy.ConfigVersion = nextVersion

	return policy, nil
}

func encodeCaptureDecisions(decisions []CaptureDecision) ([]byte, error) {
	out := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		out = append(out, string(decision))
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode payload capture decisions: %w", err)
	}

	return raw, nil
}

func decodeCaptureDecisions(raw []byte) ([]CaptureDecision, error) {
	var strings []string

	err := json.Unmarshal(raw, &strings)
	if err != nil {
		return nil, fmt.Errorf("decode payload capture decisions: %w", err)
	}

	out := make([]CaptureDecision, 0, len(strings))
	for _, s := range strings {
		out = append(out, CaptureDecision(s))
	}

	return out, nil
}

// NewPostgresQuotaStore builds a QuotaStore over the given pool.
func NewPostgresQuotaStore(pool *pgxpool.Pool) QuotaStore {
	return &postgresQuotaStore{pool: pool}
}

func quotaPeriod(period string) string {
	if period == "" {
		return quotaPeriodMonthly
	}

	return period
}

func requestCountPolicyToBool(policy string) bool {
	// count_on_success is the only value that disables count-on-admission;
	// every other value (including the default) counts admitted attempts
	// (docs/planning/20 "P0 default: count_on_admission=true").
	return policy != "count_on_success"
}

func boolToRequestCountPolicy(countOnAdmission bool) string {
	if countOnAdmission {
		return "count_on_admission"
	}

	return "count_on_success"
}

func quotaFailPolicy(policy string) string {
	if policy == postgresFailOpen {
		return postgresFailOpen
	}

	return postgresFailClosed
}

// Get fetches a tenant's monthly quota, defaulting to an empty version-0 config.
func (s *postgresQuotaStore) Get(ctx context.Context, tenantID string) (QuotaConfig, error) {
	var (
		period           string
		requestLimit     *int64
		bandwidthLimit   *int64
		countOnAdmission bool
		failPolicy       string
		version          int64
	)

	err := s.pool.QueryRow(ctx,
		`SELECT quota_period, request_count_limit, bandwidth_bytes_limit,
		        count_on_admission, fail_policy, config_version
		 FROM quota_configs
		 WHERE tenant_id = $1 AND quota_period = 'monthly'`,
		tenantID,
	).Scan(&period, &requestLimit, &bandwidthLimit, &countOnAdmission, &failPolicy, &version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QuotaConfig{TenantID: tenantID, Period: quotaPeriodMonthly, ConfigVersion: 0}, nil
		}

		return QuotaConfig{}, fmt.Errorf("postgres quota get: %w", err)
	}

	configVersion, err := dbUint64(version, "quota config version")
	if err != nil {
		return QuotaConfig{}, err
	}

	return QuotaConfig{
		TenantID:           tenantID,
		Period:             period,
		MaxRequests:        derefInt64(requestLimit),
		MaxBandwidthBytes:  derefInt64(bandwidthLimit),
		RequestCountPolicy: boolToRequestCountPolicy(countOnAdmission),
		RedisFailPolicy:    failPolicy,
		ConfigVersion:      configVersion,
	}, nil
}

// Put upserts a tenant's monthly quota under optimistic concurrency on the
// quota's config_version.
func (s *postgresQuotaStore) Put(ctx context.Context, quota QuotaConfig, expectedVersion uint64) (QuotaConfig, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return QuotaConfig{}, fmt.Errorf("postgres quota put begin: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := putQuotaConfigTx(ctx, tx, quota, expectedVersion)
	if err != nil {
		return QuotaConfig{}, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return QuotaConfig{}, fmt.Errorf("postgres quota put commit: %w", err)
	}

	return saved, nil
}

// PutQuotaConfig writes quota config, bumps the tenant snapshot version, and
// appends config audit in one transaction.
func (s *PostgresConfigStore) PutQuotaConfig(ctx context.Context, quota QuotaConfig, expectedVersion uint64, actor ConfigActor) (QuotaConfig, error) {
	nextVersion, _, err := nextConfigVersionParam(expectedVersion)
	if err != nil {
		return QuotaConfig{}, err
	}

	saved := quota
	saved.Period = quotaPeriod(quota.Period)
	saved.ConfigVersion = nextVersion

	_, err = writeTenantConfig(ctx, s.pool, quota.TenantID, auditEntry{
		actor: actor, resourceType: "quota_config", resourceID: quota.TenantID, action: configActionUpdate,
		newValue: saved,
	}, func(ctx context.Context, tx pgx.Tx) error {
		var writeErr error

		saved, writeErr = putQuotaConfigTx(ctx, tx, quota, expectedVersion)

		return writeErr
	})
	if err != nil {
		return QuotaConfig{}, err
	}

	return saved, nil
}

func putQuotaConfigTx(ctx context.Context, tx pgx.Tx, quota QuotaConfig, expectedVersion uint64) (QuotaConfig, error) {
	period := quotaPeriod(quota.Period)

	current, err := currentResourceVersion(ctx, tx,
		`SELECT config_version FROM quota_configs WHERE tenant_id = $1 AND quota_period = $2`,
		quota.TenantID, period)
	if err != nil {
		return QuotaConfig{}, err
	}

	if current != expectedVersion {
		return QuotaConfig{}, ErrQuotaVersionConflict
	}

	nextVersion, newVersion, err := nextConfigVersionParam(expectedVersion)
	if err != nil {
		return QuotaConfig{}, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO quota_configs
		  (tenant_id, quota_period, enabled, request_count_limit, bandwidth_bytes_limit,
		   count_on_admission, fail_policy, created_at, updated_at, config_version)
		 VALUES ($1, $2, true, $3, $4, $5, $6, now(), now(), $7)
		 ON CONFLICT (tenant_id, quota_period) DO UPDATE SET
		   enabled = true,
		   request_count_limit = EXCLUDED.request_count_limit,
		   bandwidth_bytes_limit = EXCLUDED.bandwidth_bytes_limit,
		   count_on_admission = EXCLUDED.count_on_admission,
		   fail_policy = EXCLUDED.fail_policy,
		   updated_at = now(),
		   config_version = EXCLUDED.config_version`,
		quota.TenantID, period, quota.MaxRequests, quota.MaxBandwidthBytes,
		requestCountPolicyToBool(quota.RequestCountPolicy), quotaFailPolicy(quota.RedisFailPolicy), newVersion,
	)
	if err != nil {
		return QuotaConfig{}, fmt.Errorf("postgres quota put: %w", err)
	}

	quota.Period = period
	quota.ConfigVersion = nextVersion

	return quota, nil
}

// ---- Rate-limit configs ----

// postgresRateLimitConfigStore implements RateLimitConfigStore over
// rate_limit_configs, modeling a tenant's limit set as one aggregate versioned
// unit (matching the in-memory store's per-tenant RateLimitConfig).
type postgresRateLimitConfigStore struct {
	pool *pgxpool.Pool
}

// NewPostgresRateLimitConfigStore builds a RateLimitConfigStore over the pool.
func NewPostgresRateLimitConfigStore(pool *pgxpool.Pool) RateLimitConfigStore {
	return &postgresRateLimitConfigStore{pool: pool}
}

func rateLimitFailPolicyToDB(policy RateLimitFailPolicy) string {
	if policy == RateLimitFailClosed {
		return postgresFailClosed
	}

	return postgresFailOpen
}

func rateLimitFailPolicyFromDB(policy string) RateLimitFailPolicy {
	if policy == postgresFailClosed {
		return RateLimitFailClosed
	}

	return RateLimitFailOpen
}

// Get returns a tenant's rate-limit config, aggregating the enabled rows under
// their max config_version.
func (s *postgresRateLimitConfigStore) Get(ctx context.Context, tenantID string) (RateLimitConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT dimension, key, window_ms, limit_count, fail_policy, config_version
		 FROM rate_limit_configs
		 WHERE tenant_id = $1 AND enabled = true
		 ORDER BY dimension, key`,
		tenantID,
	)
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("postgres rate limit get: %w", err)
	}

	defer rows.Close()

	cfg := RateLimitConfig{TenantID: tenantID}

	for rows.Next() {
		rule, configVersion, err := scanRateLimitConfigRow(rows)
		if err != nil {
			return RateLimitConfig{}, err
		}

		if configVersion > cfg.ConfigVersion {
			cfg.ConfigVersion = configVersion
		}

		cfg.Limits = append(cfg.Limits, rule)
	}

	err = rows.Err()
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("postgres rate limit rows: %w", err)
	}

	return cfg, nil
}

func scanRateLimitConfigRow(rows pgx.Rows) (RateLimitRule, uint64, error) {
	var (
		dimension  string
		key        string
		windowMS   int64
		limitCount int64
		failPolicy string
		version    int64
	)

	err := rows.Scan(&dimension, &key, &windowMS, &limitCount, &failPolicy, &version)
	if err != nil {
		return RateLimitRule{}, 0, fmt.Errorf("postgres rate limit scan: %w", err)
	}

	configVersion, err := dbUint64(version, "rate limit config version")
	if err != nil {
		return RateLimitRule{}, 0, err
	}

	windowSeconds, err := dbWindowSeconds(windowMS)
	if err != nil {
		return RateLimitRule{}, 0, err
	}

	maxRequests, err := dbUint32(limitCount, "rate limit count")
	if err != nil {
		return RateLimitRule{}, 0, err
	}

	return RateLimitRule{
		Dimension:     RateLimitDimension(dimension),
		Key:           key,
		WindowSeconds: windowSeconds,
		MaxRequests:   maxRequests,
		FailPolicy:    rateLimitFailPolicyFromDB(failPolicy),
	}, configVersion, nil
}

// Put replaces a tenant's rate-limit set under optimistic concurrency on the
// aggregate config version, after validating each limit against the ceiling.
func (s *postgresRateLimitConfigStore) Put(ctx context.Context, cfg RateLimitConfig, expectedVersion uint64, ceiling *RateLimitCeiling) (RateLimitConfig, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("postgres rate limit put begin: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := putRateLimitConfigTx(ctx, tx, cfg, expectedVersion, ceiling)
	if err != nil {
		return RateLimitConfig{}, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("postgres rate limit put commit: %w", err)
	}

	return saved, nil
}

// PutRateLimitConfig writes rate-limit config, bumps the tenant snapshot
// version, and appends config audit in one transaction.
func (s *PostgresConfigStore) PutRateLimitConfig(ctx context.Context, cfg RateLimitConfig, expectedVersion uint64, ceiling *RateLimitCeiling, actor ConfigActor) (RateLimitConfig, error) {
	nextVersion, _, err := nextConfigVersionParam(expectedVersion)
	if err != nil {
		return RateLimitConfig{}, err
	}

	saved := cfg
	saved.ConfigVersion = nextVersion

	_, err = writeTenantConfig(ctx, s.pool, cfg.TenantID, auditEntry{
		actor: actor, resourceType: "rate_limit_config", resourceID: cfg.TenantID, action: configActionUpdate,
		newValue: saved,
	}, func(ctx context.Context, tx pgx.Tx) error {
		var writeErr error

		saved, writeErr = putRateLimitConfigTx(ctx, tx, cfg, expectedVersion, ceiling)

		return writeErr
	})
	if err != nil {
		return RateLimitConfig{}, err
	}

	return saved, nil
}

func putRateLimitConfigTx(ctx context.Context, tx pgx.Tx, cfg RateLimitConfig, expectedVersion uint64, ceiling *RateLimitCeiling) (RateLimitConfig, error) {
	if ceiling != nil {
		if slices.ContainsFunc(cfg.Limits, ceiling.exceeds) {
			return RateLimitConfig{}, ErrRateLimitCeilingExceeded
		}
	}

	current, err := currentResourceVersion(ctx, tx,
		`SELECT COALESCE(MAX(config_version), 0) FROM rate_limit_configs WHERE tenant_id = $1`,
		cfg.TenantID)
	if err != nil {
		return RateLimitConfig{}, err
	}

	if current != expectedVersion {
		return RateLimitConfig{}, ErrRateLimitVersionConflict
	}

	nextVersion, newVersion, err := nextConfigVersionParam(expectedVersion)
	if err != nil {
		return RateLimitConfig{}, err
	}

	_, err = tx.Exec(ctx, `DELETE FROM rate_limit_configs WHERE tenant_id = $1`, cfg.TenantID)
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("postgres rate limit clear: %w", err)
	}

	for _, rule := range cfg.Limits {
		_, err = tx.Exec(ctx,
			`INSERT INTO rate_limit_configs
			  (tenant_id, dimension, key, enabled, window_ms, fail_policy, limit_count,
			   created_at, updated_at, config_version)
			 VALUES ($1, $2, $3, true, $4, $5, $6, now(), now(), $7)`,
			cfg.TenantID, string(rule.Dimension), rule.Key, int64(rule.WindowSeconds)*millisPerSecond,
			rateLimitFailPolicyToDB(rule.FailPolicy), int64(rule.MaxRequests), newVersion,
		)
		if err != nil {
			return RateLimitConfig{}, fmt.Errorf("postgres rate limit insert: %w", err)
		}
	}

	cfg.ConfigVersion = nextVersion

	return cfg, nil
}

// currentResourceVersion reads a single config_version scalar (0 when absent).
func currentResourceVersion(ctx context.Context, tx pgx.Tx, query string, args ...any) (uint64, error) {
	var v int64

	err := tx.QueryRow(ctx, query, args...).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}

		return 0, fmt.Errorf("read resource version: %w", err)
	}

	version, err := dbUint64(v, "resource config version")
	if err != nil {
		return 0, err
	}

	return version, nil
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}

	return *v
}
