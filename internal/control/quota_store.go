package control

import (
	"context"
	"errors"
	"sync"
	"time"
)

// QuotaConfig mirrors the Quota Config resource schema in
// docs/planning/26-config-management-api-surface.md. It is written only by
// system_admin through PUT /tenants/{id}/quotas; tenant keys retain
// read-only access through GET /quotas.
type QuotaConfig struct {
	TenantID           string
	Period             string
	MaxRequests        int64
	MaxBandwidthBytes  int64
	RequestCountPolicy string
	RedisFailPolicy    string
	ConfigVersion      uint64
	UpdatedAt          time.Time
}

// ErrQuotaVersionConflict is returned when optimistic concurrency fails.
var ErrQuotaVersionConflict = errors.New("quota config version conflict")

// QuotaStore persists per-tenant quota configuration with optimistic
// concurrency on ConfigVersion (docs/planning/26: "All update requests
// include expected_config_version. Version mismatch returns conflict.").
type QuotaStore interface {
	Get(ctx context.Context, tenantID string) (QuotaConfig, error)
	Put(ctx context.Context, quota QuotaConfig, expectedVersion uint64) (QuotaConfig, error)
}

// InMemoryQuotaStore is the test/local quota store implementation.
type InMemoryQuotaStore struct {
	mu    sync.Mutex
	byTid map[string]QuotaConfig
}

// NewInMemoryQuotaStore builds an empty quota store.
func NewInMemoryQuotaStore() *InMemoryQuotaStore {
	return &InMemoryQuotaStore{byTid: make(map[string]QuotaConfig)}
}

// Get fetches a tenant quota config.
func (s *InMemoryQuotaStore) Get(_ context.Context, tenantID string) (QuotaConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.byTid[tenantID]
	if !ok {
		// An unconfigured tenant reads as an empty, version-0 quota config
		// rather than an error, mirroring the tenant-snapshot behavior.
		return QuotaConfig{TenantID: tenantID, Period: quotaPeriodMonthly, ConfigVersion: 0}, nil
	}

	return q, nil
}

// Put updates a tenant quota config with optimistic concurrency.
func (s *InMemoryQuotaStore) Put(_ context.Context, quota QuotaConfig, expectedVersion uint64) (QuotaConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.byTid[quota.TenantID]

	currentVersion := uint64(0)
	if ok {
		currentVersion = current.ConfigVersion
	}

	if currentVersion != expectedVersion {
		return QuotaConfig{}, ErrQuotaVersionConflict
	}

	quota.ConfigVersion = currentVersion + 1
	s.byTid[quota.TenantID] = quota

	return quota, nil
}
