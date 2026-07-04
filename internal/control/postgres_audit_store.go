package control

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresAuditStore implements AuditStore using Postgres.
type postgresAuditStore struct {
	pool *pgxpool.Pool
}

// NewPostgresAuditStore creates an AuditStore backed by the given pool.
func NewPostgresAuditStore(pool *pgxpool.Pool) AuditStore {
	return &postgresAuditStore{pool: pool}
}

// Record appends an audit record.
func (s *postgresAuditStore) Record(ctx context.Context, record AuditRecord) error {
	now := time.Now().UTC()

	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO config_audit_source
		 (tenant_id, actor_type, actor_id, resource_type, resource_id, action, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		nullString(record.TenantID),
		record.ActorType,
		record.ActorID,
		record.ResourceType,
		record.ResourceID,
		record.Action,
		record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres audit record: %w", err)
	}

	return nil
}

// ListTenant returns audit records for a tenant.
func (s *postgresAuditStore) ListTenant(ctx context.Context, tenantID string) ([]AuditRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, actor_type, actor_id, resource_type, resource_id,
		        action, created_at
		 FROM config_audit_source
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres audit list tenant: %w", err)
	}

	defer rows.Close()

	var out []AuditRecord

	for rows.Next() {
		var (
			r        AuditRecord
			tenantID *string
		)

		err := rows.Scan(
			&r.ID,
			&tenantID,
			&r.ActorType,
			&r.ActorID,
			&r.ResourceType,
			&r.ResourceID,
			&r.Action,
			&r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres audit scan: %w", err)
		}

		if tenantID != nil {
			r.TenantID = *tenantID
		}

		out = append(out, r)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("postgres audit rows: %w", err)
	}

	return out, nil
}

// ListTenantPage returns a paginated, tenant-scoped view of the audit log,
// sorted created_at descending then id ascending (docs/planning/26 shared
// list contract).
func (s *postgresAuditStore) ListTenantPage(ctx context.Context, tenantID string, limit, offset int) ([]AuditRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, actor_type, actor_id, resource_type, resource_id,
		        action, created_at
		 FROM config_audit_source
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id ASC
		 LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres audit list tenant page: %w", err)
	}

	defer rows.Close()

	out := []AuditRecord{}

	for rows.Next() {
		var (
			r        AuditRecord
			tenantID *string
		)

		err := rows.Scan(
			&r.ID,
			&tenantID,
			&r.ActorType,
			&r.ActorID,
			&r.ResourceType,
			&r.ResourceID,
			&r.Action,
			&r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres audit scan: %w", err)
		}

		if tenantID != nil {
			r.TenantID = *tenantID
		}

		out = append(out, r)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("postgres audit rows: %w", err)
	}

	return out, nil
}

// nullString returns nil for empty strings so Postgres gets NULL instead of "".
func nullString(s string) any {
	if s == "" {
		return nil
	}

	return s
}
