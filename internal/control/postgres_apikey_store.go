package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresAPIKeyStore implements APIKeyStore using Postgres.
type postgresAPIKeyStore struct {
	pool   *pgxpool.Pool
	pepper []byte
}

// NewPostgresAPIKeyStore creates an APIKeyStore backed by the given pool.
func NewPostgresAPIKeyStore(pool *pgxpool.Pool, pepper []byte) APIKeyStore {
	return &postgresAPIKeyStore{pool: pool, pepper: pepper}
}

// Create inserts an API key record.
func (s *postgresAPIKeyStore) Create(ctx context.Context, record APIKeyRecord) error {
	var tenantIDArg any

	if record.TenantID != "" {
		tenantIDArg = record.TenantID
	}

	now := time.Now().UTC()

	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO api_keys (id, scope_type, tenant_id, role, prefix, secret_hash,
		                        status, created_at, revoked_at, config_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		record.ID,
		string(record.ScopeType),
		tenantIDArg,
		string(record.Role),
		record.Prefix,
		record.SecretHash,
		string(record.Status),
		now,
		nil, // revoked_at
		record.ConfigVersion,
	)
	if err != nil {
		return fmt.Errorf("postgres api key create: %w", err)
	}

	return nil
}

// FindByPrefix returns active records whose prefix matches.
func (s *postgresAPIKeyStore) FindByPrefix(ctx context.Context, prefix string) ([]APIKeyRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, scope_type, tenant_id, role, prefix, secret_hash,
		        status, created_at, revoked_at, config_version
		 FROM api_keys
		 WHERE prefix = $1 AND status = 'active'`,
		prefix,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres api key find by prefix: %w", err)
	}

	defer rows.Close()

	return scanAPIKeyRows(rows)
}

// Get returns an API key record by ID.
func (s *postgresAPIKeyStore) Get(ctx context.Context, id string) (APIKeyRecord, error) {
	var (
		r        APIKeyRecord
		tenantID *string
	)

	err := s.pool.QueryRow(
		ctx,
		`SELECT id, scope_type, tenant_id, role, prefix, secret_hash,
		        status, created_at, revoked_at, config_version
		 FROM api_keys WHERE id = $1`,
		id,
	).Scan(
		&r.ID,
		(*string)(&r.ScopeType),
		&tenantID,
		(*string)(&r.Role),
		&r.Prefix,
		&r.SecretHash,
		(*string)(&r.Status),
		&r.CreatedAt,
		&r.RevokedAt,
		&r.ConfigVersion,
	)
	if err != nil {
		return APIKeyRecord{}, fmt.Errorf("postgres api key get: %w", err)
	}

	if tenantID != nil {
		r.TenantID = *tenantID
	}

	return r, nil
}

// Revoke marks an API key as revoked.
func (s *postgresAPIKeyStore) Revoke(ctx context.Context, id string, revokedAt time.Time) (APIKeyRecord, error) {
	var (
		r        APIKeyRecord
		tenantID *string
	)

	err := s.pool.QueryRow(
		ctx,
		`UPDATE api_keys
		 SET status = 'revoked', revoked_at = $2
		 WHERE id = $1
		   AND status = 'active'
		 RETURNING id, scope_type, tenant_id, role, prefix, secret_hash,
		           status, created_at, revoked_at, config_version`,
		id,
		revokedAt,
	).Scan(
		&r.ID,
		(*string)(&r.ScopeType),
		&tenantID,
		(*string)(&r.Role),
		&r.Prefix,
		&r.SecretHash,
		(*string)(&r.Status),
		&r.CreatedAt,
		&r.RevokedAt,
		&r.ConfigVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APIKeyRecord{}, ErrAPIKeyNotFound
		}

		return APIKeyRecord{}, fmt.Errorf("postgres api key revoke: %w", err)
	}

	if tenantID != nil {
		r.TenantID = *tenantID
	}

	return r, nil
}

// ListPlatform returns platform-scoped API keys.
func (s *postgresAPIKeyStore) ListPlatform(ctx context.Context) ([]APIKeyRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, scope_type, tenant_id, role, prefix, secret_hash,
		        status, created_at, revoked_at, config_version
		 FROM api_keys WHERE scope_type = 'platform'`,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres api key list platform: %w", err)
	}

	defer rows.Close()

	return scanAPIKeyRows(rows)
}

// ListTenant returns tenant-scoped API keys for a tenant.
func (s *postgresAPIKeyStore) ListTenant(ctx context.Context, tenantID string) ([]APIKeyRecord, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, scope_type, tenant_id, role, prefix, secret_hash,
		        status, created_at, revoked_at, config_version
		 FROM api_keys
		 WHERE scope_type = 'tenant' AND tenant_id = $1`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres api key list tenant: %w", err)
	}

	defer rows.Close()

	return scanAPIKeyRows(rows)
}

// CountPlatformSystemAdmins returns the number of active platform system_admin keys.
func (s *postgresAPIKeyStore) CountPlatformSystemAdmins(ctx context.Context) (int, error) {
	var count int

	err := s.pool.QueryRow(
		ctx,
		`SELECT count(*) FROM api_keys
		 WHERE scope_type = 'platform' AND role = 'system_admin' AND status = 'active'`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("postgres api key count platform admins: %w", err)
	}

	return count, nil
}

// scanAPIKeyRows is a shared helper to decode rows from api_keys queries.
func scanAPIKeyRows(rows interface {
	Next() bool
	Scan(val ...any) error
	Close()
	Err() error
},
) ([]APIKeyRecord, error) {
	var out []APIKeyRecord

	for rows.Next() {
		var (
			r        APIKeyRecord
			tenantID *string
		)

		err := rows.Scan(
			&r.ID,
			(*string)(&r.ScopeType),
			&tenantID,
			(*string)(&r.Role),
			&r.Prefix,
			&r.SecretHash,
			(*string)(&r.Status),
			&r.CreatedAt,
			&r.RevokedAt,
			&r.ConfigVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres api key scan: %w", err)
		}

		if tenantID != nil {
			r.TenantID = *tenantID
		}

		out = append(out, r)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("postgres api key rows: %w", err)
	}

	return out, nil
}
