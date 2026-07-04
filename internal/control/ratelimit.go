package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimitDimension is one of the P0 rate-limit dimensions
// (docs/planning/20-rate-limits-and-quotas.md).
type RateLimitDimension string

const (
	// RateLimitDimTenant is the tenant-wide dimension; its key is always "*".
	RateLimitDimTenant RateLimitDimension = "tenant"
	// RateLimitDimAPIKey scopes the limit to one tenant API key.
	RateLimitDimAPIKey RateLimitDimension = "api_key"
	// RateLimitDimTargetHost scopes the limit to one upstream host.
	RateLimitDimTargetHost RateLimitDimension = "target_host"
	// RateLimitDimIPType scopes the limit to one egress IP type.
	RateLimitDimIPType RateLimitDimension = "ip_type"
)

// RateLimitFailPolicy controls admission behavior when Redis is unavailable
// (docs/planning/20 "Redis Failure Policy").
type RateLimitFailPolicy string

const (
	// RateLimitFailOpen admits requests when Redis is unreachable.
	RateLimitFailOpen RateLimitFailPolicy = "open"
	// RateLimitFailClosed denies requests when Redis is unreachable.
	RateLimitFailClosed RateLimitFailPolicy = "closed"
)

// RateLimitRule is one dimension's configured limit
// (docs/planning/26-config-management-api-surface.md Rate Limit Config
// schema). Key is the dimension's concrete value: "*" for the tenant
// dimension, or an API key ID / target host / ip_type for the others.
type RateLimitRule struct {
	Dimension     RateLimitDimension
	Key           string
	WindowSeconds uint32
	MaxRequests   uint32
	FailPolicy    RateLimitFailPolicy
}

// RateLimitCeiling bounds tenant-managed rate-limit values
// (docs/planning/26: settable only by system_admin on the tenant record). A
// nil ceiling means unbounded.
type RateLimitCeiling struct {
	WindowSeconds uint32
	MaxRequests   uint32
}

// exceeds reports whether rule's requests-per-window rate is above the
// ceiling's rate. Cross-multiplied to avoid floating point.
func (c RateLimitCeiling) exceeds(rule RateLimitRule) bool {
	if c.WindowSeconds == 0 {
		return false
	}

	return uint64(rule.MaxRequests)*uint64(c.WindowSeconds) > uint64(c.MaxRequests)*uint64(rule.WindowSeconds)
}

// RateLimitConfig is one tenant's full rate-limit configuration
// (docs/planning/26).
type RateLimitConfig struct {
	TenantID      string
	Limits        []RateLimitRule
	ConfigVersion uint64
}

var (
	// ErrRateLimitVersionConflict is returned on optimistic concurrency failure.
	ErrRateLimitVersionConflict = errors.New("rate limit config version conflict")
	// ErrRateLimitCeilingExceeded is returned when a tenant-managed
	// rate-limit value exceeds the tenant's system_admin-set ceiling.
	ErrRateLimitCeilingExceeded = errors.New("rate limit exceeds tenant ceiling")
)

// RateLimitConfigStore persists per-tenant rate-limit configuration.
// Durable storage is Postgres in production; P0 uses an in-memory store
// (docs/planning/21).
type RateLimitConfigStore interface {
	Get(ctx context.Context, tenantID string) (RateLimitConfig, error)
	// Put validates cfg.Limits against ceiling (if non-nil) before applying
	// optimistic concurrency, returning ErrRateLimitCeilingExceeded on
	// violation and ErrRateLimitVersionConflict on a version mismatch.
	Put(ctx context.Context, cfg RateLimitConfig, expectedVersion uint64, ceiling *RateLimitCeiling) (RateLimitConfig, error)
}

// InMemoryRateLimitConfigStore is the test/local rate-limit config store.
type InMemoryRateLimitConfigStore struct {
	mu    sync.Mutex
	byTid map[string]RateLimitConfig
}

// NewInMemoryRateLimitConfigStore builds an empty rate-limit config store.
func NewInMemoryRateLimitConfigStore() *InMemoryRateLimitConfigStore {
	return &InMemoryRateLimitConfigStore{byTid: make(map[string]RateLimitConfig)}
}

// Get fetches a tenant's rate-limit config, defaulting to an empty,
// version-0 config for an unconfigured tenant.
func (s *InMemoryRateLimitConfigStore) Get(_ context.Context, tenantID string) (RateLimitConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, ok := s.byTid[tenantID]
	if !ok {
		return RateLimitConfig{TenantID: tenantID, ConfigVersion: 0}, nil
	}

	return cfg, nil
}

// Put updates a tenant's rate-limit config with optimistic concurrency and
// ceiling validation.
func (s *InMemoryRateLimitConfigStore) Put(_ context.Context, cfg RateLimitConfig, expectedVersion uint64, ceiling *RateLimitCeiling) (RateLimitConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.byTid[cfg.TenantID]

	currentVersion := uint64(0)
	if ok {
		currentVersion = current.ConfigVersion
	}

	if currentVersion != expectedVersion {
		return RateLimitConfig{}, ErrRateLimitVersionConflict
	}

	if ceiling != nil && slices.ContainsFunc(cfg.Limits, ceiling.exceeds) {
		return RateLimitConfig{}, ErrRateLimitCeilingExceeded
	}

	cfg.ConfigVersion = currentVersion + 1
	s.byTid[cfg.TenantID] = cfg

	return cfg, nil
}

// RateLimitGuardrails bound per-key and per-tenant Redis memory use
// (docs/planning/20 "Memory bounds").
type RateLimitGuardrails struct {
	// MaxEntriesPerKey caps sorted-set entries per rate-limit key before the
	// limiter switches that key to a conservative deny policy until the
	// window expires (docs/planning/20).
	MaxEntriesPerKey int
	// MaxKeysPerTenant caps distinct non-tenant-dimension keys tracked per
	// tenant; beyond this, new dimension keys fall back to the tenant-level
	// rule (docs/planning/20).
	MaxKeysPerTenant int
}

const (
	defaultMaxEntriesPerKey = 10000
	defaultMaxKeysPerTenant = 1000
)

// DefaultRateLimitGuardrails returns the P0 default guardrail values.
func DefaultRateLimitGuardrails() RateLimitGuardrails {
	return RateLimitGuardrails{MaxEntriesPerKey: defaultMaxEntriesPerKey, MaxKeysPerTenant: defaultMaxKeysPerTenant}
}

// RateLimitDecision is the outcome of one dimension's admission check.
type RateLimitDecision struct {
	Allowed      bool
	RetryAfterMs int64
	Dimension    RateLimitDimension
	// RedisFailure is true when the decision came from FailPolicy rather
	// than a real counter (docs/planning/20 "Redis Failure Policy").
	RedisFailure bool
}

const rateLimitOpTimeout = 500 * time.Millisecond

const millisPerSecond = 1000

const rateLimitKeysSetTTL = 24 * time.Hour

// slidingWindowScript implements the Redis sliding-window log algorithm
// (docs/planning/20 "Algorithm: Redis sliding-window log using sorted
// sets") plus the per-key memory guardrail: once a key's sorted set exceeds
// max_entries, the key is compacted and a guard marker denies all requests
// for that key until the window elapses.
//
// KEYS[1] = sorted-set key, KEYS[2] = guard key.
// ARGV: now_ms, window_ms, max_requests, max_entries, member.
// Returns {allowed(0|1), retry_after_ms}.
const slidingWindowScript = `
local zkey = KEYS[1]
local guardkey = KEYS[2]
local now_ms = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local max_requests = tonumber(ARGV[3])
local max_entries = tonumber(ARGV[4])
local member = ARGV[5]

if redis.call('EXISTS', guardkey) == 1 then
  local ttl = redis.call('PTTL', guardkey)
  if ttl < 0 then ttl = window_ms end
  return {0, ttl}
end

redis.call('ZREMRANGEBYSCORE', zkey, 0, now_ms - window_ms)

local count = redis.call('ZCARD', zkey)

if count >= max_entries then
  redis.call('DEL', zkey)
  redis.call('SET', guardkey, '1', 'PX', window_ms)
  return {0, window_ms}
end

if count >= max_requests then
  local oldest = redis.call('ZRANGE', zkey, 0, 0, 'WITHSCORES')
  local retry = window_ms
  if oldest[2] ~= nil then
    retry = tonumber(oldest[2]) + window_ms - now_ms
    if retry < 0 then retry = 0 end
  end
  return {0, retry}
end

redis.call('ZADD', zkey, now_ms, member)
redis.call('PEXPIRE', zkey, window_ms + 1000)

return {1, 0}
`

// RateLimiter enforces a Redis sliding-window log per dimension key
// (docs/planning/20). Every Redis key it writes carries a TTL.
type RateLimiter struct {
	client     redis.Cmdable
	guardrails RateLimitGuardrails
	now        func() time.Time
}

// NewRateLimiter builds a RateLimiter. now may be nil (defaults to
// time.Now).
func NewRateLimiter(client redis.Cmdable, guardrails RateLimitGuardrails, now func() time.Time) *RateLimiter {
	if now == nil {
		now = time.Now
	}

	return &RateLimiter{client: client, guardrails: guardrails, now: now}
}

func rateLimitKey(tenantID string, dim RateLimitDimension, key string) string {
	return fmt.Sprintf("straw:ratelimit:%s:%s:%s", tenantID, dim, key)
}

func rateLimitKeysSetKey(tenantID string) string {
	return "straw:ratelimit:keys:" + tenantID
}

// Allow evaluates rule for the given tenant, returning whether the request
// is admitted. On Redis failure it applies rule.FailPolicy rather than
// returning an error, since a Redis outage is an explicit, expected
// admission outcome, not a caller-visible failure.
func (rl *RateLimiter) Allow(ctx context.Context, tenantID string, rule RateLimitRule) RateLimitDecision {
	opCtx, cancel := context.WithTimeout(ctx, rateLimitOpTimeout)
	defer cancel()

	zkey := rateLimitKey(tenantID, rule.Dimension, rule.Key)
	guardKey := zkey + ":guard"
	nowMs := rl.now().UnixMilli()
	windowMs := int64(rule.WindowSeconds) * millisPerSecond
	member := fmt.Sprintf("%d-%s", nowMs, randomMember())

	res, err := rl.client.Eval(opCtx, slidingWindowScript, []string{zkey, guardKey},
		nowMs, windowMs, rule.MaxRequests, rl.guardrails.MaxEntriesPerKey, member,
	).Result()
	if err != nil {
		return rl.failureDecision(rule)
	}

	values, ok := res.([]any)
	if !ok || len(values) != 2 {
		return rl.failureDecision(rule)
	}

	return RateLimitDecision{
		Allowed:      toInt64(values[0]) == 1,
		RetryAfterMs: toInt64(values[1]),
		Dimension:    rule.Dimension,
	}
}

// WithinKeyBudget reports whether dimensionKey is (or can become) a tracked
// distinct rate-limit key for tenantID, applying the per-tenant key-count
// guardrail (docs/planning/20 "Maximum rate-limit keys per tenant"). When
// the budget is exhausted for a new key, callers should evaluate the
// tenant-level rule instead of this dimension. Redis failure here fails
// open (the budget check itself is best-effort; the window check below
// still applies its own configured fail policy).
func (rl *RateLimiter) WithinKeyBudget(ctx context.Context, tenantID, dimensionKey string) bool {
	opCtx, cancel := context.WithTimeout(ctx, rateLimitOpTimeout)
	defer cancel()

	setKey := rateLimitKeysSetKey(tenantID)

	added, err := rl.client.SAdd(opCtx, setKey, dimensionKey).Result()
	if err != nil {
		return true
	}

	rl.client.Expire(opCtx, setKey, rateLimitKeysSetTTL)

	if added == 0 {
		return true
	}

	card, err := rl.client.SCard(opCtx, setKey).Result()
	if err != nil {
		return true
	}

	if int(card) > rl.guardrails.MaxKeysPerTenant {
		rl.client.SRem(opCtx, setKey, dimensionKey)

		return false
	}

	return true
}

func (rl *RateLimiter) failureDecision(rule RateLimitRule) RateLimitDecision {
	return RateLimitDecision{
		Allowed:      rule.FailPolicy != RateLimitFailClosed,
		Dimension:    rule.Dimension,
		RedisFailure: true,
	}
}

// RateLimitRequest carries the request-derived dimension values used to
// evaluate tenant-managed rate-limit rules.
type RateLimitRequest struct {
	TenantID   string
	APIKeyID   string
	TargetHost string
	IPType     string
}

// RateLimitAdmission evaluates every dimension configured for a tenant
// against one request.
type RateLimitAdmission struct {
	limiter *RateLimiter
	metrics *Metrics
}

// NewRateLimitAdmission builds a RateLimitAdmission over limiter.
func NewRateLimitAdmission(limiter *RateLimiter) *RateLimitAdmission {
	return &RateLimitAdmission{limiter: limiter}
}

// SetMetrics attaches the Prometheus metrics recorder used for
// straw_rate_limit_rejections_total (docs/planning/23).
func (a *RateLimitAdmission) SetMetrics(m *Metrics) {
	if a == nil {
		return
	}

	a.metrics = m
}

// Check evaluates every rule in cfg applicable to req and returns the most
// restrictive breach, if any. An empty cfg.Limits always allows.
func (a *RateLimitAdmission) Check(ctx context.Context, cfg RateLimitConfig, req RateLimitRequest) RateLimitDecision {
	worst := RateLimitDecision{Allowed: true}

	for _, rule := range cfg.Limits {
		key, ok := dimensionKeyFor(rule.Dimension, req)
		if !ok {
			continue
		}

		if rule.Dimension != RateLimitDimTenant {
			if !a.limiter.WithinKeyBudget(ctx, req.TenantID, string(rule.Dimension)+":"+key) {
				// Per-tenant key budget exhausted: this dimension key falls
				// back to the tenant-level rule instead of being evaluated
				// on its own.
				continue
			}
		}

		evalRule := rule
		evalRule.Key = key

		decision := a.limiter.Allow(ctx, req.TenantID, evalRule)
		if !decision.Allowed && (worst.Allowed || decision.RetryAfterMs > worst.RetryAfterMs) {
			worst = decision
		}
	}

	if !worst.Allowed {
		a.metrics.IncRateLimitRejection(req.TenantID)
	}

	return worst
}

func dimensionKeyFor(dim RateLimitDimension, req RateLimitRequest) (string, bool) {
	switch dim {
	case RateLimitDimTenant:
		return "*", true
	case RateLimitDimAPIKey:
		if req.APIKeyID == "" {
			return "", false
		}

		return req.APIKeyID, true
	case RateLimitDimTargetHost:
		if req.TargetHost == "" {
			return "", false
		}

		return req.TargetHost, true
	case RateLimitDimIPType:
		if req.IPType == "" {
			return "", false
		}

		return req.IPType, true
	default:
		return "", false
	}
}

func randomMember() string {
	var b [8]byte

	_, _ = rand.Read(b[:])

	return hex.EncodeToString(b[:])
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)

		return i
	default:
		return 0
	}
}
