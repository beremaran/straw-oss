package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	client := newTestRedisClient(t)
	rl := NewRateLimiter(client, DefaultRateLimitGuardrails(), nil)

	rule := RateLimitRule{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 3, FailPolicy: RateLimitFailOpen}

	for i := range 3 {
		d := rl.Allow(context.Background(), "ten_a", rule)
		if !d.Allowed {
			t.Fatalf("request %d: Allow = %+v, want allowed", i, d)
		}
	}
}

func TestRateLimiterDeniesOverLimitWithRetryAfter(t *testing.T) {
	client := newTestRedisClient(t)
	rl := NewRateLimiter(client, DefaultRateLimitGuardrails(), nil)

	rule := RateLimitRule{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 2, FailPolicy: RateLimitFailOpen}

	for i := range 2 {
		d := rl.Allow(context.Background(), "ten_b", rule)
		if !d.Allowed {
			t.Fatalf("warmup request %d denied unexpectedly: %+v", i, d)
		}
	}

	d := rl.Allow(context.Background(), "ten_b", rule)
	if d.Allowed {
		t.Fatalf("Allow over limit = %+v, want denied", d)
	}

	if d.RetryAfterMs <= 0 || d.RetryAfterMs > 60000 {
		t.Fatalf("RetryAfterMs = %d, want in (0, 60000]", d.RetryAfterMs)
	}
}

// TestRateLimiterDimensionsAreIndependent proves the tenant, api_key,
// target_host, and ip_type dimensions (docs/planning/20) are tracked under
// independent keys: exhausting one dimension does not affect another.
func TestRateLimiterDimensionsAreIndependent(t *testing.T) {
	client := newTestRedisClient(t)
	rl := NewRateLimiter(client, DefaultRateLimitGuardrails(), nil)

	dims := []RateLimitRule{
		{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 1, FailPolicy: RateLimitFailOpen},
		{Dimension: RateLimitDimAPIKey, Key: "key_1", WindowSeconds: 60, MaxRequests: 1, FailPolicy: RateLimitFailOpen},
		{Dimension: RateLimitDimTargetHost, Key: testExampleHost, WindowSeconds: 60, MaxRequests: 1, FailPolicy: RateLimitFailOpen},
		{Dimension: RateLimitDimIPType, Key: "datacenter", WindowSeconds: 60, MaxRequests: 1, FailPolicy: RateLimitFailOpen},
	}

	for _, rule := range dims {
		first := rl.Allow(context.Background(), "ten_dims", rule)
		if !first.Allowed {
			t.Fatalf("dimension %s: first Allow = %+v, want allowed", rule.Dimension, first)
		}
	}

	// Each dimension independently exhausted its own MaxRequests=1 budget;
	// a second call on any one of them must now deny without affecting the
	// others (already proven allowed above).
	for _, rule := range dims {
		second := rl.Allow(context.Background(), "ten_dims", rule)
		if second.Allowed {
			t.Fatalf("dimension %s: second Allow = %+v, want denied", rule.Dimension, second)
		}
	}
}

func TestRateLimiterMemoryGuardrailFallback(t *testing.T) {
	client := newTestRedisClient(t)
	rl := NewRateLimiter(client, RateLimitGuardrails{MaxEntriesPerKey: 2, MaxKeysPerTenant: 1000}, nil)

	// MaxRequests is set high so the memory guardrail (max_entries=2), not
	// the request limit, is what trips the deny.
	rule := RateLimitRule{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 1000, FailPolicy: RateLimitFailOpen}

	for i := range 2 {
		d := rl.Allow(context.Background(), "ten_guard", rule)
		if !d.Allowed {
			t.Fatalf("warmup request %d denied unexpectedly: %+v", i, d)
		}
	}

	d := rl.Allow(context.Background(), "ten_guard", rule)
	if d.Allowed {
		t.Fatalf("Allow after memory guardrail exceeded = %+v, want conservative deny", d)
	}

	// The guard marker denies subsequent requests for this key until the
	// window expires, even though max_requests was never reached.
	d2 := rl.Allow(context.Background(), "ten_guard", rule)
	if d2.Allowed {
		t.Fatalf("Allow while guard marker active = %+v, want denied", d2)
	}
}

func TestRateLimiterKeyBudgetFallsBackToTenantDimension(t *testing.T) {
	client := newTestRedisClient(t)
	rl := NewRateLimiter(client, RateLimitGuardrails{MaxEntriesPerKey: 10000, MaxKeysPerTenant: 1}, nil)

	if !rl.WithinKeyBudget(context.Background(), "ten_budget", "target_host:a.example.com") {
		t.Fatalf("first key: WithinKeyBudget = false, want true")
	}

	// Re-checking the same key is always within budget (already tracked).
	if !rl.WithinKeyBudget(context.Background(), "ten_budget", "target_host:a.example.com") {
		t.Fatalf("first key repeat: WithinKeyBudget = false, want true")
	}

	if rl.WithinKeyBudget(context.Background(), "ten_budget", "target_host:b.example.com") {
		t.Fatalf("second distinct key: WithinKeyBudget = true, want false (budget=1 exceeded)")
	}
}

func TestRateLimiterRedisFailurePolicy(t *testing.T) {
	// Point at an address nothing is listening on so every command fails
	// fast without waiting for a connect timeout.
	unreachable := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = unreachable.Close() })

	rl := NewRateLimiter(unreachable, DefaultRateLimitGuardrails(), nil)

	openRule := RateLimitRule{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 1, FailPolicy: RateLimitFailOpen}

	openDecision := rl.Allow(context.Background(), "ten_fail", openRule)
	if !openDecision.Allowed || !openDecision.RedisFailure {
		t.Fatalf("fail-open Allow = %+v, want Allowed=true RedisFailure=true", openDecision)
	}

	closedRule := openRule
	closedRule.FailPolicy = RateLimitFailClosed

	closedDecision := rl.Allow(context.Background(), "ten_fail", closedRule)
	if closedDecision.Allowed || !closedDecision.RedisFailure {
		t.Fatalf("fail-closed Allow = %+v, want Allowed=false RedisFailure=true", closedDecision)
	}
}

func TestRateLimitCeilingRejectsExceedingValues(t *testing.T) {
	store := NewInMemoryRateLimitConfigStore()
	ceiling := &RateLimitCeiling{WindowSeconds: 60, MaxRequests: 600} // 10 req/s

	// 700 req / 60s = ~11.6 req/s, above the 10 req/s ceiling.
	cfg := RateLimitConfig{
		TenantID: "ten_ceiling",
		Limits:   []RateLimitRule{{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 700, FailPolicy: RateLimitFailOpen}},
	}

	_, err := store.Put(context.Background(), cfg, 0, ceiling)
	if err == nil {
		t.Fatalf("Put() error = nil, want ErrRateLimitCeilingExceeded")
	}

	if !errors.Is(err, ErrRateLimitCeilingExceeded) {
		t.Fatalf("Put() error = %v, want ErrRateLimitCeilingExceeded", err)
	}
}

func TestRateLimitCeilingAllowsWithinValues(t *testing.T) {
	store := NewInMemoryRateLimitConfigStore()
	ceiling := &RateLimitCeiling{WindowSeconds: 60, MaxRequests: 600}

	cfg := RateLimitConfig{
		TenantID: "ten_ceiling_ok",
		Limits:   []RateLimitRule{{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 600, FailPolicy: RateLimitFailOpen}},
	}

	saved, err := store.Put(context.Background(), cfg, 0, ceiling)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	if saved.ConfigVersion != 1 {
		t.Fatalf("ConfigVersion = %d, want 1", saved.ConfigVersion)
	}
}

func TestRateLimitCeilingNilMeansUnbounded(t *testing.T) {
	store := NewInMemoryRateLimitConfigStore()

	cfg := RateLimitConfig{
		TenantID: "ten_no_ceiling",
		Limits:   []RateLimitRule{{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 1, MaxRequests: 1000000, FailPolicy: RateLimitFailOpen}},
	}

	_, err := store.Put(context.Background(), cfg, 0, nil)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil (nil ceiling is unbounded)", err)
	}
}

func TestRateLimitConfigVersionConflict(t *testing.T) {
	store := NewInMemoryRateLimitConfigStore()

	cfg := RateLimitConfig{TenantID: "ten_conflict", Limits: nil}

	_, err := store.Put(context.Background(), cfg, 5, nil)
	if !errors.Is(err, ErrRateLimitVersionConflict) {
		t.Fatalf("Put() error = %v, want ErrRateLimitVersionConflict", err)
	}
}

func TestRateLimitAdmissionDeniesWorstDimension(t *testing.T) {
	client := newTestRedisClient(t)
	limiter := NewRateLimiter(client, DefaultRateLimitGuardrails(), nil)
	admission := NewRateLimitAdmission(limiter)

	cfg := RateLimitConfig{
		TenantID: "ten_admission",
		Limits: []RateLimitRule{
			{Dimension: RateLimitDimTenant, Key: "*", WindowSeconds: 60, MaxRequests: 1000, FailPolicy: RateLimitFailOpen},
			{Dimension: RateLimitDimAPIKey, Key: "*", WindowSeconds: 60, MaxRequests: 1, FailPolicy: RateLimitFailOpen},
		},
	}

	req := RateLimitRequest{TenantID: "ten_admission", APIKeyID: "key_abc"}

	first := admission.Check(context.Background(), cfg, req)
	if !first.Allowed {
		t.Fatalf("first Check() = %+v, want allowed", first)
	}

	second := admission.Check(context.Background(), cfg, req)
	if second.Allowed || second.Dimension != RateLimitDimAPIKey {
		t.Fatalf("second Check() = %+v, want denied on api_key dimension", second)
	}
}

func TestErrorResponseFromCodeWithRetryOmitsZero(t *testing.T) {
	resp := ErrorResponseFromCodeWithRetry(RateLimitExceeded, "req_1", nil, 0)
	if resp.RetryAfterMs != 0 {
		t.Fatalf("RetryAfterMs = %d, want 0", resp.RetryAfterMs)
	}

	resp2 := ErrorResponseFromCodeWithRetry(RateLimitExceeded, "req_1", nil, 1500)
	if resp2.RetryAfterMs != 1500 {
		t.Fatalf("RetryAfterMs = %d, want 1500", resp2.RetryAfterMs)
	}

	if resp2.Code != errorCodeRateLimitExceeded {
		t.Fatalf("Code = %q, want %q", resp2.Code, errorCodeRateLimitExceeded)
	}
}
