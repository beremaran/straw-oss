package control

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

// ProtocolMajor is the worker protocol major version Control speaks. A worker
// registering with a different major is rejected as incompatible. Minor
// versions are forward/backward tolerated in P0.
const ProtocolMajor uint32 = 1

// WorkerRuntimeState is the ephemeral, session-derived worker state
// (docs/planning/11-worker-discovery-and-health.md). It is never persisted as
// control-plane config.
type WorkerRuntimeState string

const (
	RuntimeUnregistered      WorkerRuntimeState = "unregistered"
	RuntimeRegistered        WorkerRuntimeState = "registered"
	RuntimeReady             WorkerRuntimeState = "ready"
	RuntimeDegraded          WorkerRuntimeState = "degraded"
	RuntimeUnhealthy         WorkerRuntimeState = "unhealthy"
	RuntimeUnavailable       WorkerRuntimeState = "unavailable"
	RuntimeDead              WorkerRuntimeState = "dead"
	RuntimeDraining          WorkerRuntimeState = "draining"
	RuntimeCooldown          WorkerRuntimeState = "cooldown"
	RuntimeDuplicateReplaced WorkerRuntimeState = "duplicate_replaced"
)

// AdminState is a durable admin override (global or per-tenant). It survives
// worker session churn.
type AdminState string

const (
	AdminEnabled  AdminState = "enabled"
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
		AvailabilityTimeout:   15 * time.Second,
		DeadTimeout:           30 * time.Second,
		DuplicateSessionGrace: 10 * time.Second,
		CooldownFailureCount:  3,
		CooldownWindow:        60 * time.Second,
		CooldownDuration:      30 * time.Second,
	}
}

// Registration rejection reasons. These are stable identifiers surfaced in
// the RegisterAck error field and asserted by tests.
const (
	RejectInvalidWorkerID   = "invalid_worker_id"
	RejectUnknownCredential = "unknown_credential"
	RejectCredentialRevoked = "credential_revoked"
	RejectExecutorMismatch  = "executor_type_mismatch"
	RejectTenantScope       = "tenant_scope"
	RejectPoolScope         = "pool_out_of_scope"
	RejectCapabilityScope   = "capability_out_of_scope"
	RejectIncompatibleProto = "incompatible_protocol"
	RejectInvalidSignature  = "invalid_signature"
	RejectInvalidCredKey    = "invalid_credential_key"
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
	sessionID      string
	executorType   string
	credentialID   string
	tenantScope    []string
	pools          []AllowedPool
	tags           []string
	countries      []string
	regions        []string
	ipTypes        []string
	ingressModes   []string
	maxConcurrency uint32

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

// WorkerRegistry tracks worker registration, heartbeat-derived runtime state,
// duplicate/stale session handling, cooldown, and admin overrides. It is the
// P0 in-process store; runtime state is never made durable
// (docs/planning/11). Redis-backed state with TTLs is future work.
type WorkerRegistry struct {
	mu      sync.Mutex
	now     func() time.Time
	timings WorkerTimings
	creds   WorkerCredentialStore
	workers map[string]*workerEntry
}

// NewWorkerRegistry builds a registry. now may be nil (defaults to
// time.Now); tests inject a controllable clock.
func NewWorkerRegistry(creds WorkerCredentialStore, timings WorkerTimings, now func() time.Time) *WorkerRegistry {
	if now == nil {
		now = time.Now
	}
	return &WorkerRegistry{
		now:     now,
		timings: timings,
		creds:   creds,
		workers: make(map[string]*workerEntry),
	}
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

// Register validates a RegisterRequest against the referenced worker
// credential and, on success, replaces any existing session with a fresh one
// and returns the new session_id.
func (r *WorkerRegistry) Register(ctx context.Context, req *strawpb.RegisterRequest) (RegisterOutcome, error) {
	if req == nil {
		return RegisterOutcome{Reason: RejectInvalidWorkerID}, nil
	}
	if err := natsx.ValidateSubjectToken(req.GetWorkerId()); err != nil {
		return RegisterOutcome{Reason: RejectInvalidWorkerID}, nil
	}

	cred, err := r.creds.Get(ctx, req.GetCredentialId())
	if err != nil {
		return RegisterOutcome{Reason: RejectUnknownCredential}, nil
	}
	if cred.Status != WorkerCredentialStatusActive {
		return RegisterOutcome{Reason: RejectCredentialRevoked}, nil
	}
	if cred.ExecutorType != "" && req.GetExecutorType() != "" && cred.ExecutorType != req.GetExecutorType() {
		return RegisterOutcome{Reason: RejectExecutorMismatch}, nil
	}
	if req.GetProtocolMajor() != ProtocolMajor {
		return RegisterOutcome{Reason: RejectIncompatibleProto}, nil
	}

	// Pool scope: every registered pool must be granted by the credential.
	for _, p := range req.GetAllowedPools() {
		if !credentialAllowsPool(cred, p.GetTenantId(), p.GetPoolId()) {
			return RegisterOutcome{Reason: RejectPoolScope}, nil
		}
	}

	// Capability scope: each claimed capability must be inside the
	// credential's allow-list for that dimension (empty list = unrestricted).
	caps := cred.AllowedCapabilities
	if !subset(req.GetTags(), caps.Tags) ||
		!subset(req.GetCountries(), caps.Countries) ||
		!subset(req.GetRegions(), caps.Regions) ||
		!subset(req.GetIpTypes(), caps.IPTypes) ||
		!subset(req.GetSupportedIngressModes(), caps.SupportedIngressModes) {
		return RegisterOutcome{Reason: RejectCapabilityScope}, nil
	}

	// Signature: prove possession of the credential private key.
	pub, err := base64.StdEncoding.DecodeString(cred.PublicKeyEd25519Base64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return RegisterOutcome{Reason: RejectInvalidCredKey}, nil
	}
	if !strawpb.VerifyRegistrationSignature(ed25519.PublicKey(pub), req, req.GetSignedToken()) {
		return RegisterOutcome{Reason: RejectInvalidSignature}, nil
	}

	sessionID, err := newSessionID()
	if err != nil {
		return RegisterOutcome{}, err
	}

	now := r.now()
	session := &runtimeSession{
		sessionID:      sessionID,
		executorType:   req.GetExecutorType(),
		credentialID:   req.GetCredentialId(),
		tenantScope:    append([]string(nil), cred.TenantScope...),
		pools:          append([]AllowedPool(nil), poolRefsToAllowed(req.GetAllowedPools())...),
		tags:           append([]string(nil), req.GetTags()...),
		countries:      append([]string(nil), req.GetCountries()...),
		regions:        append([]string(nil), req.GetRegions()...),
		ipTypes:        append([]string(nil), req.GetIpTypes()...),
		ingressModes:   append([]string(nil), req.GetSupportedIngressModes()...),
		maxConcurrency: req.GetMaxConcurrency(),
		health:         strawpb.WorkerHealth_WORKER_HEALTH_READY,
		draining:       req.GetInitialDraining(),
		registeredAt:   now,
	}

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

	return RegisterOutcome{OK: true, SessionID: sessionID}, nil
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
	return true, nil
}

// RecordFailure records a worker failure for cooldown accounting. When the
// failure count reaches the configured threshold within the window, the
// worker enters cooldown for the configured duration.
func (r *WorkerRegistry) RecordFailure(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

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
	e.failures = append(kept, now)
	if len(e.failures) >= r.timings.CooldownFailureCount {
		e.cooldownUntil = now.Add(r.timings.CooldownDuration)
		e.failures = nil
	}
}

// runtimeState derives the current runtime state for the worker's active
// session under lock.
func (r *WorkerRegistry) runtimeState(e *workerEntry, now time.Time) WorkerRuntimeState {
	s := e.current
	if s == nil {
		return RuntimeUnregistered
	}
	staleness := now.Sub(s.lastSeen())
	if staleness > r.timings.DeadTimeout {
		return RuntimeDead
	}
	if staleness > r.timings.AvailabilityTimeout {
		return RuntimeUnavailable
	}
	if !s.hasHeartbeat {
		return RuntimeRegistered
	}
	if now.Before(e.cooldownUntil) {
		return RuntimeCooldown
	}
	if s.draining {
		return RuntimeDraining
	}
	switch s.health {
	case strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED:
		return RuntimeDegraded
	case strawpb.WorkerHealth_WORKER_HEALTH_UNHEALTHY:
		return RuntimeUnhealthy
	default:
		return RuntimeReady
	}
}

// RuntimeState returns the current runtime state for a worker (unregistered
// if unknown or with no active session).
func (r *WorkerRegistry) RuntimeState(workerID string) WorkerRuntimeState {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.workers[workerID]
	if !ok {
		return RuntimeUnregistered
	}
	return r.runtimeState(e, r.now())
}

// EligibleForTenant reports whether the worker may receive new assignments
// for tenantID. It applies the exclusion precedence from docs/planning/11:
// global disable overrides everything, then tenant disable, then drain, then
// runtime health. Degraded workers are eligible here; pool-level degraded
// policy is a routing (task 09) concern.
func (r *WorkerRegistry) EligibleForTenant(workerID, tenantID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.workers[workerID]
	if !ok || e.current == nil {
		return false
	}
	if !containsString(e.current.tenantScope, tenantID) {
		return false
	}
	if e.globalAdmin == AdminDisabled {
		return false
	}
	if e.tenantAdmin[tenantID] == AdminDisabled {
		return false
	}
	if e.globalDrain || e.tenantDrain[tenantID] {
		return false
	}
	switch r.runtimeState(e, r.now()) {
	case RuntimeReady, RuntimeDegraded:
		return true
	default:
		return false
	}
}

// PoolCandidate is one worker session eligible (per admin/runtime state) for
// assignment in a specific tenant+pool, with the capability and load data
// routing needs to filter and rank it.
type PoolCandidate struct {
	WorkerID       string
	SessionID      string
	AssignSubject  string
	Degraded       bool
	Tags           []string
	Countries      []string
	Regions        []string
	IPTypes        []string
	IngressModes   []string
	ActiveRequests uint32
	MaxConcurrency uint32
	AvailableCap   uint32
}

// CandidatesForPool returns every worker session eligible for tenantID and
// scoped to poolID, applying the same admin/runtime exclusion precedence as
// EligibleForTenant (global disable, tenant disable, drain, cooldown,
// runtime health) plus pool scope. Capability matching and load ranking are
// left to the caller (routing, task 09).
func (r *WorkerRegistry) CandidatesForPool(tenantID, poolID string) []PoolCandidate {
	r.mu.Lock()
	defer r.mu.Unlock()

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
		if e.globalAdmin == AdminDisabled || e.tenantAdmin[tenantID] == AdminDisabled {
			continue
		}
		if e.globalDrain || e.tenantDrain[tenantID] {
			continue
		}
		state := r.runtimeState(e, now)
		if state != RuntimeReady && state != RuntimeDegraded {
			continue
		}
		subject, err := natsx.AssignmentSubject(workerID, s.sessionID)
		if err != nil {
			continue
		}
		out = append(out, PoolCandidate{
			WorkerID:       workerID,
			SessionID:      s.sessionID,
			AssignSubject:  subject,
			Degraded:       state == RuntimeDegraded,
			Tags:           s.tags,
			Countries:      s.countries,
			Regions:        s.regions,
			IPTypes:        s.ipTypes,
			IngressModes:   s.ingressModes,
			ActiveRequests: s.activeRequests,
			MaxConcurrency: s.maxConcurrency,
			AvailableCap:   s.availableCap,
		})
	}
	return out
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
	r.entry(workerID).globalAdmin = state
}

// SetGlobalDrain sets the durable platform drain flag for a worker.
func (r *WorkerRegistry) SetGlobalDrain(workerID string, draining bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entry(workerID).globalDrain = draining
}

// SetTenantAdmin sets the durable per-tenant admin override for a worker.
func (r *WorkerRegistry) SetTenantAdmin(workerID, tenantID string, state AdminState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entry(workerID).tenantAdmin[tenantID] = state
}

// SetTenantDrain sets the per-tenant drain flag for a worker.
func (r *WorkerRegistry) SetTenantDrain(workerID, tenantID string, draining bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entry(workerID).tenantDrain[tenantID] = draining
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
	TenantAdmin map[string]AdminState
	TenantDrain map[string]bool
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
			if subject, err := natsx.AssignmentSubject(workerID, e.current.sessionID); err == nil {
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
			WorkerID:         workerID,
			RuntimeState:     r.runtimeState(e, now),
			GlobalAdminState: e.globalAdmin,
			GlobalDraining:   e.globalDrain,
			TenantAdmin:      map[string]AdminState{tenantID: tenantAdminState(e, tenantID)},
			TenantDrain:      map[string]bool{tenantID: e.tenantDrain[tenantID]},
			ExecutorType:     e.current.executorType,
		}
		out = append(out, v)
	}
	return out
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
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyBoolMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func newSessionID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "sess_" + hex.EncodeToString(raw), nil
}
