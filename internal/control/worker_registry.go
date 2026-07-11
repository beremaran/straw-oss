package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/natsx"
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
	PoolID string
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
	mu      sync.Mutex
	now     func() time.Time
	timings WorkerTimings
	workers map[string]*workerSession
}

// NewDeploymentWorkerRegistry creates the in-process worker registry.
func NewDeploymentWorkerRegistry(timings WorkerTimings, now func() time.Time) *WorkerRegistry {
	if now == nil {
		now = time.Now
	}

	return &WorkerRegistry{now: now, timings: timings, workers: make(map[string]*workerSession)}
}

// Register creates or replaces one worker session.
func (r *WorkerRegistry) Register(_ context.Context, req *strawpb.RegisterRequest) (RegisterOutcome, error) {
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

	pools := deploymentPools(req.GetAllowedPools())
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

	return RegisterOutcome{OK: true, SessionID: sessionID}, nil
}

func rejectFingerprintProfileCapabilities(req *strawpb.RegisterRequest) string {
	profiles := req.GetSupportedFingerprintProfiles()
	if len(profiles) == 0 {
		return ""
	}

	if req.GetProtocolMinor() < 1 || len(profiles) != 1 || profiles[0] != fingerprintProfileChrome120 {
		return rejectCapabilityScope
	}

	return ""
}

func deploymentPools(refs []*strawpb.RegisterRequest_PoolRef) []AllowedPool {
	if len(refs) == 0 {
		return []AllowedPool{{PoolID: config.DefaultPoolID}}
	}

	pools := make([]AllowedPool, 0, len(refs))
	for _, ref := range refs {
		poolID := ref.GetPoolId()
		if poolID == "" {
			poolID = config.DefaultPoolID
		}

		pools = append(pools, AllowedPool{PoolID: poolID})
	}

	return pools
}

// Heartbeat refreshes one current worker session.
func (r *WorkerRegistry) Heartbeat(hb *strawpb.HeartbeatRequest) (bool, error) {
	if hb == nil {
		return false, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session := r.workers[hb.GetWorkerId()]
	if session == nil || session.sessionID != hb.GetSessionId() {
		return false, nil
	}

	session.hasHeartbeat = true

	session.lastHeartbeat = r.now()
	if hb.Health.Valid() && hb.GetHealth() != strawpb.WorkerHealth_WORKER_HEALTH_UNSPECIFIED {
		session.health = hb.GetHealth()
	}

	session.activeRequests = hb.GetActiveRequests()

	session.availableCapacity = hb.GetAvailableCapacity()
	if hb.GetMaxConcurrency() > 0 {
		session.maxConcurrency = hb.GetMaxConcurrency()
	}

	session.draining = hb.GetDraining()

	return true, nil
}

// RecordFailure contributes to a worker's short cooldown circuit breaker.
func (r *WorkerRegistry) RecordFailure(workerID string) {
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
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()

	candidates := make([]PoolCandidate, 0, len(r.workers))
	for workerID, session := range r.workers {
		state := runtimeStateForSession(r.timings, session, now)
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
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()

	stats := WorkerRegistryStats{Sessions: len(r.workers)}
	for _, session := range r.workers {
		state := runtimeStateForSession(r.timings, session, now)
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
