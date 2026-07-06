package egress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const (
	// frameIdleTimeout is the P0 default from
	// docs/planning/12-nats-protocol.md ("control.transport.frame_idle_timeout_ms").
	frameIdleTimeout  = 15 * time.Second
	idleCheckInterval = 500 * time.Millisecond
	// workerDrainTimeout mirrors the Control drain timeout default in
	// docs/planning/29-operational-behavior.md; egress has no separate
	// configured default yet.
	workerDrainTimeout       = 60 * time.Second
	streamFrameChannelBuffer = 32
)

var errExecutorRequired = errors.New("executor is required")

const rawTunnelEstablishedStatus = 200

var supportedModes = []strawpb.RequestMode{
	strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
	strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL,
}

// Worker runs the live NATS assignment execution loop for one registered
// worker session (docs/planning/12 "Assignment Flow", docs/planning/09 steps
// 13-21).
type Worker struct {
	conn      *natsx.Connection
	id        Identity
	executor  *Executor
	sessionID string

	maxConcurrency uint32

	mu       sync.Mutex
	active   uint32
	draining bool
	cancels  map[string]context.CancelFunc

	wg sync.WaitGroup
}

// NewWorker builds a Worker bound to a live registered session.
func NewWorker(conn *natsx.Connection, id Identity, executor *Executor, sessionID string, maxConcurrency uint32) (*Worker, error) {
	if conn == nil {
		return nil, errConnRequired
	}

	if executor == nil {
		return nil, errExecutorRequired
	}

	err := natsx.ValidateSubjectToken(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session_id: %w", err)
	}

	return &Worker{
		conn:           conn,
		id:             id,
		executor:       executor,
		sessionID:      sessionID,
		maxConcurrency: maxConcurrency,
		cancels:        make(map[string]context.CancelFunc),
	}, nil
}

// ActiveRequests reports the number of assignments currently executing.
func (w *Worker) ActiveRequests() uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.active
}

// Serve subscribes to this worker's exact-session assignment subject (never a
// queue group, per docs/planning/12) and processes assignments until stop is
// closed. It then stops accepting new assignments and drains in-flight
// requests until they finish or workerDrainTimeout elapses (docs/planning/29
// "Worker Graceful Shutdown", steps 2-3). stop is a plain signal channel
// rather than a context.Context: the caller decides exactly when draining
// begins (e.g. after sending a draining heartbeat), which a context
// inherited from the caller's own lifecycle cannot express without racing
// that ordering.
func (w *Worker) Serve(stop <-chan struct{}) error {
	defer w.closeExecutorIdleConnections()

	subject, err := natsx.AssignmentSubject(w.id.WorkerID, w.sessionID)
	if err != nil {
		return fmt.Errorf("assignment subject: %w", err)
	}

	sub, err := w.conn.Subscribe(subject, w.handleAssign)
	if err != nil {
		return fmt.Errorf("subscribe assignment: %w", err)
	}

	err = w.conn.Flush()
	if err != nil {
		_ = sub.Unsubscribe()

		return fmt.Errorf("flush assignment subscription: %w", err)
	}

	<-stop

	w.mu.Lock()
	w.draining = true
	w.mu.Unlock()

	_ = sub.Unsubscribe()

	drained := make(chan struct{})

	go func() {
		w.wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(workerDrainTimeout):
		w.abandonInFlight()
		<-drained
	}

	return nil
}

func (w *Worker) closeExecutorIdleConnections() {
	w.executor.CloseIdleConnections()
}

func (w *Worker) abandonInFlight() {
	w.mu.Lock()

	cancels := make([]context.CancelFunc, 0, len(w.cancels))
	for _, cancel := range w.cancels {
		cancels = append(cancels, cancel)
	}
	w.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

// handleAssign implements the executor side of the assignment flow
// (docs/planning/12 "Assignment Flow", steps 3-5): evaluate and reserve
// capacity, subscribe and flush the request-scoped c2e subject before
// accepting, then reply with AssignAck.
func (w *Worker) handleAssign(msg *nats.Msg) {
	env, decodeErr := natsx.UnmarshalEnvelope(msg.Data)
	if decodeErr != nil {
		w.reply(msg, nil, &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_ERROR})

		return
	}

	req := env.GetAssignRequest()

	code := EvaluateAssignment(req, w.snapshotCapacity())
	if code != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		w.reply(msg, env, &strawpb.AssignAck{Code: code})

		return
	}

	requestID := env.GetRequestId()

	_, e2cSubject, c2eSub, frames, ok := w.prepareRequestStream(env)
	if !ok {
		w.reply(msg, env, &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_ERROR})

		return
	}

	reqCtx, cancel := requestContext(env.GetDeadlineUnixMs())
	w.reserve(requestID, cancel)

	w.reply(msg, env, &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED})

	w.wg.Go(func() {
		defer func() { _ = c2eSub.Unsubscribe() }()
		defer w.release(requestID)

		w.runRequest(reqCtx, cancel, req, env, frames, e2cSubject)
	})
}

// prepareRequestStream resolves the request-scoped c2e/e2c subjects and
// subscribes and flushes c2e (docs/planning/12 "Assignment Flow" step 4)
// before the caller replies with AssignAck.
func (w *Worker) prepareRequestStream(env *strawpb.Envelope) (string, string, *nats.Subscription, chan *strawpb.StreamFrame, bool) {
	requestID := env.GetRequestId()

	c2eSubject, err := natsx.StreamSubject(requestID, w.id.WorkerID, w.sessionID, natsx.DirectionControlToExecutor)
	if err != nil {
		return "", "", nil, nil, false
	}

	e2cSubject, err := natsx.StreamSubject(requestID, w.id.WorkerID, w.sessionID, natsx.DirectionExecutorToControl)
	if err != nil {
		return "", "", nil, nil, false
	}

	frames := make(chan *strawpb.StreamFrame, streamFrameChannelBuffer)

	c2eSub, err := w.conn.Subscribe(c2eSubject, func(m *nats.Msg) {
		frames <- decodeStreamFrame(m.Data)
	})
	if err != nil {
		return "", "", nil, nil, false
	}

	err = w.conn.Flush()
	if err != nil {
		_ = c2eSub.Unsubscribe()

		return "", "", nil, nil, false
	}

	return c2eSubject, e2cSubject, c2eSub, frames, true
}

func (w *Worker) snapshotCapacity() Capacity {
	w.mu.Lock()
	defer w.mu.Unlock()

	return Capacity{
		Draining:       w.draining,
		ActiveRequests: w.active,
		MaxConcurrency: w.maxConcurrency,
		SupportedModes: supportedModes,
	}
}

func (w *Worker) reserve(requestID string, cancel context.CancelFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.active++
	w.cancels[requestID] = cancel
}

func (w *Worker) release(requestID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.active--
	delete(w.cancels, requestID)
}

func requestContext(deadlineUnixMs int64) (context.Context, context.CancelFunc) {
	if deadlineUnixMs <= 0 {
		return context.WithCancel(context.Background())
	}

	return context.WithDeadline(context.Background(), time.UnixMilli(deadlineUnixMs))
}

// runRequest waits for RequestStart and its inline body over the c2e stream,
// executes the outbound request, and publishes the resulting frames to e2c
// (docs/planning/09 steps 16-21). If RequestStart never arrives (deadline,
// protocol error, or a pre-start Cancel), nothing is published: Control
// synthesizes the terminal outcome per docs/planning/09 "Terminal Rule".
func (w *Worker) runRequest(reqCtx context.Context, cancel context.CancelFunc, req *strawpb.AssignRequest, env *strawpb.Envelope, frames <-chan *strawpb.StreamFrame, e2cSubject string) {
	defer cancel()

	validator := natsx.NewStreamValidator(req.GetAttempt(), req.GetInitialUploadCreditBytes(), frameIdleTimeout, nil)

	start, body, ok := readRequestBody(reqCtx, validator, frames, req.GetExpectedUploadBytes())
	if !ok {
		return
	}

	if start.GetMode() == strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL {
		w.runRawTunnel(reqCtx, cancel, req, env, start, frames, validator, e2cSubject)

		return
	}

	resultCh := make(chan []*strawpb.StreamFrame, 1)
	downloadCredit := newResponseCreditGate(req.GetInitialDownloadCreditBytes())

	go func() {
		seq := uint64(0)

		resultCh <- w.executor.ExecuteWithTenant(reqCtx, env.GetTenantId(), start, body, req.GetAttempt(), func(frame *strawpb.StreamFrame) {
			if data := frame.GetData(); data != nil {
				offset := data.GetOffset()

				remaining := data.GetData()
				for len(remaining) > 0 {
					n, ok := downloadCredit.takeAvailable(reqCtx, min(len(remaining), responseFrameDataBytes))
					if !ok {
						return
					}

					seq++
					chunk := remaining[:n]
					w.publish(e2cSubject, env, []*strawpb.StreamFrame{{
						StreamSeq: seq,
						Attempt:   frame.GetAttempt(),
						Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: offset, Data: append([]byte(nil), chunk...)}},
					}})
					offset += uint64FromInt(len(chunk))
					remaining = remaining[n:]
				}

				return
			}

			seq++
			frame.StreamSeq = seq

			// Publish OutboundStart before DNS/connect (docs/planning/09
			// step 19) so Control measures the real egress phase instead of
			// receiving it batched with the terminal frame.
			w.publish(e2cSubject, env, []*strawpb.StreamFrame{frame})
		})
	}()

	result, canceled, reason := waitForResult(resultCh, frames, validator, cancel, downloadCredit)
	if canceled {
		result = applyCancellation(result, reason)
	}

	w.publish(e2cSubject, env, result)
}

func (w *Worker) runRawTunnel(ctx context.Context, cancel context.CancelFunc, req *strawpb.AssignRequest, env *strawpb.Envelope, start *strawpb.RequestStart, frames <-chan *strawpb.StreamFrame, validator *natsx.StreamValidator, e2cSubject string) {
	defer cancel()

	conn, target, failure := w.executor.openTunnel(ctx, start)

	builder := newSafeFrameBuilder(req.GetAttempt())
	if failure != nil {
		w.publish(e2cSubject, env, []*strawpb.StreamFrame{builder.outboundStart(target.host, target.port), builder.error(failure)})

		return
	}

	defer func() { _ = conn.Close() }()

	stream := rawTunnelStream{
		worker:    w,
		env:       env,
		subject:   e2cSubject,
		conn:      conn,
		builder:   builder,
		validator: validator,
		credit:    newResponseCreditGate(req.GetInitialDownloadCreditBytes()),
	}
	stream.publish(builder.outboundStart(target.host, target.port), builder.responseStart(rawTunnelEstablishedStatus, nil))
	stream.run(ctx, frames)
}

type rawTunnelStream struct {
	worker    *Worker
	env       *strawpb.Envelope
	subject   string
	conn      net.Conn
	builder   *safeFrameBuilder
	validator *natsx.StreamValidator
	credit    *responseCreditGate
}

func (s rawTunnelStream) run(ctx context.Context, frames <-chan *strawpb.StreamFrame) {
	done := make(chan error, 1)

	go streamTunnelDownload(ctx, s.conn, s.credit, s.builder, s.publishOne, done)

	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			s.handleDone(err)

			return
		case <-ctx.Done():
			s.publishOne(s.builder.error(mapHTTPError(ctx, ctx.Err())))

			return
		case <-ticker.C:
			if s.validator.IdleExpired() {
				return
			}
		case frame := <-frames:
			if s.handleFrame(ctx, frame) {
				return
			}
		}
	}
}

func (s rawTunnelStream) handleDone(err error) {
	if err != nil && !errors.Is(err, io.EOF) {
		s.publishOne(s.builder.error(executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, upstreamResetFact)))

		return
	}

	s.publishOne(s.builder.end())
}

func (s rawTunnelStream) handleFrame(ctx context.Context, frame *strawpb.StreamFrame) bool {
	if s.validator.Accept(frame) != natsx.FrameAccepted {
		return false
	}

	if data := frame.GetData(); data != nil {
		return s.handleData(ctx, data)
	}

	if credit := frame.GetCredit(); credit != nil {
		s.credit.grant(credit.GetDownloadCreditBytes())

		return false
	}

	if cancelFrame := frame.GetCancel(); cancelFrame != nil {
		s.publishOne(s.builder.cancelled(cancelFrame.GetReason()))

		return true
	}

	return false
}

func (s rawTunnelStream) handleData(ctx context.Context, data *strawpb.DataFrame) bool {
	_, err := s.conn.Write(data.GetData())
	if err != nil {
		s.publishOne(s.builder.error(mapHTTPError(ctx, err)))

		return true
	}

	s.publishOne(s.builder.uploadCredit(uint64FromInt(len(data.GetData()))))

	return false
}

func (s rawTunnelStream) publish(frames ...*strawpb.StreamFrame) {
	s.worker.publish(s.subject, s.env, frames)
}

func (s rawTunnelStream) publishOne(frame *strawpb.StreamFrame) {
	s.publish(frame)
}

func streamTunnelDownload(ctx context.Context, conn net.Conn, credit *responseCreditGate, builder *safeFrameBuilder, publish func(*strawpb.StreamFrame), done chan<- error) {
	buf := make([]byte, responseFrameDataBytes)
	offset := uint64(0)

	for {
		n, err := conn.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if !credit.take(ctx, uint64FromInt(n)) {
				done <- ctx.Err()

				return
			}

			publish(builder.data(offset, chunk))
			offset += uint64FromInt(n)
		}

		if err != nil {
			done <- err

			return
		}
	}
}

type safeFrameBuilder struct {
	mu sync.Mutex
	b  *frameBuilder
}

func newSafeFrameBuilder(attempt uint32) *safeFrameBuilder {
	return &safeFrameBuilder{b: newFrameBuilder(attempt)}
}

func (b *safeFrameBuilder) outboundStart(host string, port uint32) *strawpb.StreamFrame {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.b.outboundStart(host, port)
}

func (b *safeFrameBuilder) responseStart(status uint32, headers []*strawpb.Header) *strawpb.StreamFrame {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.b.responseStart(status, headers)
}

func (b *safeFrameBuilder) data(offset uint64, data []byte) *strawpb.StreamFrame {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.b.data(offset, data)
}

func (b *safeFrameBuilder) uploadCredit(bytes uint64) *strawpb.StreamFrame {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.b.seq,
		Attempt:   b.b.attempt,
		Payload:   &strawpb.StreamFrame_Credit{Credit: &strawpb.CreditFrame{UploadCreditBytes: bytes}},
	}
}

func (b *safeFrameBuilder) cancelled(reason string) *strawpb.StreamFrame {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.b.seq,
		Attempt:   b.b.attempt,
		Payload:   &strawpb.StreamFrame_Cancelled{Cancelled: &strawpb.CancelledFrame{Reason: reason}},
	}
}

func (b *safeFrameBuilder) end() *strawpb.StreamFrame {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.b.end()
}

func (b *safeFrameBuilder) error(failure *executionError) *strawpb.StreamFrame {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.b.error(failure)
}

// readRequestBody accepts c2e frames until RequestStart and its full inline
// body (sized by AssignRequest.ExpectedUploadBytes) have arrived. P0 does not
// support BodyRef (docs/planning/16), so the body is always inline DataFrames.
func readRequestBody(ctx context.Context, validator *natsx.StreamValidator, frames <-chan *strawpb.StreamFrame, expectedUploadBytes int64) (*strawpb.RequestStart, []byte, bool) {
	state := &requestBodyState{}
	if expectedUploadBytes > 0 {
		state.expected = uint64(expectedUploadBytes)
	}

	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for !state.complete() {
		select {
		case <-ctx.Done():
			return nil, nil, false
		case frame, ok := <-frames:
			if !ok || !state.accept(validator, frame) {
				return nil, nil, false
			}
		case <-ticker.C:
			if validator.IdleExpired() {
				return nil, nil, false
			}
		}
	}

	return state.start, state.body, true
}

// requestBodyState accumulates c2e RequestStart/DataFrame payloads into the
// inline request body Executor.Execute expects.
type requestBodyState struct {
	start    *strawpb.RequestStart
	body     []byte
	received uint64
	expected uint64
}

func (s *requestBodyState) complete() bool {
	return s.start != nil && s.received >= s.expected
}

// accept applies one c2e frame to the body state. It returns false when the
// frame is a protocol violation, an unexpected payload, or a pre-start
// Cancel, any of which abort the request per docs/planning/09 "Terminal
// Rule" (nothing is published; Control synthesizes the outcome).
func (s *requestBodyState) accept(validator *natsx.StreamValidator, frame *strawpb.StreamFrame) bool {
	outcome := validator.Accept(frame)
	if outcome == natsx.FrameDuplicate {
		return true
	}

	if outcome != natsx.FrameAccepted {
		return false
	}

	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_RequestStart:
		s.start = p.RequestStart
	case *strawpb.StreamFrame_Data:
		s.body = append(s.body, p.Data.GetData()...)
		s.received += uint64(len(p.Data.GetData()))
	case *strawpb.StreamFrame_Credit:
		// Download credit grants are consumed after RequestStart while the
		// executor streams response DataFrames.
	default:
		return false
	}

	return true
}

// waitForResult waits for the outbound execution to finish while still
// accepting c2e frames, most importantly a CancelFrame, which cancels
// execCtx so Executor.Execute aborts promptly.
func waitForResult(resultCh <-chan []*strawpb.StreamFrame, frames <-chan *strawpb.StreamFrame, validator *natsx.StreamValidator, cancel context.CancelFunc, downloadCredit *responseCreditGate) ([]*strawpb.StreamFrame, bool, string) {
	for {
		select {
		case result := <-resultCh:
			return result, false, ""
		case frame, ok := <-frames:
			if !ok {
				continue
			}

			if validator.Accept(frame) != natsx.FrameAccepted {
				continue
			}

			if credit := frame.GetCredit(); credit != nil {
				downloadCredit.grant(credit.GetDownloadCreditBytes())

				continue
			}

			cancelFrame, isCancel := frame.GetPayload().(*strawpb.StreamFrame_Cancel)
			if !isCancel {
				continue
			}

			cancel()

			return <-resultCh, true, cancelFrame.Cancel.GetReason()
		}
	}
}

type responseCreditGate struct {
	credit uint64
	grants chan uint64
}

func newResponseCreditGate(initial uint64) *responseCreditGate {
	if initial == 0 {
		return nil
	}

	return &responseCreditGate{credit: initial, grants: make(chan uint64, streamFrameChannelBuffer)}
}

func (g *responseCreditGate) grant(bytes uint64) {
	if g == nil || bytes == 0 {
		return
	}

	g.grants <- bytes
}

func (g *responseCreditGate) take(ctx context.Context, bytes uint64) bool {
	if g == nil || bytes == 0 {
		return true
	}

	for g.credit < bytes {
		select {
		case <-ctx.Done():
			return false
		case grant := <-g.grants:
			g.credit += grant
		}
	}

	g.credit -= bytes

	return true
}

func (g *responseCreditGate) takeAvailable(ctx context.Context, limit int) (int, bool) {
	if g == nil {
		return limit, true
	}

	for g.credit == 0 {
		select {
		case <-ctx.Done():
			return 0, false
		case grant := <-g.grants:
			g.credit += grant
		}
	}

	if g.credit < uint64FromInt(limit) {
		g.credit--

		return 1, true
	}

	g.credit -= uint64FromInt(limit)

	return limit, true
}

// applyCancellation replaces the terminal frame of a Cancel-aborted execution
// with a CancelledFrame (docs/planning/09 "Terminal Rule"), unless execution
// had already completed successfully before the cancel took effect
// (cancellation is best effort).
func applyCancellation(frames []*strawpb.StreamFrame, reason string) []*strawpb.StreamFrame {
	if len(frames) == 0 {
		return frames
	}

	last := frames[len(frames)-1]
	if _, ok := last.GetPayload().(*strawpb.StreamFrame_End); ok {
		return frames
	}

	frames[len(frames)-1] = &strawpb.StreamFrame{
		StreamSeq: last.GetStreamSeq(),
		Attempt:   last.GetAttempt(),
		Payload:   &strawpb.StreamFrame_Cancelled{Cancelled: &strawpb.CancelledFrame{Reason: reason}},
	}

	return frames
}

func (w *Worker) publish(subject string, env *strawpb.Envelope, frames []*strawpb.StreamFrame) {
	for _, frame := range frames {
		out := &strawpb.Envelope{
			RequestId:      env.GetRequestId(),
			TenantId:       env.GetTenantId(),
			TraceId:        env.GetTraceId(),
			DeadlineUnixMs: env.GetDeadlineUnixMs(),
			ProtocolMajor:  ProtocolMajor,
			Attempt:        env.GetAttempt(),
			Payload:        &strawpb.Envelope_StreamFrame{StreamFrame: frame},
		}

		raw, err := natsx.MarshalEnvelope(out)
		if err != nil {
			return
		}

		err = w.conn.Publish(subject, raw)
		if err != nil {
			return
		}
	}
}

func decodeStreamFrame(raw []byte) *strawpb.StreamFrame {
	env, err := natsx.UnmarshalEnvelope(raw)
	if err != nil {
		return nil
	}

	return env.GetStreamFrame()
}

func (w *Worker) reply(msg *nats.Msg, env *strawpb.Envelope, ack *strawpb.AssignAck) {
	reply := &strawpb.Envelope{
		ProtocolMajor: ProtocolMajor,
		Payload:       &strawpb.Envelope_AssignAck{AssignAck: ack},
	}

	if env != nil {
		reply.RequestId = env.GetRequestId()
		reply.TenantId = env.GetTenantId()
		reply.TraceId = env.GetTraceId()
		reply.DeadlineUnixMs = env.GetDeadlineUnixMs()
		reply.Attempt = env.GetAttempt()
	}

	raw, err := natsx.MarshalEnvelope(reply)
	if err != nil {
		return
	}

	_ = msg.Respond(raw)
}
