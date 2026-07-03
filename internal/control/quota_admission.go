package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	quotaOpTimeout = 500 * time.Millisecond
	// quotaTTLGraceSeconds extends each quota hot-counter key's TTL past
	// its monthly period boundary so a slow clock or clock skew across
	// Control replicas cannot expire a key mid-period.
	quotaTTLGraceSeconds = 86400

	quotaReasonRequestCount = "request_count"
	quotaReasonBandwidth    = "bandwidth"

	quotaRequestCountOnSuccess   = "count_on_success"
	quotaRequestCountOnAdmission = "count_on_admission"
)

// QuotaDecision is the outcome of one quota admission check
// (docs/planning/20 "Quotas": operational admission control, not
// billing-grade accounting).
type QuotaDecision struct {
	Allowed bool
	// Reason is quotaReasonRequestCount or quotaReasonBandwidth when denied.
	Reason string
	// RedisFailure is true when the decision came from cfg.RedisFailPolicy
	// rather than a real counter check.
	RedisFailure bool
}

// QuotaAdmission enforces monthly request-count and bandwidth quotas using
// Redis fixed-window counters (docs/planning/20). Every key it writes
// carries a TTL tied to the monthly period boundary.
type QuotaAdmission struct {
	client redis.Cmdable
	now    func() time.Time
}

// NewQuotaAdmission builds a QuotaAdmission. now may be nil (defaults to
// time.Now).
func NewQuotaAdmission(client redis.Cmdable, now func() time.Time) *QuotaAdmission {
	if now == nil {
		now = time.Now
	}

	return &QuotaAdmission{client: client, now: now}
}

func monthlyPeriodKey(t time.Time) string {
	return t.UTC().Format("200601")
}

func secondsUntilNextMonth(t time.Time) int64 {
	t = t.UTC()
	nextMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)

	return int64(nextMonth.Sub(t).Seconds())
}

func quotaRequestKey(tenantID, period string) string {
	return fmt.Sprintf("straw:quota:%s:%s:requests", tenantID, period)
}

func quotaBandwidthKey(tenantID, period string) string {
	return fmt.Sprintf("straw:quota:%s:%s:bandwidth", tenantID, period)
}

// quotaAdmissionScript checks the bandwidth counter (already-transferred
// bytes this period) and the request-count counter against cfg's limits,
// then conditionally increments the request counter in the same atomic
// step (docs/planning/20 "Quota Accounting: Request vs Attempt Semantics").
//
// KEYS[1] = request-count key, KEYS[2] = bandwidth key.
// ARGV: max_requests, max_bandwidth, increment_now ("0"|"1"), ttl_seconds.
// Returns {allowed(0|1), reason}.
const quotaAdmissionScript = `
local reqkey = KEYS[1]
local bwkey = KEYS[2]
local max_requests = tonumber(ARGV[1])
local max_bandwidth = tonumber(ARGV[2])
local increment_now = ARGV[3]
local ttl = tonumber(ARGV[4])

local bw = tonumber(redis.call('GET', bwkey) or '0')
if max_bandwidth > 0 and bw >= max_bandwidth then
  return {0, 'bandwidth'}
end

local count = tonumber(redis.call('GET', reqkey) or '0')
if max_requests > 0 and count >= max_requests then
  return {0, 'request_count'}
end

if increment_now == '1' then
  local newcount = redis.call('INCR', reqkey)
  if newcount == 1 then
    redis.call('EXPIRE', reqkey, ttl)
  end
end

return {1, ''}
`

// incrByScript atomically increments a counter by n and sets its TTL only
// on first creation, so a refreshed TTL never resets an in-progress period.
const incrByScript = `
local exists = redis.call('EXISTS', KEYS[1])
local v = redis.call('INCRBY', KEYS[1], ARGV[1])
if exists == 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return v
`

// CheckAdmission evaluates whether a new request is admitted under cfg's
// monthly quota. When cfg.RequestCountPolicy is "count_on_admission" (the
// P0 default), it increments the request-count hot counter immediately;
// "count_on_success" callers must call RecordSuccess after a successful
// upstream transport instead. On Redis failure it applies
// cfg.RedisFailPolicy.
func (q *QuotaAdmission) CheckAdmission(ctx context.Context, cfg QuotaConfig) QuotaDecision {
	opCtx, cancel := context.WithTimeout(ctx, quotaOpTimeout)
	defer cancel()

	period := monthlyPeriodKey(q.now())
	ttl := secondsUntilNextMonth(q.now()) + quotaTTLGraceSeconds

	incrementNow := "1"
	if cfg.RequestCountPolicy == quotaRequestCountOnSuccess {
		incrementNow = "0"
	}

	res, err := q.client.Eval(opCtx, quotaAdmissionScript,
		[]string{quotaRequestKey(cfg.TenantID, period), quotaBandwidthKey(cfg.TenantID, period)},
		cfg.MaxRequests, cfg.MaxBandwidthBytes, incrementNow, ttl,
	).Result()
	if err != nil {
		return q.failureDecision(cfg)
	}

	values, ok := res.([]any)
	if !ok || len(values) != 2 {
		return q.failureDecision(cfg)
	}

	reason, _ := values[1].(string)

	return QuotaDecision{Allowed: toInt64(values[0]) == 1, Reason: reason}
}

// RecordSuccess increments the request-count counter for
// "count_on_success" quota policy after a successful upstream transport. It
// is a no-op for "count_on_admission" policy, which already incremented in
// CheckAdmission.
func (q *QuotaAdmission) RecordSuccess(ctx context.Context, cfg QuotaConfig) error {
	if cfg.RequestCountPolicy != quotaRequestCountOnSuccess {
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, quotaOpTimeout)
	defer cancel()

	period := monthlyPeriodKey(q.now())
	ttl := secondsUntilNextMonth(q.now()) + quotaTTLGraceSeconds

	err := q.client.Eval(opCtx, incrByScript, []string{quotaRequestKey(cfg.TenantID, period)}, 1, ttl).Err()
	if err != nil {
		return fmt.Errorf("record quota success: %w", err)
	}

	return nil
}

// AddBandwidth increments the bandwidth hot counter by n bytes, accounting
// for bytes actually transferred per attempt (docs/planning/20 "Bandwidth
// Accounting": both partial fallback attempts count toward the quota).
func (q *QuotaAdmission) AddBandwidth(ctx context.Context, tenantID string, n int64) error {
	if n <= 0 {
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, quotaOpTimeout)
	defer cancel()

	period := monthlyPeriodKey(q.now())
	ttl := secondsUntilNextMonth(q.now()) + quotaTTLGraceSeconds

	err := q.client.Eval(opCtx, incrByScript, []string{quotaBandwidthKey(tenantID, period)}, n, ttl).Err()
	if err != nil {
		return fmt.Errorf("add quota bandwidth: %w", err)
	}

	return nil
}

// Usage returns the current hot-counter values for tenantID's active
// period, for GET /quotas display. This is an operational snapshot, not a
// billing-grade reconciled total (docs/planning/20).
func (q *QuotaAdmission) Usage(ctx context.Context, tenantID string) (int64, int64, error) {
	opCtx, cancel := context.WithTimeout(ctx, quotaOpTimeout)
	defer cancel()

	period := monthlyPeriodKey(q.now())

	requestCount, err := getInt64(opCtx, q.client, quotaRequestKey(tenantID, period))
	if err != nil {
		return 0, 0, err
	}

	bandwidthBytes, err := getInt64(opCtx, q.client, quotaBandwidthKey(tenantID, period))
	if err != nil {
		return 0, 0, err
	}

	return requestCount, bandwidthBytes, nil
}

func (q *QuotaAdmission) failureDecision(cfg QuotaConfig) QuotaDecision {
	return QuotaDecision{Allowed: cfg.RedisFailPolicy != "closed", RedisFailure: true}
}

func getInt64(ctx context.Context, client redis.Cmdable, key string) (int64, error) {
	v, err := client.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}

		return 0, fmt.Errorf("get %s: %w", key, err)
	}

	return v, nil
}
