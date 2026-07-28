package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

var errInvalidRedisScanReply = errors.New("invalid Redis SCAN reply")

const (
	runtimeInstanceIDBytes = 12
	instanceDrainTimeout   = 2 * time.Second
)

// RuntimeState is the shared coordination boundary required by multi-Control
// deployments. The in-memory WorkerRegistry and StickyStore remain the default;
// RedisRuntimeState implements this interface for the opt-in HA profile.
type RuntimeState interface {
	Ping(ctx context.Context) error
	putWorker(ctx context.Context, workerID string, worker sharedWorker, ttl time.Duration) error
	heartbeatWorker(ctx context.Context, workerID, sessionID string, worker sharedWorker, ttl time.Duration) (bool, error)
	workers(ctx context.Context) (map[string]sharedWorker, error)
	recordWorkerFailure(ctx context.Context, workerID, sessionID string, timings WorkerTimings) error
	workerCoolingDown(ctx context.Context, workerID, sessionID string) (bool, error)
	getSticky(ctx context.Context, deploymentID, sessionID string) (string, bool, error)
	setSticky(ctx context.Context, deploymentID, sessionID, workerID string, ttl time.Duration) error
	claimRequest(ctx context.Context, requestID, deploymentID, owner string, ttl time.Duration) (bool, error)
	renewRequest(ctx context.Context, requestID, owner string, ttl time.Duration) (bool, error)
	releaseRequest(ctx context.Context, requestID, owner string) error
	requests(ctx context.Context) ([]InFlightRequest, error)
	requestOwner(ctx context.Context, requestID string) (string, bool, error)
	touchInstance(ctx context.Context, instanceID, state string, ttl time.Duration) error
}

type sharedWorker struct {
	SessionID                    string               `json:"session_id"`
	ExecutorType                 string               `json:"executor_type"`
	SupportedProtocolMinor       uint32               `json:"supported_protocol_minor,omitempty"`
	ProtocolMinor                uint32               `json:"protocol_minor,omitempty"`
	Pools                        []AllowedPool        `json:"pools"`
	Tags                         []string             `json:"tags,omitempty"`
	Countries                    []string             `json:"countries,omitempty"`
	Regions                      []string             `json:"regions,omitempty"`
	IPTypes                      []string             `json:"ip_types,omitempty"`
	IngressModes                 []string             `json:"ingress_modes,omitempty"`
	SupportedFingerprintProfiles []string             `json:"supported_fingerprint_profiles,omitempty"`
	MaxConcurrency               uint32               `json:"max_concurrency"`
	ActiveRequests               uint32               `json:"active_requests"`
	AvailableCapacity            uint32               `json:"available_capacity"`
	Health                       strawpb.WorkerHealth `json:"health"`
	Draining                     bool                 `json:"draining"`
	RegisteredAt                 time.Time            `json:"registered_at"`
	LastHeartbeat                time.Time            `json:"last_heartbeat"`
	HasHeartbeat                 bool                 `json:"has_heartbeat"`
}

func sharedFromSession(s *workerSession) sharedWorker {
	return sharedWorker{
		SessionID: s.sessionID, ExecutorType: s.executorType, SupportedProtocolMinor: s.supportedProtocolMinor,
		ProtocolMinor: s.protocolMinor, Pools: s.pools, Tags: s.tags,
		Countries: s.countries, Regions: s.regions, IPTypes: s.ipTypes, IngressModes: s.ingressModes,
		SupportedFingerprintProfiles: s.supportedFingerprintProfiles, MaxConcurrency: s.maxConcurrency,
		ActiveRequests: s.activeRequests, AvailableCapacity: s.availableCapacity, Health: s.health,
		Draining: s.draining, RegisteredAt: s.registeredAt, LastHeartbeat: s.lastHeartbeat, HasHeartbeat: s.hasHeartbeat,
	}
}

func (w sharedWorker) session() *workerSession {
	return &workerSession{
		sessionID: w.SessionID, executorType: w.ExecutorType, supportedProtocolMinor: w.SupportedProtocolMinor,
		protocolMinor: w.ProtocolMinor, pools: w.Pools, tags: w.Tags,
		countries: w.Countries, regions: w.Regions, ipTypes: w.IPTypes, ingressModes: w.IngressModes,
		supportedFingerprintProfiles: w.SupportedFingerprintProfiles, maxConcurrency: w.MaxConcurrency,
		activeRequests: w.ActiveRequests, availableCapacity: w.AvailableCapacity, health: w.Health,
		draining: w.Draining, registeredAt: w.RegisteredAt, lastHeartbeat: w.LastHeartbeat, hasHeartbeat: w.HasHeartbeat,
	}
}

type redisDoer interface {
	do(ctx context.Context, args ...string) (any, error)
}

// RedisRuntimeState stores only ephemeral coordination in Redis. Durable
// configuration remains in JetStream KV.
type RedisRuntimeState struct {
	redis      redisDoer
	prefix     string
	available  atomic.Bool
	operations atomic.Uint64
	errors     atomic.Uint64
}

// NewRedisRuntimeState creates a Redis-backed ephemeral coordination store.
func NewRedisRuntimeState(redis *RESPClient, prefix string) *RedisRuntimeState {
	r := &RedisRuntimeState{redis: redis, prefix: strings.TrimSuffix(prefix, ":")}

	return r
}

// Stats returns bounded-cardinality shared-state metrics.
func (r *RedisRuntimeState) Stats() RuntimeStateStats {
	return RuntimeStateStats{Available: r.available.Load(), Operations: r.operations.Load(), Errors: r.errors.Load()}
}

// Available reports whether the most recent Redis operation succeeded.
func (r *RedisRuntimeState) Available() bool { return r.available.Load() }

// Ping checks whether Redis is reachable and updates availability.
func (r *RedisRuntimeState) Ping(ctx context.Context) error {
	_, err := r.run(ctx, "PING")

	return err
}

func (r *RedisRuntimeState) key(parts ...string) string {
	return r.prefix + ":" + strings.Join(parts, ":")
}

func (r *RedisRuntimeState) run(ctx context.Context, args ...string) (any, error) {
	value, err := r.redis.do(ctx, args...)
	r.operations.Add(1)

	if err != nil {
		r.errors.Add(1)
	}

	r.available.Store(err == nil)

	if err != nil {
		return nil, fmt.Errorf("execute Redis runtime-state operation: %w", err)
	}

	return value, nil
}

func (r *RedisRuntimeState) putWorker(ctx context.Context, id string, w sharedWorker, ttl time.Duration) error {
	raw, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("encode shared worker: %w", err)
	}

	script := `redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[3]); redis.call('SET',KEYS[2],ARGV[2],'PX',ARGV[3]); return 1`
	_, err = r.run(ctx, "EVAL", script, "2", r.key("worker-fence", id), r.key("worker", id), w.SessionID, string(raw), ms(ttl))

	return err
}

func (r *RedisRuntimeState) heartbeatWorker(ctx context.Context, id, session string, w sharedWorker, ttl time.Duration) (bool, error) {
	raw, err := json.Marshal(w)
	if err != nil {
		return false, fmt.Errorf("encode shared worker heartbeat: %w", err)
	}

	script := `if redis.call('GET',KEYS[1])~=ARGV[1] then return 0 end; redis.call('PEXPIRE',KEYS[1],ARGV[3]); redis.call('SET',KEYS[2],ARGV[2],'PX',ARGV[3]); return 1`
	v, err := r.run(ctx, "EVAL", script, "2", r.key("worker-fence", id), r.key("worker", id), session, string(raw), ms(ttl))

	return integer(v) == 1, err
}

func (r *RedisRuntimeState) workers(ctx context.Context) (map[string]sharedWorker, error) {
	keys, err := r.scan(ctx, r.key("worker", "*"))
	if err != nil {
		return nil, err
	}

	out := make(map[string]sharedWorker, len(keys))
	for _, key := range keys {
		v, getErr := r.run(ctx, "GET", key)
		if getErr != nil {
			return nil, getErr
		}

		if v == nil {
			continue
		}

		var w sharedWorker

		decodeErr := json.Unmarshal([]byte(stringValue(v)), &w)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode shared worker: %w", decodeErr)
		}

		out[strings.TrimPrefix(key, r.key("worker", ""))] = w
	}

	return out, nil
}

func (r *RedisRuntimeState) recordWorkerFailure(ctx context.Context, id, session string, t WorkerTimings) error {
	key := r.key("failure", id, session)
	script := `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; if n>=tonumber(ARGV[2]) then redis.call('SET',KEYS[2],'1','PX',ARGV[3]); redis.call('DEL',KEYS[1]) end; return n`
	_, err := r.run(ctx, "EVAL", script, "2", key, r.key("cooldown", id, session), ms(t.CooldownWindow), strconv.Itoa(t.CooldownFailureCount), ms(t.CooldownDuration))

	return err
}

func (r *RedisRuntimeState) workerCoolingDown(ctx context.Context, id, session string) (bool, error) {
	v, err := r.run(ctx, "EXISTS", r.key("cooldown", id, session))

	return integer(v) == 1, err
}

func (r *RedisRuntimeState) getSticky(ctx context.Context, deployment, session string) (string, bool, error) {
	v, err := r.run(ctx, "GET", r.key("sticky", deployment, session))

	return stringValue(v), v != nil, err
}

func (r *RedisRuntimeState) setSticky(ctx context.Context, deployment, session, worker string, ttl time.Duration) error {
	_, err := r.run(ctx, "SET", r.key("sticky", deployment, session), worker, "PX", ms(ttl))

	return err
}

func (r *RedisRuntimeState) claimRequest(ctx context.Context, requestID, deployment, owner string, ttl time.Duration) (bool, error) {
	v, err := r.run(ctx, "SET", r.key("request", requestID), owner+"|"+deployment, "NX", "PX", ms(ttl))

	return stringValue(v) == "OK", err
}

func (r *RedisRuntimeState) renewRequest(ctx context.Context, requestID, owner string, ttl time.Duration) (bool, error) {
	script := `local v=redis.call('GET',KEYS[1]); if not v or string.sub(v,1,string.len(ARGV[1])+1)~=ARGV[1]..'|' then return 0 end; redis.call('PEXPIRE',KEYS[1],ARGV[2]); return 1`
	v, err := r.run(ctx, "EVAL", script, "1", r.key("request", requestID), owner, ms(ttl))

	return integer(v) == 1, err
}

func (r *RedisRuntimeState) releaseRequest(ctx context.Context, requestID, owner string) error {
	script := `local v=redis.call('GET',KEYS[1]); if v and string.sub(v,1,string.len(ARGV[1])+1)==ARGV[1]..'|' then return redis.call('DEL',KEYS[1]) end; return 0`
	_, err := r.run(ctx, "EVAL", script, "1", r.key("request", requestID), owner)

	return err
}

func (r *RedisRuntimeState) requestOwner(ctx context.Context, requestID string) (string, bool, error) {
	v, err := r.run(ctx, "GET", r.key("request", requestID))
	if v == nil {
		return "", false, err
	}

	owner, _, _ := strings.Cut(stringValue(v), "|")

	return owner, true, err
}

func (r *RedisRuntimeState) requests(ctx context.Context) ([]InFlightRequest, error) {
	keys, err := r.scan(ctx, r.key("request", "*"))
	if err != nil {
		return nil, err
	}

	out := make([]InFlightRequest, 0, len(keys))
	for _, key := range keys {
		v, getErr := r.run(ctx, "GET", key)
		if getErr != nil {
			return nil, getErr
		}

		if v == nil {
			continue
		}

		_, dep, _ := strings.Cut(stringValue(v), "|")
		out = append(out, InFlightRequest{RequestID: strings.TrimPrefix(key, r.key("request", "")), DeploymentID: dep})
	}

	return out, nil
}

func (r *RedisRuntimeState) touchInstance(ctx context.Context, id, state string, ttl time.Duration) error {
	_, err := r.run(ctx, "SET", r.key("instance", id), state, "PX", ms(ttl))

	return err
}

func (r *RedisRuntimeState) scan(ctx context.Context, pattern string) ([]string, error) {
	var out []string

	cursor := "0"
	for {
		v, err := r.run(ctx, "SCAN", cursor, "MATCH", pattern, "COUNT", "100")
		if err != nil {
			return nil, err
		}

		a, ok := v.([]any)
		if !ok || len(a) != 2 {
			return nil, errInvalidRedisScanReply
		}

		cursor = stringValue(a[0])
		for _, item := range a[1].([]any) {
			out = append(out, stringValue(item))
		}

		if cursor == "0" {
			return out, nil
		}
	}
}
func ms(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }
func integer(v any) int64 {
	n, _ := v.(int64)

	return n
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}

	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}

// NewRuntimeInstanceID generates a process-unique fencing identity.
func NewRuntimeInstanceID() (string, error) {
	b := make([]byte, runtimeInstanceIDBytes)

	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("read instance id randomness: %w", err)
	}

	return "control-" + hex.EncodeToString(b), nil
}

// RuntimeStateStats is the bounded-cardinality operational view.
type RuntimeStateStats struct {
	Available          bool
	Operations, Errors uint64
}

// RunInstanceLease advertises active/draining lifecycle and continuously
// probes Redis so readiness recovers without restarting Control.
func RunInstanceLease(ctx context.Context, state RuntimeState, instanceID string, ttl time.Duration) {
	touch := func(ctx context.Context, status string) { _ = state.touchInstance(ctx, instanceID, status, ttl) }
	touch(ctx, "active")

	interval := ttl / leaseRefreshDivisor
	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), min(ttl/leaseRefreshDivisor, instanceDrainTimeout))
			touch(drainCtx, "draining")
			cancel()

			return
		case <-ticker.C:
			touch(ctx, "active")
		}
	}
}

// RedisStickyBackend adapts shared runtime state to the routing interface. A
// Redis outage drops optional affinity; readiness simultaneously fails closed.
type RedisStickyBackend struct {
	ctx   context.Context
	state RuntimeState
}

// NewRedisStickyBackend adapts shared state to the routing sticky-store boundary.
func NewRedisStickyBackend(ctx context.Context, state RuntimeState) *RedisStickyBackend {
	return &RedisStickyBackend{ctx: ctx, state: state}
}

// Get returns a shared sticky-session pin.
func (s *RedisStickyBackend) Get(deploymentID, sessionID string) (string, bool) {
	v, ok, err := s.state.getSticky(s.ctx, deploymentID, sessionID)

	return v, ok && err == nil
}

// Set stores a shared sticky-session pin.
func (s *RedisStickyBackend) Set(deploymentID, sessionID, workerID string, ttl time.Duration) {
	_ = s.state.setSticky(s.ctx, deploymentID, sessionID, workerID, ttl)
}

// Refresh extends a shared sticky-session pin.
func (s *RedisStickyBackend) Refresh(deploymentID, sessionID, workerID string, ttl time.Duration) {
	s.Set(deploymentID, sessionID, workerID, ttl)
}
