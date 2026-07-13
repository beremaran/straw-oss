package control

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func (d *DefaultRequestDispatcher) streamRawResponse(ctx context.Context, frames <-chan *strawpb.StreamFrame, route RouteOutcome, deadline time.Time, c2eSubject string, in DispatchInput, c2eSeq uint64, upload *requestBodyUpload, w http.ResponseWriter) (dispatchResult, *PipelineError, bool) {
	state := rawResponseStreamState{
		dispatcher: d,
		validator:  natsx.NewStreamValidator(defaultRequestAttempt, d.initialDownloadCredit(), d.opts.FrameIdleTimeout, d.opts.Now),
		route:      route,
		deadline:   deadline,
		c2eSubject: c2eSubject,
		in:         in,
		c2eSeq:     c2eSeq,
		w:          w,
		result:     dispatchResult{status: http.StatusOK, selectedFingerprintProfile: selectedFingerprintForRequest(in.Request)},
	}
	if upload != nil {
		state.upload = upload
		state.c2eSeq = upload.c2e.seq
	}

	state.uploadErr = uploadErrorChan(upload)
	defer closeUploadGate(upload)

	ticker := time.NewTicker(responseFrameCheckInterval)
	defer ticker.Stop()

	for {
		done, perr := state.next(ctx, ticker.C, frames)
		if done || perr != nil {
			return state.result, perr, state.wroteHeader
		}
	}
}

type rawResponseStreamState struct {
	dispatcher  *DefaultRequestDispatcher
	validator   *natsx.StreamValidator
	route       RouteOutcome
	deadline    time.Time
	c2eSubject  string
	in          DispatchInput
	c2eSeq      uint64
	upload      *requestBodyUpload
	uploadErr   <-chan error
	w           http.ResponseWriter
	result      dispatchResult
	egressStart time.Time
	wroteHeader bool
}

func (s *rawResponseStreamState) next(ctx context.Context, ticks <-chan time.Time, frames <-chan *strawpb.StreamFrame) (bool, *PipelineError) {
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

func (s *rawResponseStreamState) uploadEvent(err error) (bool, *PipelineError) {
	perr := requestStreamPipelineErrorOrNil(err)
	if perr == nil {
		return false, nil
	}

	s.dispatcher.sendCancel(s.c2eSubject, s.in, s.deadline, s.c2eSeq, "client_upload_failed")

	return true, perr
}

func (s *rawResponseStreamState) frameEvent(frame *strawpb.StreamFrame, ok bool) (bool, *PipelineError) {
	if !ok {
		return true, s.dispatcher.streamLost(s.route, WorkerDisconnected)
	}

	return s.acceptValidated(frame, s.validator)
}

func (s *rawResponseStreamState) acceptValidated(frame *strawpb.StreamFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	ok, done, perr := acceptedResponseFrame(validator.Accept(frame))
	if !ok || done {
		return true, perr
	}

	return s.accept(frame, validator)
}

func (s *rawResponseStreamState) accept(frame *strawpb.StreamFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	if credit := frame.GetCredit(); credit != nil {
		s.grantUploadCredit(credit)

		return false, nil
	}

	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_OutboundStart:
		return s.acceptOutboundStart(p.OutboundStart)
	case *strawpb.StreamFrame_ResponseStart:
		s.result.status = p.ResponseStart.GetStatus()
		s.result.headers = p.ResponseStart.GetHeaders()
		writeRawResponseStart(s.w, s.result.status, s.result.headers)

		s.wroteHeader = true
	case *strawpb.StreamFrame_Data:
		return false, s.writeData(p.Data, validator)
	case *strawpb.StreamFrame_Trailers:
		s.writeTrailers(p.Trailers)
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

func (s *rawResponseStreamState) writeTrailers(trailers *strawpb.TrailersFrame) {
	if !s.wroteHeader {
		return
	}

	writeRawTrailers(s.w, trailers.GetHeaders())
}

func (s *rawResponseStreamState) grantUploadCredit(credit *strawpb.CreditFrame) {
	if s.upload == nil {
		return
	}

	s.upload.gate.grant(credit.GetUploadCreditBytes())
}

func (s *rawResponseStreamState) writeData(data *strawpb.DataFrame, validator *natsx.StreamValidator) *PipelineError {
	if !s.wroteHeader {
		routeFailure(s.dispatcher.opts.Workers, s.route.WorkerID)

		return &PipelineError{Code: ProtocolError}
	}

	n, err := s.w.Write(data.GetData())
	if err != nil {
		s.dispatcher.sendCancel(s.c2eSubject, s.in, s.deadline, s.c2eSeq, "client_disconnect")

		return &PipelineError{Code: Cancelled}
	}

	written := uint64FromInt(n)
	s.result.size += written

	flushRawResponse(s.w)

	if s.upload != nil {
		s.upload.c2e.credit(written)
		s.c2eSeq = s.upload.c2e.seq
	} else {
		s.c2eSeq = s.dispatcher.sendDownloadCredit(s.c2eSubject, s.in, s.deadline, s.c2eSeq, written)
	}

	validator.GrantCredit(written)

	return nil
}

// sendCancel publishes a best-effort CancelFrame to the c2e subject.
// Cancellation is best-effort per docs/public/architecture.md; errors are silently
// dropped.
func writeRawResponseStart(w http.ResponseWriter, status uint32, headers []*strawpb.Header) {
	if stream, ok := w.(interface {
		WriteRawResponseStart(status uint32, headers []*strawpb.Header)
	}); ok {
		stream.WriteRawResponseStart(status, headers)

		return
	}

	for _, h := range headers {
		if !rawResponseHeaderAllowed(h.GetName()) {
			continue
		}

		w.Header().Add(h.GetName(), string(h.GetValue()))
	}

	w.WriteHeader(int(status))
	flushRawResponse(w)
}

func writeRawTrailers(w http.ResponseWriter, trailers []*strawpb.Header) {
	if stream, ok := w.(interface {
		WriteRawTrailers(headers []*strawpb.Header)
	}); ok {
		stream.WriteRawTrailers(trailers)

		return
	}

	for _, h := range trailers {
		if !rawResponseHeaderAllowed(h.GetName()) {
			continue
		}

		w.Header().Add(http.TrailerPrefix+h.GetName(), string(h.GetValue()))
	}

	flushRawResponse(w)
}

func rawResponseHeaderAllowed(name string) bool {
	if !isValidHTTPToken(name) || strings.HasPrefix(strings.ToLower(name), "x-straw-") {
		return false
	}

	switch strings.ToLower(name) {
	case headerNameTransferEncoding, headerNameContentLength, headerNameConnection, headerNameProxyAuthorization:
		return false
	default:
		return true
	}
}

func flushRawResponse(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
