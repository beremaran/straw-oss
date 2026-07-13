package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/fingerprint"
	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	// ProtocolMajor is the Control-to-worker protocol major version.
	ProtocolMajor uint32 = 1

	workerAvailabilityTimeout  = 15 * time.Second
	workerDeadTimeout          = 30 * time.Second
	workerCooldownFailureCount = 3
	workerCooldownWindow       = 60 * time.Second
	workerCooldownDuration     = 30 * time.Second
	randomSessionIDBytes       = 16

	rejectInvalidWorkerID   = "invalid_worker_id"
	rejectIncompatibleProto = "incompatible_protocol"
	rejectCapabilityScope   = "capability_out_of_scope"
	rejectInvalidPool       = "invalid_pool_membership"
)

// WorkerRuntimeState is the worker's current availability state.
type WorkerRuntimeState string

// Worker runtime states used by routing and metrics.
const (
	RuntimeRegistered  WorkerRuntimeState = "registered"
	RuntimeReady       WorkerRuntimeState = "ready"
	RuntimeDegraded    WorkerRuntimeState = "degraded"
	RuntimeUnavailable WorkerRuntimeState = "unavailable"
	RuntimeDead        WorkerRuntimeState = "dead"
	RuntimeDraining    WorkerRuntimeState = "draining"
	RuntimeCooldown    WorkerRuntimeState = "cooldown"
	RuntimeUnhealthy   WorkerRuntimeState = "unhealthy"
	RuntimeDisabled    WorkerRuntimeState = "disabled"
)

// WorkerTimings controls liveness and failure cooldown thresholds.
type WorkerTimings struct {
	AvailabilityTimeout  time.Duration
	DeadTimeout          time.Duration
	CooldownFailureCount int
	CooldownWindow       time.Duration
	CooldownDuration     time.Duration
}

// DefaultWorkerTimings returns conservative local worker thresholds.
func DefaultWorkerTimings() WorkerTimings {
	return WorkerTimings{
		AvailabilityTimeout:  workerAvailabilityTimeout,
		DeadTimeout:          workerDeadTimeout,
		CooldownFailureCount: workerCooldownFailureCount,
		CooldownWindow:       workerCooldownWindow,
		CooldownDuration:     workerCooldownDuration,
	}
}

// RegisterOutcome is returned to a worker registration request.
type RegisterOutcome struct {
	OK        bool
	SessionID string
	Reason    string
}

// AllowedPool identifies one worker pool.
type AllowedPool struct {
	PoolID string `json:"pool_id"`
}

type workerSession struct {
	sessionID                    string
	executorType                 string
	pools                        []AllowedPool
	tags                         []string
	countries                    []string
	regions                      []string
	ipTypes                      []string
	ingressModes                 []string
	supportedFingerprintProfiles []string
	maxConcurrency               uint32
	activeRequests               uint32
	availableCapacity            uint32
	health                       strawpb.WorkerHealth
	draining                     bool
	registeredAt                 time.Time
	lastHeartbeat                time.Time
	hasHeartbeat                 bool
	failures                     []time.Time
	cooldownUntil                time.Time
}

func (s *workerSession) lastSeen() time.Time {
	if s.hasHeartbeat {
		return s.lastHeartbeat
	}

	return s.registeredAt
}

// WorkerRegistry tracks live worker sessions in memory.
type WorkerRegistry struct {
	mu        sync.Mutex
	now       func() time.Time
	timings   WorkerTimings
	workers   map[string]*workerSession
	settings  map[string]config.WorkerSetting
	pools     map[string]config.ExecutorPool
	shared    RuntimeState
	sharedTTL time.Duration
	ctx       context.Context
}

// NewDeploymentWorkerRegistry creates the in-process worker registry.
func NewDeploymentWorkerRegistry(timings WorkerTimings, now func() time.Time) *WorkerRegistry {
	if now == nil {
		now = time.Now
	}

	return &WorkerRegistry{
		now: now, timings: timings, workers: make(map[string]*workerSession), settings: make(map[string]config.WorkerSetting),
		pools: map[string]config.ExecutorPool{
			config.DefaultPoolID: {ID: config.DefaultPoolID, ExecutorType: errorCategoryEgress, Enabled: true},
		},
	}
}

// NewSharedWorkerRegistry creates a registry whose ephemeral worker sessions,
// heartbeats, capacity and cooldowns are visible to every Control instance.
func NewSharedWorkerRegistry(ctx context.Context, timings WorkerTimings, now func() time.Time, state RuntimeState, ttl time.Duration) *WorkerRegistry {
	r := NewDeploymentWorkerRegistry(timings, now)
	r.shared, r.sharedTTL = state, ttl
	r.ctx = ctx

	return r
}

// Register creates or replaces one worker session.
func (r *WorkerRegistry) Register(ctx context.Context, req *strawpb.RegisterRequest) (RegisterOutcome, error) {
	if req == nil || !validWorkerID(req.GetWorkerId()) {
		return RegisterOutcome{Reason: rejectInvalidWorkerID}, nil
	}

	if req.GetProtocolMajor() != ProtocolMajor {
		return RegisterOutcome{Reason: rejectIncompatibleProto}, nil
	}

	if reason := rejectFingerprintProfileCapabilities(req); reason != "" {
		return RegisterOutcome{Reason: reason}, nil
	}

	sessionID, err := newWorkerSessionID()
	if err != nil {
		return RegisterOutcome{}, err
	}

	r.mu.Lock()
	pools, poolReason := deploymentPools(req.GetAllowedPools(), r.pools)

	r.mu.Unlock()

	if poolReason != "" {
		return RegisterOutcome{Reason: poolReason}, nil
	}

	session := &workerSession{
		sessionID:                    sessionID,
		executorType:                 req.GetExecutorType(),
		pools:                        pools,
		tags:                         append([]string(nil), req.GetTags()...),
		countries:                    append([]string(nil), req.GetCountries()...),
		regions:                      append([]string(nil), req.GetRegions()...),
		ipTypes:                      append([]string(nil), req.GetIpTypes()...),
		ingressModes:                 append([]string(nil), req.GetSupportedIngressModes()...),
		supportedFingerprintProfiles: append([]string(nil), req.GetSupportedFingerprintProfiles()...),
		maxConcurrency:               req.GetMaxConcurrency(),
		health:                       strawpb.WorkerHealth_WORKER_HEALTH_READY,
		draining:                     req.GetInitialDraining(),
		registeredAt:                 r.now(),
	}

	r.mu.Lock()
	r.workers[req.GetWorkerId()] = session
	r.mu.Unlock()

	if r.shared != nil {
		err = r.shared.putWorker(ctx, req.GetWorkerId(), sharedFromSession(session), r.sharedTTL)
		if err != nil {
			return RegisterOutcome{}, fmt.Errorf("store shared worker session: %w", err)
		}
	}

	return RegisterOutcome{OK: true, SessionID: sessionID}, nil
}

func rejectFingerprintProfileCapabilities(req *strawpb.RegisterRequest) string {
	profiles := req.GetSupportedFingerprintProfiles()
	if len(profiles) == 0 {
		return ""
	}

	if req.GetProtocolMinor() < 1 || len(profiles) > len(fingerprint.Names()) {
		return rejectCapabilityScope
	}

	seen := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		if !fingerprint.Contains(profile) {
			return rejectCapabilityScope
		}

		if _, duplicate := seen[profile]; duplicate {
			return rejectCapabilityScope
		}

		seen[profile] = struct{}{}
	}

	return ""
}

func deploymentPools(refs []*strawpb.RegisterRequest_PoolRef, configured map[string]config.ExecutorPool) ([]AllowedPool, string) {
	if len(refs) == 0 {
		refs = []*strawpb.RegisterRequest_PoolRef{{DeploymentId: config.DefaultDeploymentID, PoolId: config.DefaultPoolID}}
	}

	pools := make([]AllowedPool, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))

	for _, ref := range refs {
		if ref == nil {
			return nil, rejectInvalidPool
		}

		deploymentID := ref.GetDeploymentId()
		if deploymentID == "" {
			deploymentID = config.DefaultDeploymentID
		}

		if deploymentID != config.DefaultDeploymentID {
			return nil, rejectInvalidPool
		}

		poolID := ref.GetPoolId()
		if poolID == "" {
			poolID = config.DefaultPoolID
		}

		if _, ok := configured[poolID]; !ok {
			return nil, rejectInvalidPool
		}

		if _, duplicate := seen[poolID]; duplicate {
			return nil, rejectInvalidPool
		}

		seen[poolID] = struct{}{}
		pools = append(pools, AllowedPool{PoolID: poolID})
	}

	return pools, ""
}

// Heartbeat refreshes one current worker session.
func (r *WorkerRegistry) Heartbeat(ctx context.Context, hb *strawpb.HeartbeatRequest) (bool, error) {
	if hb == nil {
		return false, nil
	}

	if r.shared != nil {
		workers, err := r.shared.workers(ctx)
		if err != nil {
			return false, fmt.Errorf("read shared worker session: %w", err)
		}

		shared, ok := workers[hb.GetWorkerId()]
		if !ok || shared.SessionID != hb.GetSessionId() {
			return false, nil
		}

		session := shared.session()
		applyHeartbeat(session, hb, r.now())

		ok, err = r.shared.heartbeatWorker(ctx, hb.GetWorkerId(), hb.GetSessionId(), sharedFromSession(session), r.sharedTTL)
		if err != nil {
			return false, fmt.Errorf("update shared worker heartbeat: %w", err)
		}

		return ok, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session := r.workers[hb.GetWorkerId()]
	if session == nil || session.sessionID != hb.GetSessionId() {
		return false, nil
	}

	applyHeartbeat(session, hb, r.now())

	return true, nil
}

func applyHeartbeat(session *workerSession, hb *strawpb.HeartbeatRequest, now time.Time) {
	session.hasHeartbeat = true

	session.lastHeartbeat = now
	if hb.Health.Valid() && hb.GetHealth() != strawpb.WorkerHealth_WORKER_HEALTH_UNSPECIFIED {
		session.health = hb.GetHealth()
	}

	session.activeRequests = hb.GetActiveRequests()

	session.availableCapacity = hb.GetAvailableCapacity()
	if hb.GetMaxConcurrency() > 0 {
		session.maxConcurrency = hb.GetMaxConcurrency()
	}

	session.draining = hb.GetDraining()
}

// RecordFailure contributes to a worker's short cooldown circuit breaker.
func (r *WorkerRegistry) RecordFailure(workerID string) {
	if r.shared != nil {
		workers, err := r.shared.workers(r.ctx)
		if err != nil {
			return
		}

		if worker, ok := workers[workerID]; ok {
			_ = r.shared.recordWorkerFailure(r.ctx, workerID, worker.SessionID, r.timings)
		}

		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session := r.workers[workerID]
	if session == nil {
		return
	}

	now := r.now()
	cutoff := now.Add(-r.timings.CooldownWindow)

	kept := session.failures[:0]
	for _, failure := range session.failures {
		if failure.After(cutoff) {
			kept = append(kept, failure)
		}
	}

	kept = append(kept, now)

	session.failures = kept
	if len(session.failures) >= r.timings.CooldownFailureCount {
		session.cooldownUntil = now.Add(r.timings.CooldownDuration)
		session.failures = nil
	}
}

// PoolCandidate is a live worker eligible for routing.
type PoolCandidate struct {
	WorkerID                     string
	SessionID                    string
	AssignSubject                string
	ExecutorType                 string
	Degraded                     bool
	Tags                         []string
	Countries                    []string
	Regions                      []string
	IPTypes                      []string
	IngressModes                 []string
	SupportedFingerprintProfiles []string
	ActiveRequests               uint32
	MaxConcurrency               uint32
	AvailableCap                 uint32
}

// CandidatesForPool returns live workers advertising the requested pool.
func (r *WorkerRegistry) CandidatesForPool(_ string, poolID string) []PoolCandidate {
	if r.shared != nil {
		workers, err := r.shared.workers(r.ctx)
		if err != nil {
			return nil
		}

		return candidatesFromShared(r, workers, poolID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()

	candidates := make([]PoolCandidate, 0, len(r.workers))
	for workerID, session := range r.workers {
		state := r.runtimeState(workerID, session, now)
		if state != RuntimeReady && state != RuntimeDegraded || !sessionInPool(session, poolID) {
			continue
		}

		subject, err := natsx.AssignmentSubject(workerID, session.sessionID)
		if err != nil {
			continue
		}

		candidates = append(candidates, PoolCandidate{
			WorkerID: workerID, SessionID: session.sessionID, AssignSubject: subject,
			ExecutorType: session.executorType, Degraded: state == RuntimeDegraded,
			Tags: session.tags, Countries: session.countries, Regions: session.regions,
			IPTypes: session.ipTypes, IngressModes: session.ingressModes,
			SupportedFingerprintProfiles: append([]string(nil), session.supportedFingerprintProfiles...),
			ActiveRequests:               session.activeRequests, MaxConcurrency: session.maxConcurrency,
			AvailableCap: session.availableCapacity,
		})
	}

	return candidates
}

func candidatesFromShared(r *WorkerRegistry, workers map[string]sharedWorker, poolID string) []PoolCandidate {
	r.mu.Lock()

	settings := make(map[string]config.WorkerSetting, len(r.settings))
	maps.Copy(settings, r.settings)
	r.mu.Unlock()
	now := r.now()

	out := make([]PoolCandidate, 0, len(workers))
	for workerID, shared := range workers {
		candidate, ok := sharedCandidate(r, settings, workerID, shared, poolID, now)
		if ok {
			out = append(out, candidate)
		}
	}

	return out
}

func sharedCandidate(r *WorkerRegistry, settings map[string]config.WorkerSetting, workerID string, shared sharedWorker, poolID string, now time.Time) (PoolCandidate, bool) {
	session := shared.session()

	state := runtimeStateWithSettings(r.timings, settings, workerID, session, now)
	if state != RuntimeReady && state != RuntimeDegraded {
		return PoolCandidate{}, false
	}

	if !sessionInPool(session, poolID) {
		return PoolCandidate{}, false
	}

	coolingDown, err := r.shared.workerCoolingDown(r.ctx, workerID, session.sessionID)
	if err != nil || coolingDown {
		return PoolCandidate{}, false
	}

	subject, err := natsx.AssignmentSubject(workerID, session.sessionID)
	if err != nil {
		return PoolCandidate{}, false
	}

	return candidateFromSession(workerID, subject, session, state == RuntimeDegraded), true
}

func candidateFromSession(workerID, subject string, session *workerSession, degraded bool) PoolCandidate {
	return PoolCandidate{
		WorkerID: workerID, SessionID: session.sessionID, AssignSubject: subject, ExecutorType: session.executorType,
		Degraded: degraded, Tags: session.tags, Countries: session.countries, Regions: session.regions, IPTypes: session.ipTypes,
		IngressModes: session.ingressModes, SupportedFingerprintProfiles: append([]string(nil), session.supportedFingerprintProfiles...),
		ActiveRequests: session.activeRequests, MaxConcurrency: session.maxConcurrency, AvailableCap: session.availableCapacity,
	}
}

// ApplySnapshot replaces durable worker lifecycle overrides atomically with routing reads.
func (r *WorkerRegistry) ApplySnapshot(snapshot config.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.settings = make(map[string]config.WorkerSetting, len(snapshot.WorkerSettings))
	for _, setting := range snapshot.WorkerSettings {
		r.settings[setting.WorkerID] = setting
	}

	r.pools = make(map[string]config.ExecutorPool, len(snapshot.ExecutorPools))
	for _, pool := range snapshot.ExecutorPools {
		r.pools[pool.ID] = pool
	}
}

// WorkerInfo is the administrative view of a registered worker.
type WorkerInfo struct {
	WorkerID          string             `json:"worker_id"`
	SessionID         string             `json:"session_id"`
	State             WorkerRuntimeState `json:"state"`
	Enabled           bool               `json:"enabled"`
	Draining          bool               `json:"draining"`
	ExecutorType      string             `json:"executor_type"`
	ActiveRequests    uint32             `json:"active_requests"`
	AvailableCapacity uint32             `json:"available_capacity"`
	MaxConcurrency    uint32             `json:"max_concurrency"`
	LastSeen          time.Time          `json:"last_seen"`
	Pools             []string           `json:"pools"`
}

// Workers returns a stable administrative snapshot of all registered workers.
func (r *WorkerRegistry) Workers() []WorkerInfo {
	if r.shared != nil {
		workers, err := r.shared.workers(r.ctx)
		if err != nil {
			return nil
		}

		return workerInfoFromShared(r, workers)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()

	out := make([]WorkerInfo, 0, len(r.workers))
	for workerID, session := range r.workers {
		setting, overridden := r.settings[workerID]

		enabled, draining := true, session.draining
		if overridden {
			enabled, draining = setting.Enabled, setting.Draining || session.draining
		}

		pools := make([]string, 0, len(session.pools))
		for _, pool := range session.pools {
			pools = append(pools, pool.PoolID)
		}

		out = append(out, WorkerInfo{WorkerID: workerID, SessionID: session.sessionID, State: r.runtimeState(workerID, session, now), Enabled: enabled, Draining: draining, ExecutorType: session.executorType, ActiveRequests: session.activeRequests, AvailableCapacity: session.availableCapacity, MaxConcurrency: session.maxConcurrency, LastSeen: session.lastSeen(), Pools: pools})
	}

	slices.SortFunc(out, func(a, b WorkerInfo) int { return strings.Compare(a.WorkerID, b.WorkerID) })

	return out
}

func workerInfoFromShared(r *WorkerRegistry, workers map[string]sharedWorker) []WorkerInfo {
	r.mu.Lock()

	settings := make(map[string]config.WorkerSetting, len(r.settings))
	maps.Copy(settings, r.settings)
	r.mu.Unlock()
	now := r.now()

	out := make([]WorkerInfo, 0, len(workers))
	for workerID, shared := range workers {
		s := shared.session()
		setting, overridden := settings[workerID]

		enabled, draining := true, s.draining
		if overridden {
			enabled, draining = setting.Enabled, setting.Draining || s.draining
		}

		pools := make([]string, 0, len(s.pools))
		for _, p := range s.pools {
			pools = append(pools, p.PoolID)
		}

		out = append(out, WorkerInfo{WorkerID: workerID, SessionID: s.sessionID, State: runtimeStateWithSettings(r.timings, settings, workerID, s, now), Enabled: enabled, Draining: draining, ExecutorType: s.executorType, ActiveRequests: s.activeRequests, AvailableCapacity: s.availableCapacity, MaxConcurrency: s.maxConcurrency, LastSeen: s.lastSeen(), Pools: pools})
	}

	slices.SortFunc(out, func(a, b WorkerInfo) int { return strings.Compare(a.WorkerID, b.WorkerID) })

	return out
}

func sessionInPool(session *workerSession, poolID string) bool {
	for _, pool := range session.pools {
		if pool.PoolID == poolID {
			return true
		}
	}

	return false
}

func runtimeStateForSession(timings WorkerTimings, session *workerSession, now time.Time) WorkerRuntimeState {
	staleness := now.Sub(session.lastSeen())
	if staleness > timings.DeadTimeout {
		return RuntimeDead
	}

	if staleness > timings.AvailabilityTimeout {
		return RuntimeUnavailable
	}

	if !session.hasHeartbeat {
		return RuntimeRegistered
	}

	if session.draining {
		return RuntimeDraining
	}

	if now.Before(session.cooldownUntil) {
		return RuntimeCooldown
	}

	return healthRuntimeState(session.health)
}

func healthRuntimeState(health strawpb.WorkerHealth) WorkerRuntimeState {
	switch health {
	case strawpb.WorkerHealth_WORKER_HEALTH_UNSPECIFIED:
		return RuntimeRegistered
	case strawpb.WorkerHealth_WORKER_HEALTH_READY:
		return RuntimeReady
	case strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED:
		return RuntimeDegraded
	case strawpb.WorkerHealth_WORKER_HEALTH_UNHEALTHY:
		return RuntimeUnhealthy
	default:
		return RuntimeRegistered
	}
}

// WorkerRegistryStats is an aggregate metrics snapshot.
type WorkerRegistryStats struct {
	Sessions               int
	Available              int
	MaxHeartbeatAgeSeconds float64
	ActiveRequests         uint64
	MaxConcurrency         uint64
	AvailableCapacity      uint64
}

// Stats returns current aggregate worker counts and capacity.
func (r *WorkerRegistry) Stats() WorkerRegistryStats {
	if r.shared != nil {
		workers, err := r.shared.workers(r.ctx)
		if err != nil {
			return WorkerRegistryStats{}
		}

		return r.statsFromShared(workers)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()

	stats := WorkerRegistryStats{Sessions: len(r.workers)}
	for workerID, session := range r.workers {
		state := r.runtimeState(workerID, session, now)
		if state == RuntimeReady || state == RuntimeDegraded {
			stats.Available++
		}

		if session.hasHeartbeat {
			stats.MaxHeartbeatAgeSeconds = max(stats.MaxHeartbeatAgeSeconds, now.Sub(session.lastHeartbeat).Seconds())
		}

		stats.ActiveRequests += uint64(session.activeRequests)
		stats.MaxConcurrency += uint64(session.maxConcurrency)
		stats.AvailableCapacity += uint64(session.availableCapacity)
	}

	return stats
}

func (r *WorkerRegistry) statsFromShared(workers map[string]sharedWorker) WorkerRegistryStats {
	r.mu.Lock()

	settings := make(map[string]config.WorkerSetting, len(r.settings))
	maps.Copy(settings, r.settings)
	r.mu.Unlock()
	now := r.now()

	stats := WorkerRegistryStats{Sessions: len(workers)}
	for id, w := range workers {
		s := w.session()

		state := runtimeStateWithSettings(r.timings, settings, id, s, now)
		if state == RuntimeReady || state == RuntimeDegraded {
			stats.Available++
		}

		if s.hasHeartbeat {
			stats.MaxHeartbeatAgeSeconds = max(stats.MaxHeartbeatAgeSeconds, now.Sub(s.lastHeartbeat).Seconds())
		}

		stats.ActiveRequests += uint64(s.activeRequests)
		stats.MaxConcurrency += uint64(s.maxConcurrency)
		stats.AvailableCapacity += uint64(s.availableCapacity)
	}

	return stats
}

func (r *WorkerRegistry) runtimeState(workerID string, session *workerSession, now time.Time) WorkerRuntimeState {
	return runtimeStateWithSettings(r.timings, r.settings, workerID, session, now)
}

func runtimeStateWithSettings(timings WorkerTimings, settings map[string]config.WorkerSetting, workerID string, session *workerSession, now time.Time) WorkerRuntimeState {
	if setting, ok := settings[workerID]; ok {
		if !setting.Enabled {
			return RuntimeDisabled
		}

		if setting.Draining {
			return RuntimeDraining
		}
	}

	return runtimeStateForSession(timings, session, now)
}

func newWorkerSessionID() (string, error) {
	raw := make([]byte, randomSessionIDBytes)

	_, err := rand.Read(raw)
	if err != nil {
		return "", fmt.Errorf("generate worker session id: %w", err)
	}

	return hex.EncodeToString(raw), nil
}

func validWorkerID(workerID string) bool {
	return natsx.ValidateSubjectToken(workerID) == nil
}
