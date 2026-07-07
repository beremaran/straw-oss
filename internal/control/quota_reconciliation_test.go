package control

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeQuotaUsageSource struct {
	events []RequestEvent
}

type quotaUsageRange struct {
	from time.Time
	to   time.Time
}

type recordingQuotaUsageSource struct {
	events []RequestEvent
	ranges []quotaUsageRange
}

type recordingQuotaUsageSnapshotStore struct {
	usages []QuotaUsage
}

const (
	quotaReconciliationTestRequestID = "req_1"
	quotaReconciliationTenantA       = "ten_a"
	quotaReconciliationTenantLate    = "ten_late"
	quotaReconciliationTenantSuccess = "ten_success"
)

func (s fakeQuotaUsageSource) QuotaUsageEvents(_ context.Context, _ string, _, _ time.Time) ([]RequestEvent, error) {
	return s.events, nil
}

func (s *recordingQuotaUsageSource) QuotaUsageEvents(_ context.Context, _ string, from, to time.Time) ([]RequestEvent, error) {
	s.ranges = append(s.ranges, quotaUsageRange{from: from, to: to})

	return s.events, nil
}

func (s *recordingQuotaUsageSnapshotStore) PutQuotaUsage(_ context.Context, usage QuotaUsage) error {
	s.usages = append(s.usages, usage)

	return nil
}

func (s *recordingQuotaUsageSnapshotStore) GetQuotaUsage(_ context.Context, tenantID, period string) (QuotaUsage, bool, error) {
	for _, usage := range s.usages {
		if usage.TenantID == tenantID && usage.Period == period {
			return usage, true, nil
		}
	}

	return QuotaUsage{}, false, nil
}

func TestQuotaReconciliationAggregatesIdempotently(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	snapshots := &recordingQuotaUsageSnapshotStore{}
	source := &recordingQuotaUsageSource{events: []RequestEvent{
		{Timestamp: now, TenantID: quotaReconciliationTenantA, RequestID: quotaReconciliationTestRequestID, Attempt: 1, UpstreamStatus: 200, RequestSizeBytes: 10, ResponseSizeBytes: 100},
		{Timestamp: now, TenantID: quotaReconciliationTenantA, RequestID: quotaReconciliationTestRequestID, Attempt: 1, UpstreamStatus: 200, RequestSizeBytes: 10, ResponseSizeBytes: 90},
		{Timestamp: now, TenantID: quotaReconciliationTenantA, RequestID: quotaReconciliationTestRequestID, Attempt: 2, UpstreamStatus: 502, ErrorCode: "upstream_5xx", RequestSizeBytes: 10, ResponseSizeBytes: 20},
		{Timestamp: now, TenantID: quotaReconciliationTenantA, RequestID: "req_2", Attempt: 1, UpstreamStatus: 201, RequestSizeBytes: 5, ResponseSizeBytes: 50},
	}}
	reconciler := NewQuotaReconciler(source, nil, func() time.Time { return now })
	reconciler.SetSnapshotStore(snapshots)

	usage, err := reconciler.ReconcilePeriod(context.Background(), QuotaConfig{TenantID: quotaReconciliationTenantA}, "202607")
	if err != nil {
		t.Fatalf("ReconcilePeriod() error = %v", err)
	}

	if usage.RequestCount != 2 {
		t.Fatalf("RequestCount = %d, want 2 unique external requests", usage.RequestCount)
	}
	if usage.BandwidthBytes != 195 {
		t.Fatalf("BandwidthBytes = %d, want 195 unique attempt bytes", usage.BandwidthBytes)
	}
	if usage.Source != quotaUsageSourceClickHouse || usage.NextReconcileAt.Sub(now) != quotaReconciliationCadence {
		t.Fatalf("display fields = %+v, want source and next cadence", usage)
	}
	if !usage.AccurateThrough.Equal(now) {
		t.Fatalf("AccurateThrough = %s, want current reconciliation cutoff %s", usage.AccurateThrough, now)
	}
	if len(source.ranges) != 1 || !source.ranges[0].to.Equal(now) {
		t.Fatalf("source query range = %+v, want in-progress period capped at now", source.ranges)
	}
	if len(snapshots.usages) != 1 || snapshots.usages[0].AggregationKey != quotaAggregationKeyVersion {
		t.Fatalf("snapshots = %+v, want one durable snapshot with aggregation key", snapshots.usages)
	}
}

func TestQuotaReconciliationHandlesLateEventsByRecomputingPeriod(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	source := fakeQuotaUsageSource{events: []RequestEvent{
		{Timestamp: time.Date(2026, 7, 1, 0, 0, 1, 0, time.UTC), TenantID: quotaReconciliationTenantLate, RequestID: "early", Attempt: 1, UpstreamStatus: 200},
		{Timestamp: time.Date(2026, 7, 19, 23, 59, 0, 0, time.UTC), TenantID: quotaReconciliationTenantLate, RequestID: "late", Attempt: 1, UpstreamStatus: 200},
		{Timestamp: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), TenantID: quotaReconciliationTenantLate, RequestID: "next", Attempt: 1, UpstreamStatus: 200},
	}}
	reconciler := NewQuotaReconciler(source, nil, func() time.Time { return now })

	usage, err := reconciler.ReconcilePeriod(context.Background(), QuotaConfig{TenantID: quotaReconciliationTenantLate}, "202607")
	if err != nil {
		t.Fatalf("ReconcilePeriod() error = %v", err)
	}

	if usage.RequestCount != 2 {
		t.Fatalf("RequestCount = %d, want 2 July requests including late arrival", usage.RequestCount)
	}
}

func TestQuotaReconciliationPreservesCountOnSuccess(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	source := fakeQuotaUsageSource{events: []RequestEvent{
		{Timestamp: now, TenantID: quotaReconciliationTenantSuccess, RequestID: "ok", Attempt: 1, UpstreamStatus: 200},
		{Timestamp: now, TenantID: quotaReconciliationTenantSuccess, RequestID: "upstream_500", Attempt: 1, UpstreamStatus: 500},
		{Timestamp: now, TenantID: quotaReconciliationTenantSuccess, RequestID: "failed", Attempt: 1, ErrorCode: "upstream_timeout"},
	}}
	reconciler := NewQuotaReconciler(source, nil, func() time.Time { return now })

	usage, err := reconciler.ReconcilePeriod(context.Background(), QuotaConfig{TenantID: quotaReconciliationTenantSuccess, RequestCountPolicy: quotaRequestCountOnSuccess}, "202607")
	if err != nil {
		t.Fatalf("ReconcilePeriod() error = %v", err)
	}

	if usage.RequestCount != 2 {
		t.Fatalf("RequestCount = %d, want completed upstream transports, including HTTP 500 responses", usage.RequestCount)
	}
}

func TestQuotaReconciliationCorrectsRedisHotCounters(t *testing.T) {
	client := newTestRedisClient(t)
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	tenantID := "ten_reconcile_redis"
	source := fakeQuotaUsageSource{events: []RequestEvent{
		{Timestamp: now, TenantID: tenantID, RequestID: "req_1", Attempt: 1, UpstreamStatus: 200, RequestSizeBytes: 4, ResponseSizeBytes: 6},
	}}
	reconciler := NewQuotaReconciler(source, client, func() time.Time { return now })

	err := client.Del(context.Background(), quotaRequestKey(tenantID, "202607"), quotaBandwidthKey(tenantID, "202607")).Err()
	if err != nil {
		t.Fatalf("Del() error = %v", err)
	}

	usage, err := reconciler.ReconcilePeriod(context.Background(), QuotaConfig{TenantID: tenantID}, "202607")
	if err != nil {
		t.Fatalf("ReconcilePeriod() error = %v", err)
	}

	if !usage.RedisCorrected || usage.RedisUnavailable {
		t.Fatalf("redis correction flags = %+v, want corrected without outage", usage)
	}

	requests, bandwidth, err := NewQuotaAdmission(client, func() time.Time { return now }).Usage(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Usage() error = %v", err)
	}
	if requests != 1 || bandwidth != 10 {
		t.Fatalf("redis usage = %d/%d, want 1/10 after reconciliation", requests, bandwidth)
	}
}

func TestQuotaReconciliationJobReconcilesRecentPeriods(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	source := &recordingQuotaUsageSource{}
	store := NewInMemoryQuotaStore()

	_, err := store.Put(context.Background(), QuotaConfig{TenantID: "ten_job", Period: quotaPeriodMonthly}, 0)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	reconciler := NewQuotaReconciler(source, nil, func() time.Time { return now })

	err = reconciler.ReconcileConfiguredQuotas(context.Background(), store)
	if err != nil {
		t.Fatalf("ReconcileConfiguredQuotas() error = %v", err)
	}

	if len(source.ranges) != quotaLateArrivalMonths+1 {
		t.Fatalf("source ranges = %d, want current plus late-arrival lookback", len(source.ranges))
	}
	if !source.ranges[0].from.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) || !source.ranges[0].to.Equal(now) {
		t.Fatalf("current range = %+v, want July capped at now", source.ranges[0])
	}
	if !source.ranges[3].from.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)) || !source.ranges[3].to.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("oldest lookback range = %+v, want full April", source.ranges[3])
	}
}

func TestBuildQuotaUsageSQL(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	sql := buildQuotaUsageSQL("ten'ant", from, to)

	for _, want := range []string{
		"FROM request_events",
		"tenant_id = 'ten\\'ant'",
		"parseDateTime64BestEffort('2026-07-01T00:00:00Z')",
		"ORDER BY tenant_id, timestamp, request_id, attempt",
		"FORMAT JSONEachRow",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL %q missing %q", sql, want)
		}
	}
}
