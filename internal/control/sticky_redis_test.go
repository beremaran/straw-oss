package control

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisStickyStoreSetGetRefresh(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisStickyStore(client)

	_, ok := store.Get("ten_a", "sticky-1")
	if ok {
		t.Fatalf("Get() before Set = found, want not found")
	}

	store.Set("ten_a", "sticky-1", "worker_1", time.Minute)

	worker, ok := store.Get("ten_a", "sticky-1")
	if !ok || worker != "worker_1" {
		t.Fatalf("Get() after Set = (%q, %v), want (worker_1, true)", worker, ok)
	}

	store.Refresh("ten_a", "sticky-1", "worker_1", time.Minute)

	worker, ok = store.Get("ten_a", "sticky-1")
	if !ok || worker != "worker_1" {
		t.Fatalf("Get() after Refresh = (%q, %v), want (worker_1, true)", worker, ok)
	}
}

func TestRedisStickyStoreTTLExpiry(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisStickyStore(client)

	store.Set("ten_b", "sticky-2", "worker_2", 50*time.Millisecond)

	worker, ok := store.Get("ten_b", "sticky-2")
	if !ok || worker != "worker_2" {
		t.Fatalf("Get() immediately after Set = (%q, %v), want (worker_2, true)", worker, ok)
	}

	time.Sleep(150 * time.Millisecond)

	_, ok = store.Get("ten_b", "sticky-2")
	if ok {
		t.Fatalf("Get() after TTL expiry = found, want not found")
	}
}

// TestRedisStickyStoreKeyStructure proves the store uses the canonical key
// shape from docs/planning/10-routing-model.md:
// straw:sticky:<tenant_id>:<sticky_session_id>.
func TestRedisStickyStoreKeyStructure(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisStickyStore(client)

	store.Set("ten_c", "sticky-3", "worker_3", time.Minute)

	v, err := client.Get(t.Context(), "straw:sticky:ten_c:sticky-3").Result()
	if err != nil {
		t.Fatalf("direct Get() error = %v", err)
	}

	if v != "worker_3" {
		t.Fatalf("direct Get() = %q, want worker_3", v)
	}
}

// TestRedisStickyStoreDegradesOnRedisFailure proves the documented sticky
// fail policy (docs/planning/20 "Sticky sessions: degrade according to
// route policy... may fail sticky requests"): a Redis outage makes Get
// report "no pin found" rather than erroring, so the router's existing
// allow_sticky_fallback logic decides the outcome.
func TestRedisStickyStoreDegradesOnRedisFailure(t *testing.T) {
	unreachable := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = unreachable.Close() })

	store := NewRedisStickyStore(unreachable)

	_, ok := store.Get("ten_d", "sticky-4")
	if ok {
		t.Fatalf("Get() during Redis outage = found, want degraded to not-found")
	}

	// Set/Refresh must not panic or block indefinitely during an outage.
	store.Set("ten_d", "sticky-4", "worker_4", time.Minute)
	store.Refresh("ten_d", "sticky-4", "worker_4", time.Minute)
}

// TestRouterUsesRedisStickyStore proves Router works unmodified against the
// Redis-backed StickyBackend implementation, not just the in-process
// StickyStore (routing.go NewRouter accepts the StickyBackend interface).
const testStickyRouterTenant = "ten_sticky_router"

func TestRouterUsesRedisStickyStore(t *testing.T) {
	client := newTestRedisClient(t)
	sticky := NewRedisStickyStore(client)

	rule := RoutingRule{
		ID: "route_1", TenantID: testStickyRouterTenant, Priority: 1, Enabled: true,
		TargetPoolID: "pool_1", StickySessionTTLSeconds: 60,
	}
	rules := NewStaticRuleProvider([]RoutingRule{rule})
	pools := NewStaticPoolPolicyProvider(nil)
	candidates := &fakeCandidateSource{
		candidates: []PoolCandidate{
			{WorkerID: "worker_a", SessionID: "sess_a", AssignSubject: "subj_a", MaxConcurrency: 10},
			{WorkerID: "worker_b", SessionID: "sess_b", AssignSubject: "subj_b", MaxConcurrency: 10},
		},
	}

	router := NewRouter(rules, pools, candidates, sticky, nil)

	first := router.Evaluate(RouteRequest{TenantID: testStickyRouterTenant, StickySessionID: "sticky-router-1"})
	if !first.OK || !first.Sticky {
		t.Fatalf("first Evaluate() = %+v, want sticky OK", first)
	}

	second := router.Evaluate(RouteRequest{TenantID: testStickyRouterTenant, StickySessionID: "sticky-router-1"})
	if !second.OK || second.WorkerID != first.WorkerID {
		t.Fatalf("second Evaluate() = %+v, want pinned to %s", second, first.WorkerID)
	}
}

type fakeCandidateSource struct {
	candidates []PoolCandidate
}

func (f *fakeCandidateSource) CandidatesForPool(_, _ string) []PoolCandidate {
	return f.candidates
}
