package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	quotaReconciliationCadence = 15 * time.Minute
	quotaAggregationKeyVersion = "request_id:v1;attempt:v1"
	quotaUsageSourceClickHouse = "clickhouse.request_events"
	quotaLateArrivalMonths     = 3
)

var (
	errQuotaReconciliationSourceNil = errors.New("quota reconciliation source is nil")
	errClickHouseQuotaSourceNil     = errors.New("clickhouse quota usage source is nil")
)

// QuotaUsageEventSource reads durable request_events rows for quota reconciliation.
type QuotaUsageEventSource interface {
	QuotaUsageEvents(ctx context.Context, tenantID string, from, to time.Time) ([]RequestEvent, error)
}

// QuotaConfigSource lists configured quotas for the background reconciliation job.
type QuotaConfigSource interface {
	List(ctx context.Context) ([]QuotaConfig, error)
}

// QuotaUsageSnapshotStore persists reconciled quota totals for audit/invoicing.
type QuotaUsageSnapshotStore interface {
	GetQuotaUsage(ctx context.Context, tenantID, period string) (QuotaUsage, bool, error)
	PutQuotaUsage(ctx context.Context, usage QuotaUsage) error
}

// QuotaUsage is the billing-grade user-visible usage view for a quota period.
type QuotaUsage struct {
	TenantID         string    `json:"tenant_id"`
	Period           string    `json:"period"`
	RequestCount     int64     `json:"request_count"`
	BandwidthBytes   int64     `json:"bandwidth_bytes"`
	AccurateThrough  time.Time `json:"accurate_through"`
	Source           string    `json:"source"`
	AggregationKey   string    `json:"aggregation_key"`
	NextReconcileAt  time.Time `json:"next_reconcile_at"`
	RedisCorrected   bool      `json:"redis_corrected"`
	RedisUnavailable bool      `json:"redis_unavailable"`
}

// QuotaReconciler rebuilds monthly quota usage from durable request_events.
type QuotaReconciler struct {
	source    QuotaUsageEventSource
	redis     redis.Cmdable
	snapshots QuotaUsageSnapshotStore
	now       func() time.Time
}

// NewQuotaReconciler builds a reconciler over a durable event source and optional Redis hot counters.
func NewQuotaReconciler(source QuotaUsageEventSource, redisClient redis.Cmdable, now func() time.Time) *QuotaReconciler {
	if now == nil {
		now = time.Now
	}

	return &QuotaReconciler{source: source, redis: redisClient, now: now}
}

// SetSnapshotStore attaches durable storage for reconciled quota totals.
func (r *QuotaReconciler) SetSnapshotStore(store QuotaUsageSnapshotStore) {
	if r == nil {
		return
	}

	r.snapshots = store
}

// ReconcilePeriod recomputes one tenant/month from durable events and corrects Redis hot counters when configured.
func (r *QuotaReconciler) ReconcilePeriod(ctx context.Context, cfg QuotaConfig, period string) (QuotaUsage, error) {
	if r == nil || r.source == nil {
		return QuotaUsage{}, errQuotaReconciliationSourceNil
	}

	if period == "" {
		period = monthlyPeriodKey(r.now())
	}

	from, to, err := quotaPeriodBounds(period)
	if err != nil {
		return QuotaUsage{}, err
	}

	queryTo := to

	now := r.now().UTC()
	if now.Before(queryTo) {
		queryTo = now
	}

	events, err := r.source.QuotaUsageEvents(ctx, cfg.TenantID, from, queryTo)
	if err != nil {
		return QuotaUsage{}, fmt.Errorf("read quota usage events: %w", err)
	}

	usage := aggregateQuotaUsage(cfg, period, events)
	usage.AccurateThrough = queryTo
	usage.Source = quotaUsageSourceClickHouse
	usage.AggregationKey = quotaAggregationKeyVersion
	usage.NextReconcileAt = r.now().UTC().Add(quotaReconciliationCadence)

	usage.RedisCorrected, usage.RedisUnavailable = r.correctRedis(ctx, cfg.TenantID, period, usage)

	if r.snapshots != nil {
		err = r.snapshots.PutQuotaUsage(ctx, usage)
		if err != nil {
			return QuotaUsage{}, fmt.Errorf("write quota usage snapshot: %w", err)
		}
	}

	return usage, nil
}

// ReconcileConfiguredQuotas reconciles current and recent months for every configured quota.
func (r *QuotaReconciler) ReconcileConfiguredQuotas(ctx context.Context, configs QuotaConfigSource) error {
	if configs == nil {
		return errQuotaReconciliationSourceNil
	}

	quotas, err := configs.List(ctx)
	if err != nil {
		return fmt.Errorf("list quota configs: %w", err)
	}

	for _, quota := range quotas {
		for monthsAgo := range quotaLateArrivalMonths + 1 {
			period := monthlyPeriodKey(r.now().AddDate(0, -monthsAgo, 0))

			_, err = r.ReconcilePeriod(ctx, quota, period)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Run starts periodic quota reconciliation until ctx is canceled.
func (r *QuotaReconciler) Run(ctx context.Context, configs QuotaConfigSource) {
	_ = r.ReconcileConfiguredQuotas(ctx, configs)

	ticker := time.NewTicker(quotaReconciliationCadence)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.ReconcileConfiguredQuotas(ctx, configs)
		}
	}
}

func (r *QuotaReconciler) correctRedis(ctx context.Context, tenantID, period string, usage QuotaUsage) (bool, bool) {
	if r.redis == nil {
		return false, false
	}

	ttl := int64(quotaTTLGraceSeconds)
	if period == monthlyPeriodKey(r.now()) {
		ttl = secondsUntilNextMonth(r.now()) + quotaTTLGraceSeconds
	}

	err := r.redis.Set(ctx, quotaRequestKey(tenantID, period), usage.RequestCount, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		return false, true
	}

	err = r.redis.Set(ctx, quotaBandwidthKey(tenantID, period), usage.BandwidthBytes, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		return false, true
	}

	return true, false
}

func aggregateQuotaUsage(cfg QuotaConfig, period string, events []RequestEvent) QuotaUsage {
	requests := make(map[string]struct{})
	attemptBytes := make(map[string]int64)

	var bandwidth int64

	for _, event := range events {
		if event.TenantID != cfg.TenantID || monthlyPeriodKey(event.Timestamp) != period {
			continue
		}

		if countsAsQuotaRequest(cfg, event) {
			requests[event.RequestID] = struct{}{}
		}

		attemptKey := event.RequestID + ":" + strconv.Itoa(int(event.Attempt))

		bytes := addQuotaBytes(0, event.RequestSizeBytes)
		bytes = addQuotaBytes(bytes, event.ResponseSizeBytes)

		if bytes <= attemptBytes[attemptKey] {
			continue
		}

		attemptBytes[attemptKey] = bytes
	}

	for _, bytes := range attemptBytes {
		if math.MaxInt64-bandwidth < bytes {
			bandwidth = math.MaxInt64

			break
		}

		bandwidth += bytes
	}

	return QuotaUsage{
		TenantID:       cfg.TenantID,
		Period:         period,
		RequestCount:   int64(len(requests)),
		BandwidthBytes: bandwidth,
	}
}

func addQuotaBytes(total int64, n uint64) int64 {
	if n > math.MaxInt64 {
		return math.MaxInt64
	}

	add := int64(n)
	if math.MaxInt64-total < add {
		return math.MaxInt64
	}

	return total + add
}

func countsAsQuotaRequest(cfg QuotaConfig, event RequestEvent) bool {
	if event.RequestID == "" {
		return false
	}

	if cfg.RequestCountPolicy == quotaRequestCountOnSuccess {
		return event.ErrorCode == "" && event.UpstreamStatus > 0
	}

	return true
}

func quotaPeriodBounds(period string) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation("200601", period, time.UTC)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse quota period %q: %w", period, err)
	}

	return from, from.AddDate(0, 1, 0), nil
}

// HTTPClickHouseQuotaUsageSource reads quota usage rows from ClickHouse.
type HTTPClickHouseQuotaUsageSource struct {
	sink *HTTPClickHouseSink
}

// NewHTTPClickHouseQuotaUsageSource builds a quota source over ClickHouse HTTP.
func NewHTTPClickHouseQuotaUsageSource(sink *HTTPClickHouseSink) *HTTPClickHouseQuotaUsageSource {
	return &HTTPClickHouseQuotaUsageSource{sink: sink}
}

// QuotaUsageEvents reads canonical request_events rows for one tenant and time range.
func (s *HTTPClickHouseQuotaUsageSource) QuotaUsageEvents(ctx context.Context, tenantID string, from, to time.Time) ([]RequestEvent, error) {
	if s == nil || s.sink == nil {
		return nil, errClickHouseQuotaSourceNil
	}

	sql := buildQuotaUsageSQL(tenantID, from, to)

	body, err := s.sink.query(ctx, sql)
	if err != nil {
		return nil, err
	}

	defer func() { _ = body.Close() }()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(nil, telemetryScannerMaxBytes)

	var events []RequestEvent

	for scanner.Scan() {
		var event RequestEvent

		err := json.Unmarshal(scanner.Bytes(), &event)
		if err != nil {
			return nil, fmt.Errorf("decode quota usage event: %w", err)
		}

		events = append(events, event)
	}

	err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("read quota usage events: %w", err)
	}

	return events, nil
}

func buildQuotaUsageSQL(tenantID string, from, to time.Time) string {
	var b strings.Builder
	b.WriteString("SELECT timestamp, request_id, tenant_id, attempt, upstream_status, error_code, request_size_bytes, response_size_bytes FROM request_events WHERE tenant_id = ")
	b.WriteString(chQuote(tenantID))
	b.WriteString(" AND timestamp >= ")
	b.WriteString(chTime(from))
	b.WriteString(" AND timestamp < ")
	b.WriteString(chTime(to))
	b.WriteString(" ORDER BY tenant_id, timestamp, request_id, attempt FORMAT JSONEachRow")

	return b.String()
}
