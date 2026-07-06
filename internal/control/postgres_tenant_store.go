package control

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresTenantStore implements TenantStore using Postgres.
type postgresTenantStore struct {
	pool *pgxpool.Pool
}

// NewPostgresTenantStore creates a TenantStore backed by the given pool.
func NewPostgresTenantStore(pool *pgxpool.Pool) TenantStore {
	return &postgresTenantStore{pool: pool}
}

func ceilingParams(c *RateLimitCeiling) (*int64, *int64) {
	if c == nil {
		return nil, nil
	}

	w, m := int64(c.WindowSeconds), int64(c.MaxRequests)

	return &w, &m
}

func ceilingFromRow(windowSeconds, maxRequests *int64) (*RateLimitCeiling, error) {
	if windowSeconds == nil || maxRequests == nil {
		return nil, nil
	}

	w, err := dbUint32(*windowSeconds, "rate_limit_ceiling_window_seconds")
	if err != nil {
		return nil, err
	}

	m, err := dbUint32(*maxRequests, "rate_limit_ceiling_max_requests")
	if err != nil {
		return nil, err
	}

	return &RateLimitCeiling{WindowSeconds: w, MaxRequests: m}, nil
}

func tenantTimeoutParam(v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, errInvalidTenantTimeouts
	}

	return int64(v), nil
}

// Create inserts a tenant record.
func (s *postgresTenantStore) Create(ctx context.Context, tenant Tenant) error {
	tenant = normalizeTenant(tenant)

	err := validateTenantPolicy(tenant)
	if err != nil {
		return err
	}

	defaultTimeout, err := tenantTimeoutParam(tenant.DefaultTimeoutMs)
	if err != nil {
		return err
	}

	maxTimeout, err := tenantTimeoutParam(tenant.MaxTimeoutMs)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	_, err = s.pool.Exec(ctx,
		`INSERT INTO tenants
		 (id, name, status, default_timeout_ms, max_timeout_ms, metadata_query_storage, metadata_path_storage,
		  created_at, updated_at, config_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 0)`,
		tenant.ID,
		tenant.Name,
		string(tenant.Status),
		defaultTimeout,
		maxTimeout,
		string(tenant.MetadataQueryStorage),
		string(tenant.MetadataPathStorage),
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("postgres tenant create: %w", err)
	}

	return nil
}

const tenantSelectColumns = `id, name, status, default_timeout_ms, max_timeout_ms, metadata_query_storage,
	metadata_path_storage, rate_limit_ceiling_window_seconds, rate_limit_ceiling_max_requests, config_version,
	created_at, updated_at, deleted_at`

func scanTenant(row pgx.Row) (Tenant, error) {
	var (
		id, name, status           string
		queryStorage, pathStorage  string
		windowSeconds, maxRequests *int64
		defaultTimeout, maxTimeout int64
		configVersion              int64
		createdAt, updatedAt       time.Time
		deletedAt                  *time.Time
	)

	err := row.Scan(&id, &name, &status, &defaultTimeout, &maxTimeout, &queryStorage, &pathStorage, &windowSeconds, &maxRequests, &configVersion, &createdAt, &updatedAt, &deletedAt)
	if err != nil {
		return Tenant{}, fmt.Errorf("scan tenant row: %w", err)
	}

	version, err := dbUint64(configVersion, "tenant config version")
	if err != nil {
		return Tenant{}, err
	}

	ceiling, err := ceilingFromRow(windowSeconds, maxRequests)
	if err != nil {
		return Tenant{}, err
	}

	defaultMs, err := dbUint64(defaultTimeout, "tenant default timeout")
	if err != nil {
		return Tenant{}, err
	}

	maxMs, err := dbUint64(maxTimeout, "tenant max timeout")
	if err != nil {
		return Tenant{}, err
	}

	tenant := Tenant{
		ID:                   id,
		Name:                 name,
		Status:               TenantStatus(status),
		DefaultTimeoutMs:     defaultMs,
		MaxTimeoutMs:         maxMs,
		MetadataQueryStorage: MetadataStoragePolicy(queryStorage),
		MetadataPathStorage:  MetadataStoragePolicy(pathStorage),
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
		DeletedAt:            deletedAt,
		RateLimitCeiling:     ceiling,
		ConfigVersion:        version,
	}

	err = validateTenantPolicy(tenant)
	if err != nil {
		return Tenant{}, err
	}

	return tenant, nil
}

// Get fetches a tenant by ID.
func (s *postgresTenantStore) Get(ctx context.Context, id string) (Tenant, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+tenantSelectColumns+` FROM tenants WHERE id = $1`, id)

	tenant, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tenant{}, ErrTenantNotFound
		}

		return Tenant{}, fmt.Errorf("postgres tenant get: %w", err)
	}

	return tenant, nil
}

// List returns tenants ordered by created_at descending, then id ascending.
func (s *postgresTenantStore) List(ctx context.Context, limit, offset int) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+tenantSelectColumns+` FROM tenants ORDER BY created_at DESC, id ASC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres tenant list: %w", err)
	}
	defer rows.Close()

	out := []Tenant{}

	for rows.Next() {
		tenant, scanErr := scanTenant(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("postgres tenant list scan: %w", scanErr)
		}

		out = append(out, tenant)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("postgres tenant list rows: %w", err)
	}

	return out, nil
}

// Update replaces name/status/rate_limit_ceiling under optimistic
// concurrency against the tenant's own config_version.
func (s *postgresTenantStore) Update(ctx context.Context, tenant Tenant, expectedVersion uint64) (Tenant, error) {
	tenant = normalizeTenant(tenant)

	err := validateTenantPolicy(tenant)
	if err != nil {
		return Tenant{}, err
	}

	defaultTimeout, err := tenantTimeoutParam(tenant.DefaultTimeoutMs)
	if err != nil {
		return Tenant{}, err
	}

	maxTimeout, err := tenantTimeoutParam(tenant.MaxTimeoutMs)
	if err != nil {
		return Tenant{}, err
	}

	windowSeconds, maxRequests := ceilingParams(tenant.RateLimitCeiling)

	expectedVersionParam, err := configVersionParam(expectedVersion)
	if err != nil {
		return Tenant{}, err
	}

	row := s.pool.QueryRow(ctx,
		`UPDATE tenants
		 SET name = $3, status = $4, default_timeout_ms = $5, max_timeout_ms = $6,
		     metadata_query_storage = $7, metadata_path_storage = $8,
		     rate_limit_ceiling_window_seconds = $9, rate_limit_ceiling_max_requests = $10,
		     config_version = config_version + 1, updated_at = now()
		 WHERE id = $1 AND config_version = $2 AND status != 'deleted'
		 RETURNING `+tenantSelectColumns,
		tenant.ID, expectedVersionParam, tenant.Name, string(tenant.Status), defaultTimeout,
		maxTimeout, string(tenant.MetadataQueryStorage), string(tenant.MetadataPathStorage),
		windowSeconds, maxRequests,
	)

	updated, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.updateConflictOrNotFound(ctx, tenant.ID)
		}

		return Tenant{}, fmt.Errorf("postgres tenant update: %w", err)
	}

	return updated, nil
}

// SoftDelete marks a tenant deleted.
func (s *postgresTenantStore) SoftDelete(ctx context.Context, id string) (Tenant, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET status = 'deleted', deleted_at = now(), updated_at = now(), config_version = config_version + 1
		 WHERE id = $1 AND status != 'deleted'`,
		id,
	)
	if err != nil {
		return Tenant{}, fmt.Errorf("postgres tenant soft delete: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return Tenant{}, ErrTenantNotFound
	}

	return s.Get(ctx, id)
}

// updateConflictOrNotFound distinguishes "no such live tenant" from "version
// mismatch" after an Update's RETURNING clause matched zero rows.
func (s *postgresTenantStore) updateConflictOrNotFound(ctx context.Context, id string) (Tenant, error) {
	current, err := s.Get(ctx, id)
	if err != nil {
		return Tenant{}, err
	}

	if current.Status == TenantStatusDeleted {
		return Tenant{}, ErrTenantNotFound
	}

	return Tenant{}, ErrTenantVersionConflict
}
