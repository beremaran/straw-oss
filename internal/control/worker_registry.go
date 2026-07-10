package control

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"maps"
	"sync"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const (
	workerAvailabilityTimeout   = 15 * time.Second
	workerDeadTimeout           = 30 * time.Second
	workerDuplicateSessionGrace = 10 * time.Second
	workerCooldownFailureCount  = 3
	workerCooldownWindow        = 60 * time.Second
	workerCooldownDuration      = 30 * time.Second

	// defaultRegistrationClockSkew and defaultRegistrationNonceTTL are the
	// docs/planning/27 defaults applied when SetNonceStore is never called
	// (e.g. most unit tests) or is called with zero-value durations.
	defaultRegistrationClockSkew = 60 * time.Second
	defaultRegistrationNonceTTL  = 5 * time.Minute
)

// WorkerRegistrationPolicy configures registration replay protection
// (docs/planning/27-security-controls.md "Worker Credential Signing").
type WorkerRegistrationPolicy struct {
	// ClockSkew bounds how far a signed issued-at timestamp may differ from
	// Control's receive time before registration is rejected.
	ClockSkew time.Duration
	// NonceTTL is how long a consumed nonce is remembered before it may be
	// reused; must exceed ClockSkew*2 to guarantee no replay window opens
	// before a nonce would naturally fall outside the skew tolerance anyway.
	NonceTTL time.Duration
	// FailOpenOnNonceStoreError allows registration to proceed without
	// replay protection when the nonce store errors (e.g. Redis outage).
	// Disabled by default: docs/planning/27 requires fail-closed unless a
	// deployment explicitly opts in.
	FailOpenOnNonceStoreError bool
}

// DefaultWorkerRegistrationPolicy returns the docs/planning/27 default
// clock-skew tolerance and nonce TTL, fail-closed.
func DefaultWorkerRegistrationPolicy() WorkerRegistrationPolicy {
	return WorkerRegistrationPolicy{ClockSkew: defaultRegistrationClockSkew, NonceTTL: defaultRegistrationNonceTTL}
}

// ProtocolMajor is the worker protocol major version Control speaks. A worker
// registering with a different major is rejected as incompatible. Minor
// versions are forward/backward tolerated in P0.
const ProtocolMajor uint32 = 1

// WorkerRuntimeState is the ephemeral, session-derived worker state
// (docs/planning/11-worker-discovery-and-health.md). It is never persisted as
// control-plane config.
type WorkerRuntimeState string

const (
	// RuntimeUnregistered means the worker has no live session.
	RuntimeUnregistered WorkerRuntimeState = "unregistered"
	// RuntimeRegistered means the worker registered but has not heartbeated.
	RuntimeRegistered WorkerRuntimeState = "registered"
	// RuntimeReady means the worker is healthy and available.
	RuntimeReady WorkerRuntimeState = "ready"
	// RuntimeDegraded means the worker is still eligible but degraded.
	RuntimeDegraded WorkerRuntimeState = "degraded"
	// RuntimeUnhealthy means the worker is unhealthy and not eligible.
	RuntimeUnhealthy WorkerRuntimeState = "unhealthy"
	// RuntimeUnavailable means the worker has gone stale but not yet dead.
	RuntimeUnavailable WorkerRuntimeState = "unavailable"
	// RuntimeDead means the worker session has expired.
	RuntimeDead WorkerRuntimeState = "dead"
	// RuntimeDraining means the worker is draining and should not receive new work.
	RuntimeDraining WorkerRuntimeState = "draining"
	// RuntimeCooldown means the worker is temporarily excluded after failures.
	RuntimeCooldown WorkerRuntimeState = "cooldown"
	// RuntimeDuplicateReplaced means a newer session replaced the old one.
	RuntimeDuplicateReplaced WorkerRuntimeState = "duplicate_replaced"
)

// AdminState is a durable admin override (global or per-tenant). It survives
// worker session churn.
type AdminState string

const (
	// AdminEnabled means the override allows the worker.
	AdminEnabled AdminState = "enabled"
	// AdminDisabled means the override blocks the worker.
	AdminDisabled AdminState = "disabled"
)

// WorkerTimings holds the health/liveness thresholds from the defaults table
// in docs/planning/11-worker-discovery-and-health.md.
type WorkerTimings struct {
	AvailabilityTimeout   time.Duration // excluded from new assignments after this staleness
	DeadTimeout           time.Duration // runtime session removed after this staleness
	DuplicateSessionGrace time.Duration // old session drains after replacement
	CooldownFailureCount  int           // failures within the window that trigger cooldown
	CooldownWindow        time.Duration // sliding window for counting failures
	CooldownDuration      time.Duration // exclusion period once triggered
}

// DefaultWorkerTimings returns the P0 default thresholds.
func DefaultWorkerTimings() WorkerTimings {
	return WorkerTimings{
		AvailabilityTimeout:   workerAvailabilityTimeout,
		DeadTimeout:           workerDeadTimeout,
		DuplicateSessionGrace: workerDuplicateSessionGrace,
		CooldownFailureCount:  workerCooldownFailureCount,
		CooldownWindow:        workerCooldownWindow,
		CooldownDuration:      workerCooldownDuration,
	}
}

// Registration rejection reasons. These are stable identifiers surfaced in
// the RegisterAck error field and asserted by tests.
const (
	RejectInvalidWorkerID    = "invalid_worker_id"
	RejectUnknownCredential  = "unknown_credential"
	RejectRevokedCredential  = "revoked_worker_key"
	RejectExecutorMismatch   = "executor_type_mismatch"
	RejectTenantScope        = "tenant_scope"
	RejectPoolScope          = "pool_out_of_scope"
	RejectCapabilityScope    = "capability_out_of_scope"
	RejectIncompatibleProto  = "incompatible_protocol"
	RejectInvalidSignature   = "invalid_signature"
	RejectInvalidKeyMaterial = "invalid_key_material"
	// RejectStaleIssuedAt means the signed issued-at timestamp is outside the
	// configured clock-skew tolerance (docs/planning/27).
	RejectStaleIssuedAt = "stale_issued_at"
	// RejectNonceReplayed means the signed nonce was already consumed for
	// this credential (docs/planning/27 replay protection).
	RejectNonceReplayed = "nonce_replayed"
	// RejectNonceStoreUnavailable means the nonce store could not be reached
	// and the registry's fail policy is fail-closed (the default).
	RejectNonceStoreUnavailable = "nonce_store_unavailable"
	randomIDBytes               = 16
)

// RegisterOutcome is the result of processing a RegisterRequest. OK is false
// for any validation failure, with Reason set to one of the Reject* codes.
type RegisterOutcome struct {
	OK        bool
	SessionID string
	Reason    string
}

// runtimeSession is one ephemeral worker session.
type runtimeSession struct {
	sessionID                    string
	executorType                 string
	credentialID                 string
	tenantScope                  []string
	pools                        []AllowedPool
	tags                         []string
	countries                    []string
	regions                      []string
	ipTypes                      []string
	ingressModes                 []string
	supportedFingerprintProfiles []string
	maxConcurrency               uint32

	health         strawpb.WorkerHealth
	hasHeartbeat   bool
	activeRequests uint32
	availableCap   uint32
	draining       bool
	registeredAt   time.Time
	lastHeartbeat  time.Time
}

// lastSeen returns the most recent liveness signal (receive time), falling
// back to registration time before the first heartbeat.
func (s *runtimeSession) lastSeen() time.Time {
	if s.hasHeartbeat {
		return s.lastHeartbeat
	}

	return s.registeredAt
}

// workerEntry holds the durable admin state plus the current and superseded
// runtime sessions for one worker_id.
type workerEntry struct {
	globalAdmin AdminState
	globalDrain bool
	tenantAdmin map[string]AdminState
	tenantDrain map[string]bool

	current    *runtimeSession
	superseded *runtimeSession

	failures      []time.Time
	cooldownUntil time.Time
}

// WorkerRuntimeStore stores ephemeral worker runtime state outside the
// Control process. Redis is the production P0 implementation; nil means the
// registry uses only its bounded local snapshot for tests.
type WorkerRuntimeStore interface {
	save(workerID string, e *workerEntry, ttl time.Duration) error
	loadAll() (map[string]*workerEntry, error)
}

// WorkerRegistry tracks worker registration, heartbeat-derived runtime state,
// duplicate/stale session handling, cooldown, and admin overrides. Runtime
// state can be backed by Redis with TTLs while admin overrides remain durable
// config rehydrated from Postgres.
type WorkerRegistry struct {
	mu        sync.Mutex
	now       func() time.Time
	timings   WorkerTimings
	creds     WorkerCredentialStore
	workers   map[string]*workerEntry
	runtime   WorkerRuntimeStore
	events    WorkerEventRecorder
	nonces    WorkerNonceStore
	regPolicy WorkerRegistrationPolicy
}

// NewWorkerRegistry builds a registry. now may be nil (defaults to
// time.Now); tests inject a controllable clock. Registration replay
// protection uses DefaultWorkerRegistrationPolicy with no nonce store (skew
// enforced, replay/outage checks skipped) until SetNonceStore is called.
func NewWorkerRegistry(creds WorkerCredentialStore, timings WorkerTimings, now func() time.Time) *WorkerRegistry {
	if now == nil {
		now = time.Now
	}

	return &WorkerRegistry{
		now:       now,
		timings:   timings,
		creds:     creds,
		workers:   make(map[string]*workerEntry),
		regPolicy: DefaultWorkerRegistrationPolicy(),
	}
}

// SetRuntimeStore wires the Redis-backed worker session/heartbeat/load and
// cooldown state store (docs/planning/21). Redis outage degrades to the
// registry's local snapshot; a fresh registry with no snapshot fails safe by
// seeing no available workers.
func (r *WorkerRegistry) SetRuntimeStore(store WorkerRuntimeStore) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.runtime = store
	r.refreshRuntimeLocked()
}

// SetEventRecorder wires the worker_events ClickHouse sink (docs/tasks/p0/32).
// Optional: nil (the default) disables worker_events emission without
// affecting registration/heartbeat/admin behavior.
func (r *WorkerRegistry) SetEventRecorder(rec WorkerEventRecorder) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = rec
}

// SetNonceStore wires the Redis-backed registration nonce store and its
// replay-protection policy (docs/planning/27-security-controls.md). Zero
// values in policy fall back to DefaultWorkerRegistrationPolicy's durations.
// Optional: if never called, ClockSkew stays at its default (enforced) and
// nonce replay/outage checks are skipped since there is no store to consult.
func (r *WorkerRegistry) SetNonceStore(store WorkerNonceStore, policy WorkerRegistrationPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nonces = store

	if policy.ClockSkew > 0 {
		r.regPolicy.ClockSkew = policy.ClockSkew
	}

	if policy.NonceTTL > 0 {
		r.regPolicy.NonceTTL = policy.NonceTTL
	}

	r.regPolicy.FailOpenOnNonceStoreError = policy.FailOpenOnNonceStoreError
}

// Register validates a RegisterRequest against the referenced worker
// credential and, on success, replaces any existing session with a fresh one
// and returns the new session_id.
func (r *WorkerRegistry) Register(ctx context.Context, req *strawpb.RegisterRequest) (RegisterOutcome, error) {
	if req == nil {
		return RegisterOutcome{Reason: RejectInvalidWorkerID}, nil
	}

	err := natsx.ValidateSubjectToken(req.GetWorkerId())
	if err != nil {
		return RegisterOutcome{Reason: RejectInvalidWorkerID}, nil
	}

	cred, err := r.creds.Get(ctx, req.GetCredentialId())
	if err != nil {
		return RegisterOutcome{Reason: RejectUnknownCredential}, nil
	}

	reason := rejectRegisterRequest(cred, req)
	if reason != "" {
		return RegisterOutcome{Reason: reason}, nil
	}

	reason, err = r.checkRegistrationReplay(ctx, cred.ID, req)
	if err != nil {
		return RegisterOutcome{}, err
	}

	if reason != "" {
		return RegisterOutcome{Reason: reason}, nil
	}

	sessionID, err := newSessionID()
	if err != nil {
		return RegisterOutcome{}, err
	}

	session := newRuntimeSession(sessionID, cred, req, r.now())

	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.entry(req.GetWorkerId())
	if e.current != nil {
		// Duplicate registration: the prior session is superseded and
		// receives no new assignments (docs/planning/11 "Duplicate Sessions").
		e.superseded = e.current
		e.superseded.draining = true
	}

	e.current = session
	r.persistRuntimeLocked(req.GetWorkerId(), e)

	r.emitWorkerEvent(WorkerEvent{
		WorkerID:     req.GetWorkerId(),
		SessionID:    sessionID,
		ExecutorType: session.executorType,
		EventType:    workerEventRegister,
		Health:       workerHealthLabel(session.health),
		Draining:     drainingFlag(session.draining),
	})

	return RegisterOutcome{OK: true, SessionID: sessionID}, nil
}

// newRuntimeSession builds the ephemeral session state for a successful
// registration.
func newRuntimeSession(sessionID string, cred WorkerCredential, req *strawpb.RegisterRequest, now time.Time) *runtimeSession {
	return &runtimeSession{
		sessionID:                    sessionID,
		executorType:                 req.GetExecutorType(),
		credentialID:                 req.GetCredentialId(),
		tenantScope:                  append([]string(nil), cred.TenantScope...),
		pools:                        append([]AllowedPool(nil), poolRefsToAllowed(req.GetAllowedPools())...),
		tags:                         append([]string(nil), req.GetTags()...),
		countries:                    append([]string(nil), req.GetCountries()...),
		regions:                      append([]string(nil), req.GetRegions()...),
		ipTypes:                      append([]string(nil), req.GetIpTypes()...),
		ingressModes:                 append([]string(nil), req.GetSupportedIngressModes()...),
		supportedFingerprintProfiles: append([]string(nil), req.GetSupportedFingerprintProfiles()...),
		maxConcurrency:               req.GetMaxConcurrency(),
		health:                       strawpb.WorkerHealth_WORKER_HEALTH_READY,
		draining:                     req.GetInitialDraining(),
		registeredAt:                 now,
	}
}

// Heartbeat records a heartbeat. A heartbeat for the current session updates
// its liveness (using Control receive time, not worker time) and health. A
// heartbeat for a known-but-stale session is accepted for diagnostics but
// does not affect routing. An unknown worker/session is rejected.
func (r *WorkerRegistry) Heartbeat(hb *strawpb.HeartbeatRequest) (bool, error) {
	if hb == nil {
		return false, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	e, ok := r.workers[hb.GetWorkerId()]
	if !ok || e.current == nil {
		return false, nil
	}

	if e.current.sessionID != hb.GetSessionId() {
		// Stale (or superseded) session: recognized, ignored for routing.
		if e.superseded != nil && e.superseded.sessionID == hb.GetSessionId() {
			return true, nil
		}

		return false, nil
	}

	s := e.current
	s.hasHeartbeat = true

	s.lastHeartbeat = r.now()
	if hb.Health.Valid() && hb.GetHealth() != strawpb.WorkerHealth_WORKER_HEALTH_UNSPECIFIED {
		s.health = hb.GetHealth()
	}

	s.activeRequests = hb.GetActiveRequests()

	s.availableCap = hb.GetAvailableCapacity()
	if hb.GetMaxConcurrency() > 0 {
		s.maxConcurrency = hb.GetMaxConcurrency()
	}

	s.draining = hb.GetDraining()
	r.persistRuntimeLocked(hb.GetWorkerId(), e)

	r.emitWorkerEvent(WorkerEvent{
		WorkerID:          hb.GetWorkerId(),
		SessionID:         s.sessionID,
		ExecutorType:      s.executorType,
		EventType:         workerEventHeartbeat,
		Health:            workerHealthLabel(s.health),
		ActiveRequests:    s.activeRequests,
		MaxConcurrency:    s.maxConcurrency,
		AvailableCapacity: s.availableCap,
		Draining:          drainingFlag(s.draining),
	})

	return true, nil
}

// RecordFailure records a worker failure for cooldown accounting. When the
// failure count reaches the configured threshold within the window, the
// worker enters cooldown for the configured duration.
func (r *WorkerRegistry) RecordFailure(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	e, ok := r.workers[workerID]
	if !ok {
		return
	}

	now := r.now()
	cutoff := now.Add(-r.timings.CooldownWindow)

	kept := e.failures[:0]
	for _, t := range e.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	kept = append(kept, now)
	e.failures = kept

	if len(e.failures) >= r.timings.CooldownFailureCount {
		e.cooldownUntil = now.Add(r.timings.CooldownDuration)
		e.failures = nil
	}

	r.persistRuntimeLocked(workerID, e)
}

// EligibleForTenant reports whether the worker may receive new assignments
// for tenantID. It applies the exclusion precedence from docs/planning/11:
// global disable overrides everything, then tenant disable, then drain, then
// runtime health. Degraded workers are eligible here; pool-level degraded
// policy is a routing (task 09) concern.
func (r *WorkerRegistry) EligibleForTenant(workerID, tenantID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	e, ok := r.workers[workerID]
	if !ok || e.current == nil {
		return false
	}

	if !eligibleForTenantAdmin(e, tenantID) {
		return false
	}

	state := r.runtimeState(e, r.now())

	return state == RuntimeReady || state == RuntimeDegraded
}

// PoolCandidate is one worker session eligible (per admin/runtime state) for
// assignment in a specific tenant+pool, with the capability and load data
// routing needs to filter and rank it.
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

// CandidatesForPool returns every worker session eligible for tenantID and
// scoped to poolID, applying the same admin/runtime exclusion precedence as
// EligibleForTenant (global disable, tenant disable, drain, cooldown,
// runtime health) plus pool scope. Capability matching and load ranking are
// left to the caller (routing, task 09).
func (r *WorkerRegistry) CandidatesForPool(tenantID, poolID string) []PoolCandidate {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	now := r.now()
	out := make([]PoolCandidate, 0)

	for workerID, e := range r.workers {
		s := e.current
		if s == nil || !containsString(s.tenantScope, tenantID) {
			continue
		}

		if !workerInPool(s.pools, tenantID, poolID) {
			continue
		}

		if !eligibleForTenantAdmin(e, tenantID) {
			continue
		}

		state := runtimeStateForSession(r.timings, e.cooldownUntil, s, now)
		if state != RuntimeReady && state != RuntimeDegraded {
			continue
		}

		subject, err := natsx.AssignmentSubject(workerID, s.sessionID)
		if err != nil {
			continue
		}

		out = append(out, PoolCandidate{
			WorkerID:                     workerID,
			SessionID:                    s.sessionID,
			AssignSubject:                subject,
			ExecutorType:                 s.executorType,
			Degraded:                     state == RuntimeDegraded,
			Tags:                         s.tags,
			Countries:                    s.countries,
			Regions:                      s.regions,
			IPTypes:                      s.ipTypes,
			IngressModes:                 s.ingressModes,
			SupportedFingerprintProfiles: append([]string(nil), s.supportedFingerprintProfiles...),
			ActiveRequests:               s.activeRequests,
			MaxConcurrency:               s.maxConcurrency,
			AvailableCap:                 s.availableCap,
		})
	}

	return out
}

func rejectRegisterRequest(cred WorkerCredential, req *strawpb.RegisterRequest) string {
	if cred.Status != WorkerCredentialStatusActive {
		return RejectRevokedCredential
	}

	reason := rejectRegisterRequestScope(cred, req)
	if reason != "" {
		return reason
	}

	reason = rejectRegisterRequestCapabilities(cred, req)
	if reason != "" {
		return reason
	}

	return rejectRegisterRequestSignature(cred, req)
}

func rejectRegisterRequestScope(cred WorkerCredential, req *strawpb.RegisterRequest) string {
	if cred.ExecutorType != "" && req.GetExecutorType() != "" && cred.ExecutorType != req.GetExecutorType() {
		return RejectExecutorMismatch
	}

	if req.GetProtocolMajor() != ProtocolMajor {
		return RejectIncompatibleProto
	}

	for _, p := range req.GetAllowedPools() {
		if !credentialAllowsPool(cred, p.GetTenantId(), p.GetPoolId()) {
			return RejectPoolScope
		}
	}

	return ""
}

func rejectRegisterRequestCapabilities(cred WorkerCredential, req *strawpb.RegisterRequest) string {
	caps := cred.AllowedCapabilities
	if !subset(req.GetTags(), caps.Tags) ||
		!subset(req.GetCountries(), caps.Countries) ||
		!subset(req.GetRegions(), caps.Regions) ||
		!subset(req.GetIpTypes(), caps.IPTypes) ||
		!subset(req.GetSupportedIngressModes(), caps.SupportedIngressModes) ||
		!subset(req.GetSupportedFingerprintProfiles(), caps.SupportedFingerprintProfiles) {
		return RejectCapabilityScope
	}

	return ""
}

func rejectRegisterRequestSignature(cred WorkerCredential, req *strawpb.RegisterRequest) string {
	pub, err := base64.StdEncoding.DecodeString(cred.PublicKeyEd25519Base64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return RejectInvalidKeyMaterial
	}

	if !strawpb.VerifyRegistrationSignature(ed25519.PublicKey(pub), req, req.GetSignedToken()) {
		return RejectInvalidSignature
	}

	return ""
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
}

func eligibleForTenantAdmin(e *workerEntry, tenantID string) bool {
	if !containsString(e.current.tenantScope, tenantID) {
		return false
	}

	if e.globalAdmin == AdminDisabled {
		return false
	}

	if e.tenantAdmin[tenantID] == AdminDisabled {
		return false
	}

	return !e.globalDrain && !e.tenantDrain[tenantID]
}

// RuntimeState returns the current runtime state for a worker (unregistered
// if unknown or with no active session).
func (r *WorkerRegistry) RuntimeState(workerID string) WorkerRuntimeState {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	e, ok := r.workers[workerID]
	if !ok {
		return RuntimeUnregistered
	}

	return r.runtimeState(e, r.now())
}

// WorkerRegistryStats is a point-in-time aggregate snapshot for the
// Prometheus worker gauges (docs/planning/23-observability.md). It carries
// no per-worker labels, since worker_id is not one of the P0 allowed metric
// labels.
type WorkerRegistryStats struct {
	// Sessions counts workers with a current (registered) session.
	Sessions int
	// Available counts sessions currently eligible for new assignments
	// (runtime state ready or degraded).
	Available int
	// MaxHeartbeatAgeSeconds is the staleness of the oldest current
	// heartbeat among sessions that have heartbeated at least once, or 0
	// when none have.
	MaxHeartbeatAgeSeconds float64
	// ActiveRequests is the aggregate active request count reported by
	// current worker heartbeats.
	ActiveRequests uint64
	// MaxConcurrency is the aggregate max concurrency reported by current
	// worker registration/heartbeat state.
	MaxConcurrency uint64
	// AvailableCapacity is the aggregate available capacity reported by
	// current worker heartbeats.
	AvailableCapacity uint64
}

// Stats computes the aggregate worker session/availability/heartbeat-age
// snapshot used by the Prometheus worker collector.
func (r *WorkerRegistry) Stats() WorkerRegistryStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	now := r.now()

	var stats WorkerRegistryStats

	for _, e := range r.workers {
		s := e.current
		if s == nil {
			continue
		}

		stats.Sessions++

		state := runtimeStateForSession(r.timings, e.cooldownUntil, s, now)
		if state == RuntimeReady || state == RuntimeDegraded {
			stats.Available++
		}

		if s.hasHeartbeat {
			age := now.Sub(s.lastHeartbeat).Seconds()
			if age > stats.MaxHeartbeatAgeSeconds {
				stats.MaxHeartbeatAgeSeconds = age
			}
		}

		stats.ActiveRequests += uint64(s.activeRequests)
		stats.MaxConcurrency += uint64(s.maxConcurrency)
		stats.AvailableCapacity += uint64(s.availableCap)
	}

	return stats
}

func runtimeStateForSession(timings WorkerTimings, cooldownUntil time.Time, s *runtimeSession, now time.Time) WorkerRuntimeState {
	staleness := now.Sub(s.lastSeen())
	if staleness > timings.DeadTimeout {
		return RuntimeDead
	}

	if staleness > timings.AvailabilityTimeout {
		return RuntimeUnavailable
	}

	if !s.hasHeartbeat {
		return RuntimeRegistered
	}

	if s.draining {
		return RuntimeDraining
	}

	if now.Before(cooldownUntil) {
		return RuntimeCooldown
	}

	return healthState(s.health)
}

func healthState(health strawpb.WorkerHealth) WorkerRuntimeState {
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
		return RuntimeReady
	}
}

func workerInPool(pools []AllowedPool, tenantID, poolID string) bool {
	for _, p := range pools {
		if p.TenantID == tenantID && p.PoolID == poolID {
			return true
		}
	}

	return false
}

// ---- admin actions (durable overrides) ----

// SetGlobalAdmin sets the durable platform admin override for a worker.
func (r *WorkerRegistry) SetGlobalAdmin(workerID string, state AdminState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.entry(workerID)
	e.globalAdmin = state

	eventType := workerEventEnable
	if state == AdminDisabled {
		eventType = workerEventDisable
	}

	r.emitTransitionEvent(workerID, "", eventType, e)
}

// SetGlobalDrain sets the durable platform drain flag for a worker.
func (r *WorkerRegistry) SetGlobalDrain(workerID string, draining bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.entry(workerID)
	e.globalDrain = draining

	eventType := workerEventUndrain
	if draining {
		eventType = workerEventDrain
	}

	r.emitTransitionEvent(workerID, "", eventType, e)
}

// SetTenantAdmin sets the durable per-tenant admin override for a worker.
func (r *WorkerRegistry) SetTenantAdmin(workerID, tenantID string, state AdminState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.entry(workerID)
	e.tenantAdmin[tenantID] = state

	eventType := workerEventTenantEnable
	if state == AdminDisabled {
		eventType = workerEventTenantDisable
	}

	r.emitTransitionEvent(workerID, tenantID, eventType, e)
}

// SetTenantDrain sets the per-tenant drain flag for a worker.
func (r *WorkerRegistry) SetTenantDrain(workerID, tenantID string, draining bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := r.entry(workerID)
	e.tenantDrain[tenantID] = draining

	eventType := workerEventTenantUndrain
	if draining {
		eventType = workerEventTenantDrain
	}

	r.emitTransitionEvent(workerID, tenantID, eventType, e)
}

// KnownWorker reports whether the registry has any state for workerID.
func (r *WorkerRegistry) KnownWorker(workerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.workers[workerID]

	return ok
}

// ---- listing ----

// WorkerView is a read model of one worker's state for the admin API. Fields
// that must never be exposed to tenant-scoped callers (SessionID, NATS
// subjects) are only populated by the platform-scoped listing.
type WorkerView struct {
	WorkerID         string
	RuntimeState     WorkerRuntimeState
	GlobalAdminState AdminState
	GlobalDraining   bool
	// TenantAdmin/TenantDrain hold the full per-tenant maps for platform
	// callers; the tenant-scoped listing collapses these to the caller's
	// tenant only.
	TenantAdmin                  map[string]AdminState
	TenantDrain                  map[string]bool
	SupportedFingerprintProfiles []string
	// Platform-only fields.
	SessionID       string
	ExecutorType    string
	AssignSubject   string
	SoftwareVersion string
}

// ListWorkersPlatform returns every worker with full runtime, global admin,
// per-tenant admin state, session_id, and NATS subjects. Platform scope only.
func (r *WorkerRegistry) ListWorkersPlatform() []WorkerView {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	now := r.now()

	out := make([]WorkerView, 0, len(r.workers))
	for workerID, e := range r.workers {
		v := WorkerView{
			WorkerID:         workerID,
			RuntimeState:     r.runtimeState(e, now),
			GlobalAdminState: e.globalAdmin,
			GlobalDraining:   e.globalDrain,
			TenantAdmin:      copyAdminMap(e.tenantAdmin),
			TenantDrain:      copyBoolMap(e.tenantDrain),
		}
		if e.current != nil {
			v.SessionID = e.current.sessionID

			v.ExecutorType = e.current.executorType
			v.SupportedFingerprintProfiles = append([]string(nil), e.current.supportedFingerprintProfiles...)

			subject, err := natsx.AssignmentSubject(workerID, e.current.sessionID)
			if err == nil {
				v.AssignSubject = subject
			}
		}

		out = append(out, v)
	}

	return out
}

// ListWorkersForTenant returns only workers eligible for tenantID, never
// exposing session_id or NATS subjects, and collapsing per-tenant admin
// state to that tenant only (docs/planning/26 "GET /workers").
func (r *WorkerRegistry) ListWorkersForTenant(tenantID string) []WorkerView {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshRuntimeLocked()

	now := r.now()
	out := make([]WorkerView, 0)

	for workerID, e := range r.workers {
		if e.current == nil || !containsString(e.current.tenantScope, tenantID) {
			continue
		}
		// A globally disabled worker still appears here: a tenant admin needs
		// to see workers they can act on. We surface only this tenant's admin
		// overrides and never a session_id or NATS subject.
		v := WorkerView{
			WorkerID:                     workerID,
			RuntimeState:                 r.runtimeState(e, now),
			GlobalAdminState:             e.globalAdmin,
			GlobalDraining:               e.globalDrain,
			TenantAdmin:                  map[string]AdminState{tenantID: tenantAdminState(e, tenantID)},
			TenantDrain:                  map[string]bool{tenantID: e.tenantDrain[tenantID]},
			ExecutorType:                 e.current.executorType,
			SupportedFingerprintProfiles: append([]string(nil), e.current.supportedFingerprintProfiles...),
		}
		out = append(out, v)
	}

	return out
}

// checkRegistrationReplay enforces the docs/planning/27 issued-at skew
// tolerance and, when a nonce store is configured, consumes the signed nonce
// to reject replays. It runs only after signature verification succeeds, so
// a forged nonce/issued-at cannot be used to probe this check.
func (r *WorkerRegistry) checkRegistrationReplay(ctx context.Context, credentialID string, req *strawpb.RegisterRequest) (string, error) {
	r.mu.Lock()
	nonces := r.nonces
	policy := r.regPolicy
	r.mu.Unlock()

	issuedAt := time.UnixMilli(req.GetIssuedAtUnixMs())
	if absDuration(r.now().Sub(issuedAt)) > policy.ClockSkew {
		return RejectStaleIssuedAt, nil
	}

	if nonces == nil {
		return "", nil
	}

	fresh, err := nonces.Consume(ctx, credentialID, req.GetNonce(), policy.NonceTTL)
	if err != nil {
		if policy.FailOpenOnNonceStoreError {
			return "", nil
		}

		return RejectNonceStoreUnavailable, nil
	}

	if !fresh {
		return RejectNonceReplayed, nil
	}

	return "", nil
}

// emitWorkerEvent enqueues a worker_events row if a recorder is wired. Must
// be called while holding r.mu (recorder.Enqueue uses its own lock, so this
// never deadlocks against the registry mutex).
func (r *WorkerRegistry) emitWorkerEvent(event WorkerEvent) {
	if r.events == nil {
		return
	}

	event.Timestamp = r.now().UTC()
	r.events.Enqueue(event)
}

// emitTransitionEvent builds a worker_events row for an admin state
// transition, filling health/capacity fields from the current session when
// one exists (a worker can be disabled/drained before it ever registers).
func (r *WorkerRegistry) emitTransitionEvent(workerID, tenantID, eventType string, e *workerEntry) {
	event := WorkerEvent{TenantID: tenantID, WorkerID: workerID, EventType: eventType}

	if e.current != nil {
		event.SessionID = e.current.sessionID
		event.ExecutorType = e.current.executorType
		event.Health = workerHealthLabel(e.current.health)
		event.ActiveRequests = e.current.activeRequests
		event.MaxConcurrency = e.current.maxConcurrency
		event.AvailableCapacity = e.current.availableCap
		event.Draining = drainingFlag(e.current.draining)
	}

	r.emitWorkerEvent(event)
}

func (r *WorkerRegistry) runtimeState(e *workerEntry, now time.Time) WorkerRuntimeState {
	s := e.current
	if s == nil {
		return RuntimeUnregistered
	}

	return runtimeStateForSession(r.timings, e.cooldownUntil, s, now)
}

func (r *WorkerRegistry) refreshRuntimeLocked() {
	if r.runtime == nil {
		return
	}

	loaded, err := r.runtime.loadAll()
	if err != nil {
		return
	}

	for workerID, runtimeEntry := range loaded {
		e := r.entry(workerID)
		e.current = runtimeEntry.current
		e.superseded = runtimeEntry.superseded
		e.failures = runtimeEntry.failures
		e.cooldownUntil = runtimeEntry.cooldownUntil
	}
}

func (r *WorkerRegistry) persistRuntimeLocked(workerID string, e *workerEntry) {
	if r.runtime == nil {
		return
	}

	_ = r.runtime.save(workerID, e, r.runtimeTTL())
}

func (r *WorkerRegistry) runtimeTTL() time.Duration {
	return max(r.timings.DeadTimeout, r.timings.CooldownDuration, r.timings.CooldownWindow) + time.Second
}

func (r *WorkerRegistry) entry(workerID string) *workerEntry {
	e, ok := r.workers[workerID]
	if !ok {
		e = &workerEntry{
			globalAdmin: AdminEnabled,
			tenantAdmin: make(map[string]AdminState),
			tenantDrain: make(map[string]bool),
		}
		r.workers[workerID] = e
	}

	return e
}

func tenantAdminState(e *workerEntry, tenantID string) AdminState {
	if s, ok := e.tenantAdmin[tenantID]; ok {
		return s
	}

	return AdminEnabled
}

// ---- helpers ----

func credentialAllowsPool(cred WorkerCredential, tenantID, poolID string) bool {
	for _, p := range cred.AllowedPools {
		if p.TenantID == tenantID && p.PoolID == poolID {
			return true
		}
	}

	return false
}

func poolRefsToAllowed(refs []*strawpb.RegisterRequest_PoolRef) []AllowedPool {
	out := make([]AllowedPool, 0, len(refs))
	for _, p := range refs {
		out = append(out, AllowedPool{TenantID: p.GetTenantId(), PoolID: p.GetPoolId()})
	}

	return out
}

// subset reports whether every element of claimed is present in allowed. An
// empty allowed list means "no restriction" and always returns true.
func subset(claimed, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}

	set := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		set[a] = struct{}{}
	}

	for _, c := range claimed {
		if _, ok := set[c]; !ok {
			return false
		}
	}

	return true
}

func copyAdminMap(in map[string]AdminState) map[string]AdminState {
	out := make(map[string]AdminState, len(in))
	maps.Copy(out, in)

	return out
}

func copyBoolMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	maps.Copy(out, in)

	return out
}

func newSessionID() (string, error) {
	raw := make([]byte, randomIDBytes)

	_, err := rand.Read(raw)
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	return "sess_" + hex.EncodeToString(raw), nil
}
