package control

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/beremaran/straw-oss/internal/natsx"
	"github.com/beremaran/straw-oss/internal/receipt"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func (d *DefaultRequestDispatcher) readResponse(ctx context.Context, frames <-chan *strawpb.StreamFrame, route RouteOutcome, deadline time.Time, c2eSubject string, in DispatchInput, c2eSeq uint64, upload *requestBodyUpload) (dispatchResult, *PipelineError) {
	var responseUpload *receipt.ResponseUpload

	if in.Request.ResponseBodyMode == bodyModeReceipt {
		if d.opts.Receipts == nil {
			return dispatchResult{}, &PipelineError{Code: BodyRefUnavailable, Message: "response receipts are not enabled"}
		}

		var err error

		responseUpload, err = d.opts.Receipts.BeginResponse(ctx, in.Identity.DeploymentID)
		if err != nil {
			return dispatchResult{}, &PipelineError{Code: ControlInternalError}
		}
	}

	state := decodedResponseStreamState{
		dispatcher:    d,
		validator:     natsx.NewStreamValidator(defaultRequestAttempt, d.initialDownloadCredit(), d.opts.FrameIdleTimeout, d.opts.Now),
		route:         route,
		deadline:      deadline,
		c2eSubject:    c2eSubject,
		in:            in,
		c2eSeq:        c2eSeq,
		upload:        upload,
		uploadErr:     uploadErrorChan(upload),
		result:        dispatchResult{status: http.StatusOK, selectedFingerprintProfile: selectedFingerprintForRequest(in.Request), responseUpload: responseUpload},
		egressStarted: time.Time{},
	}
	defer closeUploadGate(upload)

	ticker := time.NewTicker(responseFrameCheckInterval)
	defer ticker.Stop()

	for {
		done, perr := state.next(ctx, ticker.C, frames)
		if perr != nil {
			if responseUpload != nil {
				responseUpload.Abort(context.WithoutCancel(ctx))
			}

			return state.result, perr
		}

		if !done {
			continue
		}

		return d.commitResponseReceipt(ctx, state.result, responseUpload)
	}
}

func (d *DefaultRequestDispatcher) commitResponseReceipt(ctx context.Context, result dispatchResult, upload *receipt.ResponseUpload) (dispatchResult, *PipelineError) {
	if upload == nil {
		return result, nil
	}

	record, err := upload.Commit(ctx)
	if err != nil {
		return result, &PipelineError{Code: ControlInternalError}
	}

	responseSize, err := strconv.ParseUint(strconv.FormatInt(record.SizeBytes, 10), 10, 64)
	if err != nil {
		return result, &PipelineError{Code: ControlInternalError}
	}

	result.responseReceiptID = record.ID
	result.responseReceiptSHA256 = record.SHA256Hex
	result.size = responseSize
	result.responseUpload = nil

	return result, nil
}

type decodedResponseStreamState struct {
	dispatcher    *DefaultRequestDispatcher
	validator     *natsx.StreamValidator
	route         RouteOutcome
	deadline      time.Time
	c2eSubject    string
	in            DispatchInput
	c2eSeq        uint64
	upload        *requestBodyUpload
	uploadErr     <-chan error
	result        dispatchResult
	egressStarted time.Time
}

func (s *decodedResponseStreamState) next(ctx context.Context, ticks <-chan time.Time, frames <-chan *strawpb.StreamFrame) (bool, *PipelineError) {
	select {
	case <-ctx.Done():
		s.dispatcher.sendCancel(s.c2eSubject, s.in, s.deadline, s.c2eSeq, "client_cancelled")

		return true, &PipelineError{Code: Cancelled}
	case err := <-s.uploadErr:
		s.uploadErr = nil

		return s.uploadEvent(err)
	case <-time.After(time.Until(s.deadline)):
		s.dispatcher.sendCancel(s.c2eSubject, s.in, s.deadline, s.c2eSeq, "deadline_exceeded")

		return true, &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeTotalDeadline}
	case <-ticks:
		return false, s.dispatcher.responseStreamTick(s.validator, s.route, s.c2eSubject, s.in, s.deadline, s.c2eSeq)
	case frame, ok := <-frames:
		return s.frameEvent(frame, ok)
	}
}

func (s *decodedResponseStreamState) uploadEvent(err error) (bool, *PipelineError) {
	perr := requestStreamPipelineErrorOrNil(err)
	if perr == nil {
		return false, nil
	}

	s.dispatcher.sendCancel(s.c2eSubject, s.in, s.deadline, s.c2eSeq, "client_upload_failed")

	return true, perr
}

func (s *decodedResponseStreamState) frameEvent(frame *strawpb.StreamFrame, ok bool) (bool, *PipelineError) {
	if !ok {
		return true, s.dispatcher.streamLost(s.route, WorkerDisconnected)
	}

	return s.dispatcher.acceptResponseFrame(s.validator, frame, s.route, &s.result, &s.egressStarted, s.c2eSubject, s.in, s.deadline, &s.c2eSeq, s.upload)
}

func (d *DefaultRequestDispatcher) sendCancel(c2eSubject string, in DispatchInput, deadline time.Time, seq uint64, reason string) {
	if d.opts.NATS == nil {
		return
	}

	_ = d.publishFrame(c2eSubject, in, deadline, &strawpb.StreamFrame{
		StreamSeq: seq,
		Attempt:   defaultRequestAttempt,
		Payload:   &strawpb.StreamFrame_Cancel{Cancel: &strawpb.CancelFrame{Reason: reason}},
	})
}

func (d *DefaultRequestDispatcher) sendDownloadCredit(c2eSubject string, in DispatchInput, deadline time.Time, seq, bytes uint64) uint64 {
	if bytes == 0 || d.opts.NATS == nil {
		return seq
	}

	_ = d.publishFrame(c2eSubject, in, deadline, &strawpb.StreamFrame{
		StreamSeq: seq,
		Attempt:   defaultRequestAttempt,
		Payload:   &strawpb.StreamFrame_Credit{Credit: &strawpb.CreditFrame{DownloadCreditBytes: bytes}},
	})
	_ = d.opts.NATS.Flush()

	return seq + 1
}

func (d *DefaultRequestDispatcher) initialDownloadCredit() uint64 {
	return min(d.opts.InitialDownloadCreditBytes, d.opts.MaxInflightDownloadBytes)
}

func (d *DefaultRequestDispatcher) acceptResponseFrame(validator *natsx.StreamValidator, frame *strawpb.StreamFrame, route RouteOutcome, result *dispatchResult, egressStarted *time.Time, c2eSubject string, in DispatchInput, deadline time.Time, c2eSeq *uint64, upload *requestBodyUpload) (bool, *PipelineError) {
	ok, done, perr := acceptedResponseFrame(validator.Accept(frame))
	if !ok || done {
		return done, perr
	}

	if handled, perr := d.acceptResponseProgress(frame, result, egressStarted, validator, c2eSubject, in, deadline, c2eSeq, upload); handled {
		return false, perr
	}

	return d.acceptResponseTerminal(frame, route, result, egressStarted)
}

func (d *DefaultRequestDispatcher) acceptResponseProgress(frame *strawpb.StreamFrame, result *dispatchResult, egressStarted *time.Time, validator *natsx.StreamValidator, c2eSubject string, in DispatchInput, deadline time.Time, c2eSeq *uint64, upload *requestBodyUpload) (bool, *PipelineError) {
	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_OutboundStart:
		*egressStarted = d.opts.Now()
		result.executedFingerprintProfile = p.OutboundStart.GetExecutedFingerprintProfile()

		if !validateExecutedFingerprint(result.selectedFingerprintProfile, result.executedFingerprintProfile) {
			return true, &PipelineError{Code: ProtocolError, Message: executedFingerprintMismatchMessage}
		}
	case *strawpb.StreamFrame_ResponseStart:
		result.status = p.ResponseStart.GetStatus()
		result.headers = p.ResponseStart.GetHeaders()
	case *strawpb.StreamFrame_Data:
		_, perr := d.acceptResponseData(p.Data, result, validator, c2eSubject, in, deadline, c2eSeq, upload)

		return true, perr
	case *strawpb.StreamFrame_Credit:
		if upload != nil {
			upload.gate.grant(p.Credit.GetUploadCreditBytes())
		}
	default:
		return false, nil
	}

	return true, nil
}

func (d *DefaultRequestDispatcher) acceptResponseTerminal(frame *strawpb.StreamFrame, route RouteOutcome, result *dispatchResult, egressStarted *time.Time) (bool, *PipelineError) {
	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_Error:
		return true, d.executorError(route, p.Error)
	case *strawpb.StreamFrame_Cancelled:
		return true, &PipelineError{Code: Cancelled}
	case *strawpb.StreamFrame_End:
		result.egressMs = egressMillis(*egressStarted, d.opts.Now())

		return true, nil
	case *strawpb.StreamFrame_Trailers:
		return false, nil
	default:
		routeFailure(d.opts.Workers, route.WorkerID)

		return true, &PipelineError{Code: ProtocolError}
	}
}

func (s *rawResponseStreamState) acceptOutboundStart(frame *strawpb.OutboundStartFrame) (bool, *PipelineError) {
	s.egressStart = s.dispatcher.opts.Now()
	s.result.executedFingerprintProfile = frame.GetExecutedFingerprintProfile()

	if !validateExecutedFingerprint(s.result.selectedFingerprintProfile, s.result.executedFingerprintProfile) {
		return true, &PipelineError{Code: ProtocolError, Message: executedFingerprintMismatchMessage}
	}

	return false, nil
}

func acceptedResponseFrame(outcome natsx.FrameOutcome) (bool, bool, *PipelineError) {
	if outcome == natsx.FrameDuplicate || outcome == natsx.FrameAfterTerminal {
		return false, false, nil
	}

	if outcome != natsx.FrameAccepted {
		return false, true, &PipelineError{Code: ProtocolError}
	}

	return true, false, nil
}

func (d *DefaultRequestDispatcher) acceptResponseData(data *strawpb.DataFrame, result *dispatchResult, validator *natsx.StreamValidator, c2eSubject string, in DispatchInput, deadline time.Time, c2eSeq *uint64, upload *requestBodyUpload) (bool, *PipelineError) {
	if result.responseUpload != nil {
		err := result.responseUpload.Write(data.GetData())
		if err != nil {
			return true, &PipelineError{Code: BodyTooLarge}
		}
	} else {
		result.body = append(result.body, data.GetData()...)
		if uint64FromInt(len(result.body)) > d.opts.MaxInlineResponseBodyBytes {
			return true, &PipelineError{Code: BodyTooLarge}
		}
	}

	bytes := uint64FromInt(len(data.GetData()))
	result.size += bytes

	if upload != nil {
		upload.c2e.credit(bytes)
		*c2eSeq = upload.c2e.seq
	} else {
		*c2eSeq = d.sendDownloadCredit(c2eSubject, in, deadline, *c2eSeq, bytes)
	}

	validator.GrantCredit(bytes)

	return false, nil
}

func (d *DefaultRequestDispatcher) executorError(route RouteOutcome, frame *strawpb.ErrorFrame) *PipelineError {
	code, violation := ValidateExecutorError(frame.GetCode())
	if violation {
		routeFailure(d.opts.Workers, route.WorkerID)
	}

	return errorFramePipelineError(code, frame)
}

func (d *DefaultRequestDispatcher) responseStreamTick(validator *natsx.StreamValidator, route RouteOutcome, c2eSubject string, in DispatchInput, deadline time.Time, cancelSeq uint64) *PipelineError {
	if d.natsDisconnected() {
		return d.streamLost(route, TransportUnavailable)
	}

	if validator.IdleExpired() {
		d.sendCancel(c2eSubject, in, deadline, cancelSeq, "idle_timeout")

		return &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeIdle}
	}

	return nil
}

func (d *DefaultRequestDispatcher) streamLost(route RouteOutcome, code ErrorCode) *PipelineError {
	routeFailure(d.opts.Workers, route.WorkerID)

	if code == TransportUnavailable {
		d.opts.Metrics.IncNATSError(errorCodeLabel(code))
	}

	return &PipelineError{Code: code}
}

func (d *DefaultRequestDispatcher) natsDisconnected() bool {
	return d.opts.NATS != nil && !d.opts.NATS.IsConnected()
}

func egressMillis(start, end time.Time) int64 {
	if start.IsZero() {
		return 0
	}

	return millisSince(start, end)
}
