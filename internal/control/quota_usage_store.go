package control

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresQuotaUsageStore struct {
	pool *pgxpool.Pool
}

// NewPostgresQuotaUsageStore builds the durable reconciled quota snapshot store.
func NewPostgresQuotaUsageStore(pool *pgxpool.Pool) QuotaUsageSnapshotStore {
	return &postgresQuotaUsageStore{pool: pool}
}

func (s *postgresQuotaUsageStore) GetQuotaUsage(ctx context.Context, tenantID, period string) (QuotaUsage, bool, error) {
	var usage QuotaUsage

	err := s.pool.QueryRow(ctx,
		`SELECT tenant_id, quota_period, request_count, bandwidth_bytes, accurate_through, source, aggregation_key_version
		 FROM quota_usage_snapshots
		 WHERE tenant_id = $1 AND quota_period = $2`,
		tenantID, period,
	).Scan(&usage.TenantID, &usage.Period, &usage.RequestCount, &usage.BandwidthBytes, &usage.AccurateThrough, &usage.Source, &usage.AggregationKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QuotaUsage{}, false, nil
		}

		return QuotaUsage{}, false, fmt.Errorf("postgres quota usage snapshot get: %w", err)
	}

	return usage, true, nil
}

func (s *postgresQuotaUsageStore) PutQuotaUsage(ctx context.Context, usage QuotaUsage) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO quota_usage_snapshots
		  (tenant_id, quota_period, request_count, bandwidth_bytes, accurate_through, source, aggregation_key_version, reconciled_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 ON CONFLICT (tenant_id, quota_period) DO UPDATE SET
		   request_count = EXCLUDED.request_count,
		   bandwidth_bytes = EXCLUDED.bandwidth_bytes,
		   accurate_through = EXCLUDED.accurate_through,
		   source = EXCLUDED.source,
		   aggregation_key_version = EXCLUDED.aggregation_key_version,
		   reconciled_at = now()`,
		usage.TenantID, usage.Period, usage.RequestCount, usage.BandwidthBytes, usage.AccurateThrough,
		usage.Source, usage.AggregationKey,
	)
	if err != nil {
		return fmt.Errorf("postgres quota usage snapshot put: %w", err)
	}

	return nil
}
