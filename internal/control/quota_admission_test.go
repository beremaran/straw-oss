package control

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestQuotaAdmissionRequestCount(t *testing.T) {
	client := newTestRedisClient(t)
	q := NewQuotaAdmission(client, nil)

	cfg := QuotaConfig{TenantID: "ten_reqcount", MaxRequests: 2, RequestCountPolicy: quotaRequestCountOnAdmission}

	first := q.CheckAdmission(context.Background(), cfg)
	if !first.Allowed {
		t.Fatalf("first CheckAdmission = %+v, want allowed", first)
	}

	second := q.CheckAdmission(context.Background(), cfg)
	if !second.Allowed {
		t.Fatalf("second CheckAdmission = %+v, want allowed", second)
	}

	third := q.CheckAdmission(context.Background(), cfg)
	if third.Allowed || third.Reason != quotaReasonRequestCount {
		t.Fatalf("third CheckAdmission = %+v, want denied with reason request_count", third)
	}
}

func TestQuotaAdmissionCountOnSuccessDoesNotIncrementOnAdmission(t *testing.T) {
	client := newTestRedisClient(t)
	q := NewQuotaAdmission(client, nil)

	cfg := QuotaConfig{TenantID: "ten_success_policy", MaxRequests: 1, RequestCountPolicy: quotaRequestCountOnSuccess}

	// Admission never increments under count_on_success, so it should
	// stay allowed across repeated checks until RecordSuccess is called.
	for i := range 5 {
		d := q.CheckAdmission(context.Background(), cfg)
		if !d.Allowed {
			t.Fatalf("CheckAdmission iteration %d = %+v, want allowed (no increment under count_on_success)", i, d)
		}
	}

	err := q.RecordSuccess(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RecordSuccess() error = %v", err)
	}

	// MaxRequests=1 now reached by the one recorded success.
	d := q.CheckAdmission(context.Background(), cfg)
	if d.Allowed {
		t.Fatalf("CheckAdmission after RecordSuccess = %+v, want denied", d)
	}
}

func TestQuotaAdmissionBandwidthAccounting(t *testing.T) {
	client := newTestRedisClient(t)
	q := NewQuotaAdmission(client, nil)

	tenantID := "ten_bandwidth"
	cfg := QuotaConfig{TenantID: tenantID, MaxBandwidthBytes: 100, RequestCountPolicy: quotaRequestCountOnAdmission}

	// Two partial fallback attempts both count toward the bandwidth quota
	// (docs/planning/20 "Bandwidth Accounting").
	err := q.AddBandwidth(context.Background(), tenantID, 60)
	if err != nil {
		t.Fatalf("AddBandwidth(60) error = %v", err)
	}

	err = q.AddBandwidth(context.Background(), tenantID, 60)
	if err != nil {
		t.Fatalf("AddBandwidth(60) error = %v", err)
	}

	_, bw, err := q.Usage(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Usage() error = %v", err)
	}

	if bw != 120 {
		t.Fatalf("bandwidth usage = %d, want 120", bw)
	}

	d := q.CheckAdmission(context.Background(), cfg)
	if d.Allowed || d.Reason != quotaReasonBandwidth {
		t.Fatalf("CheckAdmission after bandwidth exhausted = %+v, want denied with reason bandwidth", d)
	}
}

func TestQuotaAdmissionRedisFailurePolicy(t *testing.T) {
	unreachable := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = unreachable.Close() })

	q := NewQuotaAdmission(unreachable, nil)

	openCfg := QuotaConfig{TenantID: "ten_fail", MaxRequests: 1, RedisFailPolicy: "open"}

	openDecision := q.CheckAdmission(context.Background(), openCfg)
	if !openDecision.Allowed || !openDecision.RedisFailure {
		t.Fatalf("fail-open CheckAdmission = %+v, want Allowed=true RedisFailure=true", openDecision)
	}

	closedCfg := openCfg
	closedCfg.RedisFailPolicy = "closed"

	closedDecision := q.CheckAdmission(context.Background(), closedCfg)
	if closedDecision.Allowed || !closedDecision.RedisFailure {
		t.Fatalf("fail-closed CheckAdmission = %+v, want Allowed=false RedisFailure=true", closedDecision)
	}
}

// TestQuotaAdmissionNotBillingGrade documents the P0 boundary
// (docs/planning/20, docs/planning/33 "Quota Accuracy"): quota counters are
// operational Redis hot counters with no durable reconciliation. A lost
// Redis key silently resets usage to zero rather than being repaired from
// ClickHouse, proving P0 does not claim billing-grade accuracy.
func TestQuotaAdmissionNotBillingGrade(t *testing.T) {
	client := newTestRedisClient(t)
	q := NewQuotaAdmission(client, nil)

	tenantID := "ten_not_billing_grade"
	cfg := QuotaConfig{TenantID: tenantID, MaxRequests: 1, RequestCountPolicy: quotaRequestCountOnAdmission}

	first := q.CheckAdmission(context.Background(), cfg)
	if !first.Allowed {
		t.Fatalf("first CheckAdmission = %+v, want allowed", first)
	}

	second := q.CheckAdmission(context.Background(), cfg)
	if second.Allowed {
		t.Fatalf("second CheckAdmission = %+v, want denied (quota reached)", second)
	}

	// Simulate Redis counter loss: no reconciliation from a durable source
	// happens, so usage silently resets and admission reopens.
	requestKey := quotaRequestKey(tenantID, monthlyPeriodKey(time.Now()))

	err := client.Del(context.Background(), requestKey).Err()
	if err != nil {
		t.Fatalf("Del() error = %v", err)
	}

	third := q.CheckAdmission(context.Background(), cfg)
	if !third.Allowed {
		t.Fatalf("CheckAdmission after simulated Redis loss = %+v, want allowed (no reconciliation, not billing-grade)", third)
	}
}

func TestQuotaKeysHaveTTL(t *testing.T) {
	client := newTestRedisClient(t)
	q := NewQuotaAdmission(client, nil)

	tenantID := "ten_ttl"
	cfg := QuotaConfig{TenantID: tenantID, MaxRequests: 100, RequestCountPolicy: quotaRequestCountOnAdmission}

	_ = q.CheckAdmission(context.Background(), cfg)

	err := q.AddBandwidth(context.Background(), tenantID, 10)
	if err != nil {
		t.Fatalf("AddBandwidth() error = %v", err)
	}

	period := monthlyPeriodKey(time.Now())

	for _, key := range []string{quotaRequestKey(tenantID, period), quotaBandwidthKey(tenantID, period)} {
		ttl, err := client.TTL(context.Background(), key).Result()
		if err != nil {
			t.Fatalf("TTL(%s) error = %v", key, err)
		}

		if ttl <= 0 {
			t.Fatalf("TTL(%s) = %v, want > 0 (every Redis key must have a TTL)", key, ttl)
		}
	}
}
