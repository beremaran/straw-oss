package egress

import (
	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

// Capacity describes the executor's current admission state when an
// AssignRequest arrives.
type Capacity struct {
	Draining       bool
	ActiveRequests uint32
	MaxConcurrency uint32
	// SupportedModes lists the RequestModes this executor accepts. An empty
	// list means "any valid mode".
	SupportedModes []strawpb.RequestMode
}

func (c Capacity) hasCapacity() bool {
	if c.MaxConcurrency == 0 {
		return true
	}

	return c.ActiveRequests < c.MaxConcurrency
}

func (c Capacity) supportsMode(mode strawpb.RequestMode) bool {
	if len(c.SupportedModes) == 0 {
		return true
	}

	for _, m := range c.SupportedModes {
		if m == mode {
			return true
		}
	}

	return false
}

// EvaluateAssignment implements the executor-side admission decision for an
// AssignRequest (docs/planning/09 steps 13-15, docs/planning/12 "Assignment
// Flow"). It returns the AssignAckCode the executor should reply with. A
// non-ACCEPTED code means the executor never subscribes to the c2e stream and
// Control is free to fall back before RequestStart.
//
// Precedence: an invalid request is rejected first, then draining, then
// unsupported mode, then capacity. Capacity is reserved by the caller only
// when this returns ASSIGN_ACK_ACCEPTED.
func EvaluateAssignment(req *strawpb.AssignRequest, cap Capacity) strawpb.AssignAckCode {
	if req == nil || req.Validate() != nil {
		return strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_ERROR
	}

	if cap.Draining {
		return strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_DRAINING
	}

	if !cap.supportsMode(req.GetMode()) {
		return strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_UNSUPPORTED
	}

	if !cap.hasCapacity() {
		return strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_CAPACITY
	}

	return strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED
}

// FakeExecutor is a deterministic stand-in for real outbound execution, used
// to drive lifecycle tests without any outbound HTTP. It emits a scripted
// sequence of e2c StreamFrames for an accepted assignment; the real outbound
// path (task 11) replaces the frame source but reuses the same sequencing.
type FakeExecutor struct {
	attempt uint32
	seq     uint64
}

// NewFakeExecutor builds a fake executor emitting frames for the given attempt.
func NewFakeExecutor(attempt uint32) *FakeExecutor {
	return &FakeExecutor{attempt: attempt}
}

// next returns the next sequenced StreamFrame shell for the e2c direction.
func (f *FakeExecutor) next(payload isPayload) *strawpb.StreamFrame {
	f.seq++
	frame := &strawpb.StreamFrame{StreamSeq: f.seq, Attempt: f.attempt}
	payload.set(frame)

	return frame
}

// isPayload lets the fake set a oneof payload without repeating the switch.
type isPayload interface{ set(*strawpb.StreamFrame) }

type outboundStart struct {
	host string
	port uint32
}

func (p outboundStart) set(f *strawpb.StreamFrame) {
	f.Payload = &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{TargetHost: p.host, TargetPort: p.port}}
}

type responseStart struct{ status uint32 }

func (p responseStart) set(f *strawpb.StreamFrame) {
	f.Payload = &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: p.status}}
}

type dataFrame struct {
	offset uint64
	data   []byte
}

func (p dataFrame) set(f *strawpb.StreamFrame) {
	f.Payload = &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: p.offset, Data: p.data}}
}

type endFrame struct{ success bool }

func (p endFrame) set(f *strawpb.StreamFrame) {
	f.Payload = &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: p.success}}
}

type errorFrame struct {
	code strawpb.ErrorCode
	fact string
}

func (p errorFrame) set(f *strawpb.StreamFrame) {
	ef := &strawpb.ErrorFrame{Code: p.code}
	if p.fact != "" {
		ef.Details = map[string]string{"fact": p.fact}
	}

	f.Payload = &strawpb.StreamFrame_Error{Error: ef}
}

// SuccessResponse scripts a minimal successful response stream: OutboundStart,
// ResponseStart, one DataFrame carrying body, and a terminal EndFrame.
func (f *FakeExecutor) SuccessResponse(host string, port uint32, status uint32, body []byte) []*strawpb.StreamFrame {
	return []*strawpb.StreamFrame{
		f.next(outboundStart{host: host, port: port}),
		f.next(responseStart{status: status}),
		f.next(dataFrame{offset: 0, data: body}),
		f.next(endFrame{success: true}),
	}
}

// ErrorResponse scripts an outbound failure: OutboundStart followed by a
// terminal ErrorFrame carrying code and originating fact.
func (f *FakeExecutor) ErrorResponse(host string, port uint32, code strawpb.ErrorCode, fact string) []*strawpb.StreamFrame {
	return []*strawpb.StreamFrame{
		f.next(outboundStart{host: host, port: port}),
		f.next(errorFrame{code: code, fact: fact}),
	}
}
