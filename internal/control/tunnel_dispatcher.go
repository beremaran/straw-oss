package control

import (
	"bufio"
	"context"
	"io"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const connectEstablishedStatus = 200

// DispatchTunnel runs admission, routing, assignment, and bidirectional
// DataFrame streaming for one raw CONNECT tunnel.
func (d *DefaultRequestDispatcher) DispatchTunnel(ctx context.Context, in DispatchInput, rw io.ReadWriter) (SuccessResponse, *PipelineError) {
	started := d.opts.Now()

	d.opts.Metrics.IncActiveRequests()
	defer d.opts.Metrics.DecActiveRequests()

	resp, perr := d.dispatchTunnel(ctx, in, rw, started)

	var code ErrorCode
	if perr != nil {
		code = perr.Code
	}

	d.opts.Metrics.ObserveRequest(in.Identity.TenantID, errorCodeLabel(code), d.opts.Now().Sub(started))

	return resp, perr
}

func (d *DefaultRequestDispatcher) dispatchTunnel(ctx context.Context, in DispatchInput, rw io.ReadWriter, started time.Time) (SuccessResponse, *PipelineError) {
	if in.Request == nil || d.opts.ConfigCache == nil || d.opts.Workers == nil || rw == nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	d.opts.InFlight.Register(ctx, in.RequestID, in.Identity.TenantID, cancel)
	defer d.opts.InFlight.Deregister(ctx, in.RequestID)

	prep, perr := d.prepareTunnelDispatch(ctx, in, started)
	if perr != nil {
		return SuccessResponse{}, perr
	}

	deadline := d.deadline(in.Request, prep.snapshot)

	result, assignmentMs, perr := d.executeTunnelAttempt(ctx, in, prep.route, prep.policy, prep.snapshot.ConfigVersion, deadline, rw)
	if perr != nil {
		perr = d.withTiming(perr, prep.routingMs, assignmentMs, started)
		perr.EgressMs = result.egressMs

		return rawSuccessFromDispatch(in.RequestID, result, prep.routingMs, assignmentMs, millisSince(started, d.opts.Now())), perr
	}

	if d.opts.QuotaAdmission != nil {
		_ = d.opts.QuotaAdmission.RecordSuccess(ctx, quotaFromSnapshot(in.Identity.TenantID, prep.snapshot.Quota))
		_ = d.opts.QuotaAdmission.AddBandwidth(ctx, in.Identity.TenantID, int64FromUint64(result.size))
	}

	return rawSuccessFromDispatch(in.RequestID, result, prep.routingMs, assignmentMs, millisSince(started, d.opts.Now())), nil
}

type tunnelDispatchPreparation struct {
	snapshot  config.TenantSnapshot
	route     RouteOutcome
	policy    *DestinationPolicyResult
	routingMs int64
}

func (d *DefaultRequestDispatcher) prepareTunnelDispatch(ctx context.Context, in DispatchInput, started time.Time) (tunnelDispatchPreparation, *PipelineError) {
	snapshot, err := d.opts.ConfigCache.Snapshot(ctx, in.Identity.TenantID)
	if err != nil {
		return tunnelDispatchPreparation{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started)
	}

	perr := d.admit(ctx, in, snapshot)
	if perr != nil {
		return tunnelDispatchPreparation{}, d.withTiming(perr, 0, 0, started)
	}

	routeStart := d.opts.Now()
	route := d.route(in, snapshot)
	routeEnd := d.opts.Now()
	routingMs := millisSince(routeStart, routeEnd)
	d.opts.Metrics.ObserveRouting(routeEnd.Sub(routeStart))

	if !route.OK {
		return tunnelDispatchPreparation{}, d.withTiming(routeError(route.ErrorCode), routingMs, 0, started)
	}

	policy, verr := ResolveDestinationPolicy(DestinationPolicyRequest{
		Snapshot:               snapshot,
		TargetURL:              in.Request.URL,
		MaxInjectedHeaderBytes: d.opts.MaxFrameDataBytes,
		UpstreamProxyEnabled:   false,
		UpstreamProxyTrusted:   false,
	})
	if verr != nil {
		return tunnelDispatchPreparation{}, d.withTiming(validationPipelineError(verr), routingMs, 0, started)
	}

	return tunnelDispatchPreparation{snapshot: snapshot, route: route, policy: policy, routingMs: routingMs}, nil
}

func (d *DefaultRequestDispatcher) executeTunnelAttempt(ctx context.Context, in DispatchInput, route RouteOutcome, policy *DestinationPolicyResult, configVersion uint64, deadline time.Time, rw io.ReadWriter) (dispatchResult, int64, *PipelineError) {
	assignmentStarted := d.opts.Now()

	if d.opts.NATS == nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}
	}

	c2eSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionControlToExecutor)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}
	}

	e2cSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionExecutorToControl)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}
	}

	frames := make(chan *strawpb.StreamFrame, defaultRequestFrameBuffer)

	sub, err := d.opts.NATS.Subscribe(e2cSubject, func(msg *nats.Msg) {
		frames <- decodeDispatchFrame(msg.Data)
	})
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}
	}

	defer func() { _ = sub.Unsubscribe() }()

	err = d.opts.NATS.Flush()
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}
	}

	assign := d.assignRequest(in, route, configVersion, deadline)

	ack, perr := d.requestAssign(route.AssignSubject, in, assign, deadline)
	if perr != nil {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), perr
	}

	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), assignRejectError(ack.GetCode())
	}

	assignmentMs := millisSince(assignmentStarted, d.opts.Now())

	nextSeq, err := d.sendRequestStart(ctx, c2eSubject, in, route, policy, configVersion, deadline)
	if err != nil {
		return dispatchResult{}, assignmentMs, requestStreamPipelineError(err)
	}

	result, perr := d.streamTunnel(ctx, frames, route, deadline, c2eSubject, in, nextSeq, rw)

	return result, assignmentMs, perr
}

func (d *DefaultRequestDispatcher) streamTunnel(ctx context.Context, frames <-chan *strawpb.StreamFrame, route RouteOutcome, deadline time.Time, c2eSubject string, in DispatchInput, c2eSeq uint64, rw io.ReadWriter) (dispatchResult, *PipelineError) {
	validator := natsx.NewStreamValidator(defaultRequestAttempt, d.opts.InitialDownloadCreditBytes, d.opts.FrameIdleTimeout, d.opts.Now)

	state := tunnelStreamState{
		dispatcher: d,
		route:      route,
		rw:         rw,
		result:     dispatchResult{status: connectEstablishedStatus},
		upload:     newTunnelUploadGate(d.opts.InitialUploadCreditBytes),
		c2e:        &c2eStreamSender{dispatcher: d, subject: c2eSubject, in: in, deadline: deadline, seq: c2eSeq},
	}
	defer state.upload.close()

	ticker := time.NewTicker(responseFrameCheckInterval)
	defer ticker.Stop()

	for {
		done, perr := state.nextTunnelEvent(ctx, deadline, ticker.C, frames, validator)
		if done || perr != nil {
			return state.result, perr
		}
	}
}

type tunnelStreamState struct {
	dispatcher  *DefaultRequestDispatcher
	route       RouteOutcome
	rw          io.ReadWriter
	result      dispatchResult
	egressStart time.Time
	established bool
	clientErr   chan error
	upload      *tunnelUploadGate
	c2e         *c2eStreamSender
}

func (s *tunnelStreamState) nextTunnelEvent(ctx context.Context, deadline time.Time, ticks <-chan time.Time, frames <-chan *strawpb.StreamFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	select {
	case <-ctx.Done():
		s.c2e.cancel("client_cancelled")

		return true, &PipelineError{Code: Cancelled}
	case <-time.After(time.Until(deadline)):
		s.c2e.cancel("deadline_exceeded")

		return true, &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeTotalDeadline}
	case <-ticks:
		if s.dispatcher.natsDisconnected() {
			return true, s.dispatcher.streamLost(s.route, TransportUnavailable)
		}

		return s.idleEvent(validator)
	case err := <-s.clientErr:
		return s.clientEvent(err)
	case frame, ok := <-frames:
		if !ok {
			return true, s.dispatcher.streamLost(s.route, WorkerDisconnected)
		}

		return s.acceptValidated(frame, validator)
	}
}

func (s *tunnelStreamState) acceptValidated(frame *strawpb.StreamFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	ok, done, perr := acceptedResponseFrame(validator.Accept(frame))
	if !ok || done {
		return true, perr
	}

	return s.accept(frame, validator)
}

func (s *tunnelStreamState) accept(frame *strawpb.StreamFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_OutboundStart:
		s.egressStart = s.dispatcher.opts.Now()
	case *strawpb.StreamFrame_ResponseStart:
		return s.acceptResponseStart(p.ResponseStart)
	case *strawpb.StreamFrame_Data:
		return s.acceptData(p.Data, validator)
	case *strawpb.StreamFrame_Credit:
		s.upload.grant(p.Credit.GetUploadCreditBytes())
	case *strawpb.StreamFrame_Error:
		return true, s.dispatcher.executorError(s.route, p.Error)
	case *strawpb.StreamFrame_Cancelled:
		return true, &PipelineError{Code: Cancelled}
	case *strawpb.StreamFrame_End:
		s.result.egressMs = egressMillis(s.egressStart, s.dispatcher.opts.Now())

		return true, nil
	default:
		routeFailure(s.dispatcher.opts.Workers, s.route.WorkerID)

		return true, &PipelineError{Code: ProtocolError}
	}

	return false, nil
}

func (s *tunnelStreamState) acceptResponseStart(resp *strawpb.ResponseStart) (bool, *PipelineError) {
	if resp.GetStatus() != connectEstablishedStatus {
		return true, &PipelineError{Code: ProtocolError}
	}

	brw, ok := asBufioReadWriter(s.rw)
	if !ok {
		return true, &PipelineError{Code: ProtocolError}
	}

	err := writeConnectEstablished(brw)
	if err != nil {
		return true, &PipelineError{Code: Cancelled}
	}

	s.established = true

	s.clientErr = make(chan error, 1)

	go s.readClient()

	return false, nil
}

func (s *tunnelStreamState) acceptData(data *strawpb.DataFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	if !s.established {
		return true, &PipelineError{Code: ProtocolError}
	}

	n, err := s.rw.Write(data.GetData())
	if err != nil {
		return true, &PipelineError{Code: Cancelled}
	}

	brw, ok := asBufioReadWriter(s.rw)
	if ok {
		err = brw.Flush()
		if err != nil {
			return true, &PipelineError{Code: Cancelled}
		}
	}

	written := uint64FromInt(n)
	s.result.size += written
	s.c2e.credit(written)
	validator.GrantCredit(written)

	return false, nil
}

func (s *tunnelStreamState) idleEvent(validator *natsx.StreamValidator) (bool, *PipelineError) {
	if !validator.IdleExpired() {
		return false, nil
	}

	s.c2e.cancel("idle_timeout")

	return true, &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeIdle}
}

func (s *tunnelStreamState) clientEvent(err error) (bool, *PipelineError) {
	if err == nil {
		return false, nil
	}

	s.c2e.cancel("client_disconnect")

	return true, &PipelineError{Code: Cancelled}
}

func (s *tunnelStreamState) readClient() {
	buf := make([]byte, s.dispatcher.opts.MaxFrameDataBytes)
	offset := uint64(0)

	for {
		n, err := s.rw.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if !s.upload.take(uint64FromInt(n)) {
				s.clientErr <- context.Canceled

				return
			}

			s.c2e.data(offset, chunk)
			offset += uint64FromInt(n)
			s.result.size += uint64FromInt(n)
		}

		if err != nil {
			s.clientErr <- err

			return
		}
	}
}

type tunnelUploadGate struct {
	mu     sync.Mutex
	credit uint64
	wake   chan struct{}
	closed bool
}

func newTunnelUploadGate(initial uint64) *tunnelUploadGate {
	return &tunnelUploadGate{credit: initial, wake: make(chan struct{}, 1)}
}

func (g *tunnelUploadGate) grant(bytes uint64) {
	if bytes == 0 {
		return
	}

	g.mu.Lock()
	g.credit += bytes
	g.mu.Unlock()

	select {
	case g.wake <- struct{}{}:
	default:
	}
}

func (g *tunnelUploadGate) take(bytes uint64) bool {
	for {
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()

			return false
		}

		if g.credit >= bytes {
			g.credit -= bytes
			g.mu.Unlock()

			return true
		}
		g.mu.Unlock()

		<-g.wake
	}
}

func (g *tunnelUploadGate) takeMax(maxBytes uint64) (uint64, bool) {
	for {
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()

			return 0, false
		}

		if g.credit > 0 {
			bytes := min(g.credit, maxBytes)
			g.credit -= bytes
			g.mu.Unlock()

			return bytes, true
		}
		g.mu.Unlock()

		<-g.wake
	}
}

func (g *tunnelUploadGate) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()

	select {
	case g.wake <- struct{}{}:
	default:
	}
}

type c2eStreamSender struct {
	mu         sync.Mutex
	dispatcher *DefaultRequestDispatcher
	subject    string
	in         DispatchInput
	deadline   time.Time
	seq        uint64
	publishFn  func(*strawpb.StreamFrame)
}

func (s *c2eStreamSender) data(offset uint64, data []byte) {
	s.publish(func(seq uint64) *strawpb.StreamFrame {
		return &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: offset, Data: data}},
		}
	})
}

func (s *c2eStreamSender) credit(bytes uint64) {
	if bytes == 0 {
		return
	}

	s.publish(func(seq uint64) *strawpb.StreamFrame {
		return &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Credit{Credit: &strawpb.CreditFrame{DownloadCreditBytes: bytes}},
		}
	})
}

func (s *c2eStreamSender) cancel(reason string) {
	s.publish(func(seq uint64) *strawpb.StreamFrame {
		return &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Cancel{Cancel: &strawpb.CancelFrame{Reason: reason}},
		}
	})
}

func (s *c2eStreamSender) publish(build func(uint64) *strawpb.StreamFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()

	frame := build(s.seq)
	if s.publishFn != nil {
		s.publishFn(frame)
	} else {
		_ = s.dispatcher.publishFrame(s.subject, s.in, s.deadline, frame)
	}

	s.seq++
}

func asBufioReadWriter(rw io.ReadWriter) (*bufio.ReadWriter, bool) {
	brw, ok := rw.(*bufio.ReadWriter)

	return brw, ok
}
