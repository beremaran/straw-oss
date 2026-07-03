package control

import (
	"context"
	"fmt"
	"time"

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

// Create inserts a tenant record.
func (s *postgresTenantStore) Create(ctx context.Context, tenant Tenant) error {
	now := time.Now().UTC()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO tenants (id, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4)`,
		tenant.ID,
		string(tenant.Status),
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("postgres tenant create: %w", err)
	}

	return nil
}

// Get fetches a tenant by ID.
func (s *postgresTenantStore) Get(ctx context.Context, id string) (Tenant, error) {
	var (
		status    string
		createdAt time.Time
	)

	err := s.pool.QueryRow(ctx,
		`SELECT status, created_at FROM tenants WHERE id = $1`,
		id,
	).Scan(&status, &createdAt)
	if err != nil {
		return Tenant{}, fmt.Errorf("postgres tenant get: %w", err)
	}

	return Tenant{
		ID:        id,
		Status:    TenantStatus(status),
		CreatedAt: createdAt,
	}, nil
}
