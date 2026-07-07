package control

import (
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

// LifecycleState is the Control-side state of one assignment attempt
// (docs/planning/09-canonical-request-lifecycle.md).
type LifecycleState int

const (
	// LifecycleAssigning is after AssignRequest is sent, before AssignAck.
	LifecycleAssigning LifecycleState = iota
	// LifecycleAccepted is after AssignAck ACCEPTED, before RequestStart.
	LifecycleAccepted
	// LifecycleStarted is after RequestStart is sent (the P0 no-replay
	// boundary).
	LifecycleStarted
	// LifecycleTerminated is after a terminal frame or synthesized outcome.
	LifecycleTerminated
)

// TerminalKind classifies how an assignment ended.
type TerminalKind int

const (
	// TerminalNone means no terminal frame has been recorded yet.
	TerminalNone TerminalKind = iota
	// TerminalEnd is a clean EndFrame.
	TerminalEnd
	// TerminalError is an ErrorFrame (or synthesized error outcome).
	TerminalError
	// TerminalCancelled is a CancelledFrame or Control-driven cancellation.
	TerminalCancelled
)

// executorEmittableCodes is the set of ErrorCodes an executor is permitted to
// send in an ErrorFrame (docs/planning/13 "Executor Error Reporting"). Any
// other code is a protocol violation mapped to executor_internal_error and
// counted toward worker cooldown.
var executorEmittableCodes = map[strawpb.ErrorCode]struct{}{
	strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED:          {},
	strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED:            {},
	strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT:     {},
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE:        {},
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE:        {},
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED: {},
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECT_TIMEOUT:    {},
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET:              {},
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE:      {},
	strawpb.ErrorCode_ERROR_CODE_STREAM_UPLOAD_ABORTED:       {},
	strawpb.ErrorCode_ERROR_CODE_STREAM_DOWNLOAD_ABORTED:     {},
	strawpb.ErrorCode_ERROR_CODE_BODY_REF_UNAVAILABLE:        {},
	strawpb.ErrorCode_ERROR_CODE_BODY_TOO_LARGE:              {},
	strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR:     {},
}

// ValidateExecutorError maps an ErrorFrame code to the outcome code Control
// records. In-set codes pass through unchanged; any out-of-set code (including
// UNSPECIFIED) maps to executor_internal_error and is flagged as a protocol
// violation the caller must count toward worker cooldown
// (docs/planning/13, docs/planning/30 "Error mapping" row).
func ValidateExecutorError(code strawpb.ErrorCode) (strawpb.ErrorCode, bool) {
	if _, ok := executorEmittableCodes[code]; ok {
		return code, false
	}

	return strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, true
}

// Assignment tracks the Control-side lifecycle of a single assignment attempt:
// ack handling, ack timeout, the RequestStart no-replay boundary, terminal
// frame handling, and cancellation. It is a pure state machine; NATS I/O is
// the caller's responsibility. Not safe for concurrent use.
type Assignment struct {
	RequestID  string
	TenantID   string
	WorkerID   string
	SessionID  string
	Attempt    uint32
	Replayable bool

	state    LifecycleState
	terminal TerminalKind
	// clientResponded records that Control has begun emitting a client-visible
	// success envelope, which forbids fallback even for replayable requests
	// (docs/planning/09 "Replay and Fallback Boundary").
	clientResponded bool
	// fallbackAllowed preserves replay eligibility after the attempt has moved
	// to a terminal state.
	fallbackAllowed bool
}

// NewAssignment builds an assignment in the Assigning state, immediately after
// AssignRequest has been published to the exact-session assignment subject.
func NewAssignment(requestID, tenantID, workerID, sessionID string, attempt uint32, replayable bool) *Assignment {
	return &Assignment{
		RequestID:  requestID,
		TenantID:   tenantID,
		WorkerID:   workerID,
		SessionID:  sessionID,
		Attempt:    attempt,
		Replayable: replayable,
		state:      LifecycleAssigning,
		terminal:   TerminalNone,
	}
}

// State returns the current lifecycle state.
func (a *Assignment) State() LifecycleState { return a.state }

// Terminal returns how the assignment ended, or TerminalNone if still active.
func (a *Assignment) Terminal() TerminalKind { return a.terminal }

// OnAssignAck applies an AssignAck. An ACCEPTED ack moves an assigning
// assignment to Accepted. Any rejection terminates the attempt; fallback is
// permitted because rejection happens before RequestStart. The returned
// accepted flag is true only for a fresh ACCEPTED transition.
func (a *Assignment) OnAssignAck(code strawpb.AssignAckCode) bool {
	if a.state != LifecycleAssigning {
		return false
	}

	if code == strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		a.state = LifecycleAccepted

		return true
	}
	// Any reject code: terminate this attempt. Fallback is allowed (before
	// RequestStart) but that is the router's decision via CanFallback.
	a.state = LifecycleTerminated
	a.terminal = TerminalError
	a.fallbackAllowed = true

	return false
}

// OnAckTimeout terminates an assignment that never received an AssignAck. It is
// a no-op once an ack has already advanced the state. Returns true if the
// timeout took effect.
func (a *Assignment) OnAckTimeout() bool {
	if a.state != LifecycleAssigning {
		return false
	}

	a.state = LifecycleTerminated
	a.terminal = TerminalError
	a.fallbackAllowed = true

	return true
}

// MarkRequestStart records that Control has sent RequestStart over c2e. It is
// only valid from the Accepted state (after AssignAck ACCEPTED). This crosses
// the P0 no-replay boundary.
func (a *Assignment) MarkRequestStart() bool {
	if a.state != LifecycleAccepted {
		return false
	}

	a.state = LifecycleStarted

	return true
}

// MarkClientResponded records that a client-visible success envelope has begun.
// After this, fallback is forbidden even for replayable requests.
func (a *Assignment) MarkClientResponded() {
	a.clientResponded = true
}

// CanFallback reports whether Control may abandon this attempt and try another
// executor without violating the P0 replay boundary (docs/planning/09):
// fallback is always allowed before RequestStart; after RequestStart it is
// allowed only for replayable requests that have not yet emitted a client
// response.
func (a *Assignment) CanFallback() bool {
	if a.fallbackAllowed && !a.clientResponded {
		return true
	}

	if a.state == LifecycleAssigning || a.state == LifecycleAccepted {
		return true
	}

	if a.state == LifecycleStarted {
		return a.Replayable && !a.clientResponded
	}

	return false
}

// RecordTerminal applies a terminal frame. The first terminal on an active
// assignment is accepted and terminates it. Any later terminal (or any frame
// once terminated) is a duplicate/late frame: ignored and counted toward
// worker cooldown by the caller (docs/planning/09 "Terminal Rule"). Returns
// accepted=true only for the first terminal.
func (a *Assignment) RecordTerminal(kind TerminalKind) bool {
	if a.state == LifecycleTerminated {
		return false
	}

	a.state = LifecycleTerminated
	a.terminal = kind

	return true
}

// SynthesizeTerminal terminates the assignment with a synthesized outcome when
// no terminal frame can arrive (worker death, transport loss, deadline). It is
// a no-op if already terminated. Returns true if it took effect.
func (a *Assignment) SynthesizeTerminal(code strawpb.ErrorCode) bool {
	if a.state == LifecycleTerminated {
		return false
	}

	a.fallbackAllowed = code != strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED && a.CanFallback()
	a.state = LifecycleTerminated
	a.terminal = TerminalError

	return true
}

// Cancel drives a Control-initiated cancellation (client disconnect, deadline,
// admin cancel, shutdown). It sets the terminal state to Cancelled and reports
// whether a CancelFrame should be sent to the executor: a CancelFrame is only
// meaningful once the request-scoped c2e path exists, i.e. from RequestStart
// onward. Cancelling an already-terminated assignment is a no-op.
func (a *Assignment) Cancel() (bool, bool) {
	if a.state == LifecycleTerminated {
		return false, false
	}

	send := a.state == LifecycleStarted
	a.state = LifecycleTerminated
	a.terminal = TerminalCancelled

	return send, true
}

// AuthorizeAdminCancel enforces the admin-cancel authorization rule
// (docs/planning task 10, docs/planning/30 "Worker admin" row): a system_admin
// (platform scope) may cancel any request; a tenant-scoped caller may cancel
// only a request that belongs to its own tenant. A mismatch returns
// ErrInsufficientPermissions without confirming whether the request exists.
func AuthorizeAdminCancel(identity Identity, requestTenantID string) error {
	if identity.IsPlatform() {
		if identity.Role != RoleSystemAdmin {
			return ErrInsufficientPermissions
		}

		return nil
	}

	if identity.TenantID == "" || identity.TenantID != requestTenantID {
		return ErrInsufficientPermissions
	}

	return nil
}

// ackDeadline is a small helper for callers enforcing the assignment ack
// timeout bounded by the remaining total deadline (docs/planning/09 "Timeout
// Hierarchy"). It returns the earlier of now+ackTimeout and the total deadline.
func ackDeadline(now time.Time, ackTimeoutDuration time.Duration, totalDeadline time.Time) time.Time {
	deadline, _ := ackTimeout(now, ackTimeoutDuration, totalDeadline)

	return deadline
}

func ackTimeout(now time.Time, ackTimeoutDuration time.Duration, totalDeadline time.Time) (time.Time, strawpb.TimeoutType) {
	d := now.Add(ackTimeoutDuration)
	if !totalDeadline.IsZero() && !d.Before(totalDeadline) {
		return totalDeadline, strawpb.TimeoutType_TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT
	}

	return d, strawpb.TimeoutType_TIMEOUT_TYPE_ASSIGNMENT_TIMEOUT
}
