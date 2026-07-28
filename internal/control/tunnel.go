package control

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

// executeTunnelAttemptOrFallback retries only assignment-time failures. Once
// Egress has opened the target and Control has written 200 Connection
// Established, the tunnel is opaque and its route cannot change.
func (d *DefaultRequestDispatcher) executeTunnelAttemptOrFallback(ctx context.Context, in DispatchInput, route RouteOutcome, snapshot config.Snapshot, deadline time.Time, rw io.ReadWriter) (dispatchResult, int64, *PipelineError, RouteOutcome) {
	execution, perr := d.resolveRouteExecution(in, snapshot, route, true)
	if perr != nil {
		return dispatchResult{}, 0, perr, route
	}

	result, assignmentMs, perr, established := d.executeTunnelAttemptUnmeasured(ctx, in, route, execution, snapshot.ConfigVersion, deadline, rw)
	d.opts.Metrics.ObserveAssignment(time.Duration(assignmentMs) * time.Millisecond)

	if perr == nil || established || !canFallbackBeforeRequestStart(perr.Code) {
		return result, assignmentMs, perr, route
	}

	routeFailure(d.opts.Workers, route.WorkerID)

	fallback := d.routeWithWorkers(in, snapshot, excludeWorkers{base: d.opts.Workers, workerID: route.WorkerID})
	if !fallback.OK {
		return result, assignmentMs, perr, route
	}

	fallbackExecution, fallbackPlanErr := d.resolveRouteExecution(in, snapshot, fallback, true)
	if fallbackPlanErr != nil {
		return result, assignmentMs, fallbackPlanErr, fallback
	}

	fallbackResult, fallbackAssignmentMs, fallbackErr, _ := d.executeTunnelAttemptUnmeasured(ctx, in, fallback, fallbackExecution, snapshot.ConfigVersion, deadline, rw)
	d.opts.Metrics.ObserveAssignment(time.Duration(fallbackAssignmentMs) * time.Millisecond)

	return fallbackResult, assignmentMs + fallbackAssignmentMs, fallbackErr, fallback
}

func (d *DefaultRequestDispatcher) executeTunnelAttemptUnmeasured(ctx context.Context, in DispatchInput, route RouteOutcome, execution routeExecution, configVersion uint64, deadline time.Time, rw io.ReadWriter) (dispatchResult, int64, *PipelineError, bool) {
	assignmentStarted := d.opts.Now()
	in.ProtocolMinor = route.ProtocolMinor

	if d.opts.NATS == nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}, false
	}

	c2eSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionControlToExecutor)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}, false
	}

	e2cSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionExecutorToControl)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}, false
	}

	frames := make(chan *strawpb.StreamFrame, defaultRequestFrameBuffer)

	sub, err := d.opts.NATS.Subscribe(e2cSubject, func(msg *nats.Msg) {
		frames <- decodeDispatchFrame(msg.Data, route.ProtocolMinor)
	})
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}, false
	}

	defer func() { _ = sub.Unsubscribe() }()

	err = d.opts.NATS.Flush()
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}, false
	}

	ack, perr := d.requestAssign(route.AssignSubject, in, d.assignRequest(in, route, configVersion, deadline), route.ProtocolMinor, deadline)
	if perr != nil {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), perr, false
	}

	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), assignRejectError(ack.GetCode()), false
	}

	assignmentMs := millisSince(assignmentStarted, d.opts.Now())

	nextSeq, err := d.sendRequestStart(ctx, c2eSubject, in, route, execution, configVersion, deadline)
	if err != nil {
		return dispatchResult{}, assignmentMs, requestStreamPipelineError(err), false
	}

	upload := d.startStreamingRequestBody(ctx, c2eSubject, in, deadline, nextSeq)
	result, perr, established := d.streamTunnel(ctx, frames, route, deadline, c2eSubject, in, nextSeq, upload, rw)

	return result, assignmentMs, perr, established
}

func (d *DefaultRequestDispatcher) streamTunnel(ctx context.Context, frames <-chan *strawpb.StreamFrame, route RouteOutcome, deadline time.Time, c2eSubject string, in DispatchInput, c2eSeq uint64, upload *requestBodyUpload, rw io.Writer) (dispatchResult, *PipelineError, bool) {
	state := tunnelStreamState{
		dispatcher: d,
		validator:  natsx.NewStreamValidator(defaultRequestAttempt, d.initialDownloadCredit(), d.opts.FrameIdleTimeout, d.opts.Now),
		route:      route,
		deadline:   deadline,
		c2eSubject: c2eSubject,
		in:         in,
		c2eSeq:     c2eSeq,
		upload:     upload,
		uploadErr:  uploadErrorChan(upload),
		rw:         rw,
	}
	if upload != nil {
		state.c2eSeq = upload.c2e.seq
	}
	defer closeUploadGate(upload)

	ticker := time.NewTicker(responseFrameCheckInterval)
	defer ticker.Stop()

	for {
		done, perr := state.next(ctx, ticker.C, frames)
		if done || perr != nil {
			return state.result, perr, state.established
		}
	}
}

type tunnelStreamState struct {
	dispatcher      *DefaultRequestDispatcher
	validator       *natsx.StreamValidator
	route           RouteOutcome
	deadline        time.Time
	c2eSubject      string
	in              DispatchInput
	c2eSeq          uint64
	upload          *requestBodyUpload
	uploadErr       <-chan error
	rw              io.Writer
	result          dispatchResult
	egressStart     time.Time
	established     bool
	outboundStarted bool
	responseStarted bool
}

func (s *tunnelStreamState) next(ctx context.Context, ticks <-chan time.Time, frames <-chan *strawpb.StreamFrame) (bool, *PipelineError) {
	select {
	case <-ctx.Done():
		s.cancel("client_cancelled")

		return true, &PipelineError{Code: Cancelled}
	case err := <-s.uploadErr:
		s.uploadErr = nil
		if perr := requestStreamPipelineErrorOrNil(err); perr != nil {
			s.cancel("client_upload_failed")

			return true, perr
		}

		return false, nil
	case <-time.After(time.Until(s.deadline)):
		s.cancel("deadline_exceeded")

		return true, &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeTotalDeadline}
	case <-ticks:
		if s.dispatcher.natsDisconnected() {
			return false, s.dispatcher.streamLost(s.route, TransportUnavailable)
		}

		if s.validator.IdleExpired() {
			s.cancel("idle_timeout")

			return false, &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeIdle}
		}

		return false, nil
	case frame, ok := <-frames:
		if !ok {
			return true, s.dispatcher.streamLost(s.route, WorkerDisconnected)
		}

		return s.accept(frame)
	}
}

func (s *tunnelStreamState) accept(frame *strawpb.StreamFrame) (bool, *PipelineError) {
	ok, done, perr := acceptedResponseFrame(s.validator.Accept(frame))
	if !ok || done {
		return done, perr
	}

	if perr := s.dispatcher.validateRouteResponseFrame(s.route, frame, &s.outboundStarted, &s.responseStarted); perr != nil {
		return true, perr
	}

	if credit := frame.GetCredit(); credit != nil {
		s.acceptCredit(credit)

		return false, nil
	}

	return s.acceptPayload(frame)
}

func (s *tunnelStreamState) acceptCredit(credit *strawpb.CreditFrame) {
	if s.upload != nil {
		s.upload.gate.grant(credit.GetUploadCreditBytes())
	}
}

func (s *tunnelStreamState) acceptPayload(frame *strawpb.StreamFrame) (bool, *PipelineError) {
	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_OutboundStart:
		s.egressStart = s.dispatcher.opts.Now()
	case *strawpb.StreamFrame_ResponseStart:
		return false, s.acceptResponseStart(p.ResponseStart)
	case *strawpb.StreamFrame_Data:
		return false, s.acceptData(p.Data)
	case *strawpb.StreamFrame_Error:
		return true, s.dispatcher.executorError(s.route, p.Error)
	case *strawpb.StreamFrame_Cancelled:
		return true, &PipelineError{Code: Cancelled}
	case *strawpb.StreamFrame_End:
		return s.acceptEnd()
	default:
		routeFailure(s.dispatcher.opts.Workers, s.route.WorkerID)

		return true, &PipelineError{Code: ProtocolError}
	}

	return false, nil
}

func (s *tunnelStreamState) acceptResponseStart(start *strawpb.ResponseStart) *PipelineError {
	if s.established || start.GetStatus() != http.StatusOK {
		return &PipelineError{Code: ProtocolError}
	}

	_, err := io.WriteString(s.rw, "HTTP/1.1 200 Connection Established\r\n\r\n")
	if err != nil {
		return &PipelineError{Code: Cancelled}
	}

	s.established = true
	s.result.status = http.StatusOK

	return nil
}

func (s *tunnelStreamState) acceptData(data *strawpb.DataFrame) *PipelineError {
	if !s.established {
		return &PipelineError{Code: ProtocolError}
	}

	n, err := s.rw.Write(data.GetData())
	if err != nil || n != len(data.GetData()) {
		s.cancel("client_disconnect")

		return &PipelineError{Code: Cancelled}
	}

	written := uint64FromInt(n)
	s.result.size += written

	if s.upload != nil {
		s.upload.c2e.credit(written)
	} else {
		s.c2eSeq = s.dispatcher.sendDownloadCredit(s.c2eSubject, s.in, s.deadline, s.c2eSeq, written)
	}

	s.validator.GrantCredit(written)

	return nil
}

func (s *tunnelStreamState) acceptEnd() (bool, *PipelineError) {
	if !s.established {
		return true, &PipelineError{Code: ProtocolError}
	}

	s.result.egressMs = egressMillis(s.egressStart, s.dispatcher.opts.Now())

	return true, nil
}

func (s *tunnelStreamState) cancel(reason string) {
	if s.upload != nil {
		s.upload.c2e.cancel(reason)

		return
	}

	s.dispatcher.sendCancel(s.c2eSubject, s.in, s.deadline, s.c2eSeq, reason)
}
