package control

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresWorkerCredentialStore implements WorkerCredentialStore using Postgres.
type postgresWorkerCredentialStore struct {
	pool *pgxpool.Pool
}

// NewPostgresWorkerCredentialStore creates a WorkerCredentialStore backed by the given pool.
func NewPostgresWorkerCredentialStore(pool *pgxpool.Pool) WorkerCredentialStore {
	return &postgresWorkerCredentialStore{pool: pool}
}

// Create inserts a worker credential record.
func (s *postgresWorkerCredentialStore) Create(ctx context.Context, record WorkerCredential) error {
	now := time.Now().UTC()

	tenantScopeJSON, err := json.Marshal(record.TenantScope)
	if err != nil {
		return fmt.Errorf("worker credential create marshal tenant scope: %w", err)
	}

	allowedPoolsJSON, err := json.Marshal(record.AllowedPools)
	if err != nil {
		return fmt.Errorf("worker credential create marshal allowed pools: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO worker_credentials (credential_id, status, executor_type,
		                                  public_key_ed25519_base64, tenant_scope_jsonb,
		                                  allowed_pools_jsonb, created_at, updated_at,
		                                  config_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		record.ID,
		string(record.Status),
		record.ExecutorType,
		record.PublicKeyEd25519Base64,
		tenantScopeJSON,
		allowedPoolsJSON,
		now,
		now,
		record.ConfigVersion,
	)
	if err != nil {
		return fmt.Errorf("postgres worker credential create: %w", err)
	}

	return nil
}

// Get fetches a worker credential by ID.
func (s *postgresWorkerCredentialStore) Get(ctx context.Context, id string) (WorkerCredential, error) {
	var (
		record                            WorkerCredential
		tenantScopeJSON, allowedPoolsJSON []byte
		createdAt, updatedAt              time.Time
	)

	err := s.pool.QueryRow(ctx,
		`SELECT status, executor_type, public_key_ed25519_base64,
		        tenant_scope_jsonb, allowed_pools_jsonb,
		        created_at, updated_at, config_version
		 FROM worker_credentials WHERE credential_id = $1`,
		id,
	).Scan(
		(*string)(&record.Status),
		&record.ExecutorType,
		&record.PublicKeyEd25519Base64,
		&tenantScopeJSON,
		&allowedPoolsJSON,
		&createdAt,
		&updatedAt,
		&record.ConfigVersion,
	)
	if err != nil {
		return WorkerCredential{}, fmt.Errorf("postgres worker credential get: %w", err)
	}

	record.ID = id
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt

	if len(tenantScopeJSON) > 0 {
		_ = json.Unmarshal(tenantScopeJSON, &record.TenantScope)
	}

	if len(allowedPoolsJSON) > 0 {
		_ = json.Unmarshal(allowedPoolsJSON, &record.AllowedPools)
	}

	return record, nil
}

// Revoke marks a worker credential as revoked.
func (s *postgresWorkerCredentialStore) Revoke(ctx context.Context, id string, revokedAt time.Time) (WorkerCredential, error) {
	var (
		record                            WorkerCredential
		tenantScopeJSON, allowedPoolsJSON []byte
	)

	err := s.pool.QueryRow(ctx,
		`UPDATE worker_credentials
		 SET status = 'revoked', updated_at = $2
		 WHERE credential_id = $1 AND status = 'active'
		 RETURNING executor_type, public_key_ed25519_base64,
		           tenant_scope_jsonb, allowed_pools_jsonb,
		           created_at, updated_at, config_version`,
		id,
		revokedAt,
	).Scan(
		&record.ExecutorType,
		&record.PublicKeyEd25519Base64,
		&tenantScopeJSON,
		&allowedPoolsJSON,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.ConfigVersion,
	)
	if err != nil {
		return WorkerCredential{}, fmt.Errorf("postgres worker credential revoke: %w", err)
	}

	record.ID = id
	record.Status = WorkerCredentialStatusRevoked

	if len(tenantScopeJSON) > 0 {
		_ = json.Unmarshal(tenantScopeJSON, &record.TenantScope)
	}

	if len(allowedPoolsJSON) > 0 {
		_ = json.Unmarshal(allowedPoolsJSON, &record.AllowedPools)
	}

	return record, nil
}

// ListTenant returns the credentials scoped to a tenant.
func (s *postgresWorkerCredentialStore) ListTenant(ctx context.Context, tenantID string) ([]WorkerCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT credential_id, status, executor_type, public_key_ed25519_base64,
		        tenant_scope_jsonb, allowed_pools_jsonb,
		        created_at, updated_at, config_version
		 FROM worker_credentials
		 WHERE status = 'active'`,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres worker credential list: %w", err)
	}

	defer rows.Close()

	var out []WorkerCredential

	for rows.Next() {
		var (
			record                            WorkerCredential
			tenantScopeJSON, allowedPoolsJSON []byte
			createdAt, updatedAt              time.Time
		)

		err := rows.Scan(
			&record.ID,
			(*string)(&record.Status),
			&record.ExecutorType,
			&record.PublicKeyEd25519Base64,
			&tenantScopeJSON,
			&allowedPoolsJSON,
			&createdAt,
			&updatedAt,
			&record.ConfigVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres worker credential scan: %w", err)
		}

		record.CreatedAt = createdAt
		record.UpdatedAt = updatedAt

		if len(tenantScopeJSON) > 0 {
			_ = json.Unmarshal(tenantScopeJSON, &record.TenantScope)
		}

		if len(allowedPoolsJSON) > 0 {
			_ = json.Unmarshal(allowedPoolsJSON, &record.AllowedPools)
		}

		// P0: single-tenant credentials only — check tenant scope
		if slices.Contains(record.TenantScope, tenantID) {
			out = append(out, record)
		}
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("postgres worker credential rows: %w", err)
	}

	return out, nil
}
