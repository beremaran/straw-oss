package natsx

import (
	"time"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

// FrameOutcome is the result of validating one StreamFrame against a
// per-direction, per-attempt stream (docs/public/architecture.md "Stream Ordering and
// Sequencing"). Ignored/counted outcomes contribute to worker cooldown; the
// caller is responsible for that accounting.
type FrameOutcome int

const (
	// FrameAccepted means the frame was the next expected frame and was
	// applied to stream state.
	FrameAccepted FrameOutcome = iota
	// FrameDuplicate means stream_seq was lower than expected: ignored and
	// counted.
	FrameDuplicate
	// FrameAfterTerminal means a frame arrived after a terminal frame:
	// ignored and counted.
	FrameAfterTerminal
	// FrameSequenceGap means stream_seq was higher than expected: a protocol
	// error.
	FrameSequenceGap
	// FrameOffsetMismatch means a DataFrame offset did not equal the
	// cumulative bytes previously accepted: a protocol error.
	FrameOffsetMismatch
	// FrameCreditExhausted means a DataFrame carried more bytes than the
	// remaining granted credit: a protocol error.
	FrameCreditExhausted
	// FrameAttemptMismatch means the frame attempt did not equal the active
	// attempt: a protocol error.
	FrameAttemptMismatch
	// FrameInvalid means the frame was nil or carried no payload.
	FrameInvalid
)

// IsProtocolError reports whether the outcome is a hard protocol violation
// (as opposed to an ignored/counted duplicate or late frame).
func (o FrameOutcome) IsProtocolError() bool {
	return o == FrameSequenceGap || o == FrameOffsetMismatch || o == FrameCreditExhausted || o == FrameAttemptMismatch || o == FrameInvalid
}

// StreamValidator enforces the ordering, offset, credit, and terminal rules
// for a single stream direction and attempt (docs/public/architecture.md). A validator
// models the receiving side of one byte stream: it holds the data-byte credit
// it has granted to the sender and rejects DataFrames that exceed it.
//
// stream_seq starts at 1 for each (attempt, direction). Credit applies only to
// DataFrame payload bytes; control and terminal frames never consume credit.
type StreamValidator struct {
	attempt      uint32
	expectedSeq  uint64
	offset       uint64
	credit       uint64
	terminal     bool
	idleTimeout  time.Duration
	lastActivity time.Time
	now          func() time.Time
	allowBodyRef bool
}

// StreamValidatorOptions configures a stream validator.
type StreamValidatorOptions struct {
	Attempt       uint32
	InitialCredit uint64
	IdleTimeout   time.Duration
	Now           func() time.Time
	AllowBodyRef  bool
}

// NewStreamValidator builds a validator for the given active attempt.
// initialCredit is the data-byte credit initially granted to the sender
// (AssignRequest initial upload/download credit). idleTimeout of zero disables
// the idle check. now may be nil (defaults to time.Now).
func NewStreamValidator(attempt uint32, initialCredit uint64, idleTimeout time.Duration, now func() time.Time) *StreamValidator {
	return NewStreamValidatorWithOptions(StreamValidatorOptions{
		Attempt:       attempt,
		InitialCredit: initialCredit,
		IdleTimeout:   idleTimeout,
		Now:           now,
	})
}

// NewStreamValidatorWithOptions builds a validator. AllowBodyRef is only set
// on P2 receive paths that are explicitly prepared to dereference BodyRef.
func NewStreamValidatorWithOptions(opts StreamValidatorOptions) *StreamValidator {
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return &StreamValidator{
		attempt:      opts.Attempt,
		expectedSeq:  1,
		credit:       opts.InitialCredit,
		idleTimeout:  opts.IdleTimeout,
		lastActivity: now(),
		now:          now,
		allowBodyRef: opts.AllowBodyRef,
	}
}

// GrantCredit adds data-byte credit for the sender (the receiver calls this
// when it emits a CreditFrame on the reverse channel after releasing buffered
// bytes).
func (v *StreamValidator) GrantCredit(bytes uint64) {
	v.credit += bytes
}

// Terminated reports whether a terminal frame has been accepted.
func (v *StreamValidator) Terminated() bool { return v.terminal }

// ExpectedSeq returns the next stream_seq the validator will accept.
func (v *StreamValidator) ExpectedSeq() uint64 { return v.expectedSeq }

// RemainingCredit returns the data-byte credit still available to the sender.
func (v *StreamValidator) RemainingCredit() uint64 { return v.credit }

// Accept validates f and, on FrameAccepted, advances stream state. All other
// outcomes leave state unchanged.
func (v *StreamValidator) Accept(f *strawpb.StreamFrame) FrameOutcome {
	if outcome := v.validateFrameShell(f); outcome != FrameAccepted {
		return outcome
	}

	seq := f.GetStreamSeq()
	switch {
	case seq < v.expectedSeq:
		return FrameDuplicate
	case seq > v.expectedSeq:
		return FrameSequenceGap
	}

	// stream_seq is exactly the next expected value. Apply payload-specific
	// validation before committing.
	if outcome := v.validatePayload(f); outcome != FrameAccepted {
		return outcome
	}

	if outcome := v.acceptData(f.GetData()); outcome != FrameAccepted {
		return outcome
	}

	v.expectedSeq++

	v.lastActivity = v.now()
	if isTerminalFrame(f) {
		v.terminal = true
	}

	return FrameAccepted
}

// IdleExpired reports whether the frame idle timeout has elapsed since the
// last accepted frame. It is never true once the stream is terminated.
func (v *StreamValidator) IdleExpired() bool {
	if v.idleTimeout <= 0 || v.terminal {
		return false
	}

	return v.now().Sub(v.lastActivity) >= v.idleTimeout
}

func (v *StreamValidator) validateFrameShell(f *strawpb.StreamFrame) FrameOutcome {
	if f == nil || f.GetPayload() == nil {
		return FrameInvalid
	}

	if v.terminal {
		return FrameAfterTerminal
	}

	if f.GetStreamSeq() == 0 {
		return FrameInvalid
	}

	if f.GetAttempt() != v.attempt {
		return FrameAttemptMismatch
	}

	return FrameAccepted
}

func (v *StreamValidator) validatePayload(f *strawpb.StreamFrame) FrameOutcome {
	if _, ok := f.GetPayload().(*strawpb.StreamFrame_BodyRef); ok && !v.allowBodyRef {
		return FrameInvalid
	}

	return FrameAccepted
}

func (v *StreamValidator) acceptData(data *strawpb.DataFrame) FrameOutcome {
	if data == nil {
		return FrameAccepted
	}

	if data.GetOffset() != v.offset {
		return FrameOffsetMismatch
	}

	n := uint64(len(data.GetData()))
	if n > v.credit {
		return FrameCreditExhausted
	}

	v.credit -= n
	v.offset += n

	return FrameAccepted
}

// isTerminalFrame reports whether f carries a terminal payload (EndFrame,
// ErrorFrame, or CancelledFrame) per docs/public/architecture.md "Terminal Rule".
func isTerminalFrame(f *strawpb.StreamFrame) bool {
	switch f.GetPayload().(type) {
	case *strawpb.StreamFrame_End, *strawpb.StreamFrame_Error, *strawpb.StreamFrame_Cancelled:
		return true
	default:
		return false
	}
}
