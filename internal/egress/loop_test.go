package egress

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/testutil"
)

const (
	loopTestWorker  = "loop_worker"
	loopTestSession = "loop_session"
	loopTestTenant  = "ten_loop"
)

func TestWorkerAcceptsAssignmentExecutesAndPublishesTerminalFrame(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, "loop.test")
	executor := NewExecutor(ExecutorOptions{Resolver: staticResolver{"loop.test": loopbackIP(t, server.URL)}})

	h := newLoopHarness(t, executor, 4)

	ack := h.assign(t, "req_1", &strawpb.AssignRequest{
		Mode:                     strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs:           time.Now().Add(5 * time.Second).UnixMilli(),
		Attempt:                  1,
		InitialUploadCreditBytes: 1 << 20,
	})
	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("assign ack code = %v, want accepted", ack.GetCode())
	}

	h.sendC2E(t, "req_1", &strawpb.StreamFrame{
		StreamSeq: 1,
		Attempt:   1,
		Payload:   &strawpb.StreamFrame_RequestStart{RequestStart: requestStart(target, directPolicy(true))},
	})

	frames := h.collectTerminal(t, "req_1")
	if len(frames) != 4 {
		t.Fatalf("len(frames) = %d, want 4: %#v", len(frames), frames)
	}
	if got := frames[0].GetOutboundStart(); got == nil || got.GetTargetHost() != "loop.test" {
		t.Fatalf("outbound start = %#v", got)
	}
	if got := frames[3].GetEnd(); got == nil || !got.GetSuccess() {
		t.Fatalf("terminal frame = %#v, want successful EndFrame", frames[3])
	}

	if got := h.worker.ActiveRequests(); got != 0 {
		t.Fatalf("ActiveRequests() = %d, want 0 after completion", got)
	}
}

func TestWorkerRejectsAssignmentAtCapacity(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(ExecutorOptions{Resolver: staticResolver{}})
	h := newLoopHarness(t, executor, 1)

	ack1 := h.assign(t, "req_full", &strawpb.AssignRequest{
		Mode:                     strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs:           time.Now().Add(200 * time.Millisecond).UnixMilli(),
		Attempt:                  1,
		InitialUploadCreditBytes: 1 << 20,
	})
	if ack1.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("first assign ack code = %v, want accepted", ack1.GetCode())
	}

	ack2 := h.assign(t, "req_overflow", &strawpb.AssignRequest{
		Mode:           strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs: time.Now().Add(5 * time.Second).UnixMilli(),
		Attempt:        1,
	})
	if ack2.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_CAPACITY {
		t.Fatalf("second assign ack code = %v, want rejected_capacity", ack2.GetCode())
	}

	// req_full's envelope deadline expires without a RequestStart; capacity
	// releases and nothing is published (Control synthesizes the outcome).
	waitForCondition(t, 2*time.Second, func() bool { return h.worker.ActiveRequests() == 0 })
}

func TestWorkerCancelFrameDuringExecutionProducesCancelledFrame(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-release:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})

	target := rewriteHost(t, server.URL, "cancel.test")
	executor := NewExecutor(ExecutorOptions{Resolver: staticResolver{"cancel.test": loopbackIP(t, server.URL)}})

	h := newLoopHarness(t, executor, 4)

	ack := h.assign(t, "req_cancel", &strawpb.AssignRequest{
		Mode:                     strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs:           time.Now().Add(5 * time.Second).UnixMilli(),
		Attempt:                  1,
		InitialUploadCreditBytes: 1 << 20,
	})
	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("assign ack code = %v, want accepted", ack.GetCode())
	}

	h.sendC2E(t, "req_cancel", &strawpb.StreamFrame{
		StreamSeq: 1,
		Attempt:   1,
		Payload:   &strawpb.StreamFrame_RequestStart{RequestStart: requestStart(target, directPolicy(true))},
	})

	// Give the executor a moment to reach the in-flight HTTP call before
	// canceling, so the cancellation races a real outbound request.
	time.Sleep(50 * time.Millisecond)
	h.sendC2E(t, "req_cancel", &strawpb.StreamFrame{
		StreamSeq: 2,
		Attempt:   1,
		Payload:   &strawpb.StreamFrame_Cancel{Cancel: &strawpb.CancelFrame{Reason: "client_disconnect"}},
	})

	frames := h.collectTerminal(t, "req_cancel")
	last := frames[len(frames)-1]
	cancelled := last.GetCancelled()
	if cancelled == nil || cancelled.GetReason() != "client_disconnect" {
		t.Fatalf("terminal frame = %#v, want CancelledFrame(reason=client_disconnect)", last)
	}
}

func TestWorkerCreditExhaustionAbortsWithoutPublishing(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(ExecutorOptions{Resolver: staticResolver{}})
	h := newLoopHarness(t, executor, 4)

	ack := h.assign(t, "req_credit", &strawpb.AssignRequest{
		Mode:                     strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs:           time.Now().Add(300 * time.Millisecond).UnixMilli(),
		ExpectedUploadBytes:      4,
		Attempt:                  1,
		InitialUploadCreditBytes: 1,
	})
	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("assign ack code = %v, want accepted", ack.GetCode())
	}

	rs := requestStart("http://credit.test/", directPolicy(true))
	h.sendC2E(t, "req_credit", &strawpb.StreamFrame{StreamSeq: 1, Attempt: 1, Payload: &strawpb.StreamFrame_RequestStart{RequestStart: rs}})
	h.sendC2E(t, "req_credit", &strawpb.StreamFrame{StreamSeq: 2, Attempt: 1, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 0, Data: []byte("body")}}})

	select {
	case frame := <-h.e2cChan(t, "req_credit"):
		t.Fatalf("unexpected e2c frame after credit exhaustion: %#v", frame)
	case <-time.After(300 * time.Millisecond):
	}

	waitForCondition(t, 2*time.Second, func() bool { return h.worker.ActiveRequests() == 0 })
}

func TestWorkerDownloadCreditGatesResponseData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ab"))
	}))
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, "download-credit.test")
	executor := NewExecutor(ExecutorOptions{Resolver: staticResolver{"download-credit.test": loopbackIP(t, server.URL)}})
	h := newLoopHarness(t, executor, 4)

	ack := h.assign(t, "req_download_credit", &strawpb.AssignRequest{
		Mode:                       strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs:             time.Now().Add(5 * time.Second).UnixMilli(),
		Attempt:                    1,
		InitialUploadCreditBytes:   1 << 20,
		InitialDownloadCreditBytes: 1,
	})
	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("assign ack code = %v, want accepted", ack.GetCode())
	}

	h.sendC2E(t, "req_download_credit", &strawpb.StreamFrame{
		StreamSeq: 1,
		Attempt:   1,
		Payload:   &strawpb.StreamFrame_RequestStart{RequestStart: requestStart(target, directPolicy(true))},
	})

	ch := h.e2cChan(t, "req_download_credit")
	for i := range 2 {
		frame := <-ch
		if frame.GetOutboundStart() == nil && frame.GetResponseStart() == nil {
			t.Fatalf("frame %d = %#v, want outbound/response start before data", i, frame)
		}
	}

	first := <-ch
	if got := string(first.GetData().GetData()); got != "a" {
		t.Fatalf("first data = %q, want a", got)
	}

	select {
	case frame := <-ch:
		t.Fatalf("unexpected response frame before download credit was replenished: %#v", frame)
	case <-time.After(100 * time.Millisecond):
	}

	h.sendC2E(t, "req_download_credit", &strawpb.StreamFrame{
		StreamSeq: 2,
		Attempt:   1,
		Payload:   &strawpb.StreamFrame_Credit{Credit: &strawpb.CreditFrame{DownloadCreditBytes: 1}},
	})

	frames := h.collectTerminal(t, "req_download_credit")
	if len(frames) != 2 {
		t.Fatalf("remaining frame count = %d, want data and end: %#v", len(frames), frames)
	}
	if got := string(frames[0].GetData().GetData()); got != "b" {
		t.Fatalf("data = %q, want b", got)
	}
	if frames[1].GetEnd() == nil {
		t.Fatalf("terminal frame = %#v, want EndFrame", frames[1])
	}
}

func TestWorkerShutdownDrainsInFlightRequestBeforeReturning(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	// Release the handler even when an assertion fails, or server.Close
	// deadlocks on the blocked in-flight request.
	t.Cleanup(releaseOnce)

	target := rewriteHost(t, server.URL, "drain.test")
	executor := NewExecutor(ExecutorOptions{Resolver: staticResolver{"drain.test": loopbackIP(t, server.URL)}})

	h := newLoopHarness(t, executor, 4)

	ack := h.assign(t, "req_drain", &strawpb.AssignRequest{
		Mode:                     strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs:           time.Now().Add(5 * time.Second).UnixMilli(),
		Attempt:                  1,
		InitialUploadCreditBytes: 1 << 20,
	})
	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("assign ack code = %v, want accepted", ack.GetCode())
	}

	h.sendC2E(t, "req_drain", &strawpb.StreamFrame{
		StreamSeq: 1,
		Attempt:   1,
		Payload:   &strawpb.StreamFrame_RequestStart{RequestStart: requestStart(target, directPolicy(true))},
	})
	time.Sleep(50 * time.Millisecond)

	serveDone := make(chan struct{})

	go func() {
		h.cancel()
		close(serveDone)
	}()

	select {
	case <-serveDone:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel() did not return")
	}

	// OutboundStart is published before connect (docs/planning/09 step 19),
	// so it arrives while the handler is still blocked. The in-flight request
	// must not be aborted just because the worker began shutting down: no
	// terminal frame may arrive until the handler is released.
	select {
	case frame := <-h.e2cChan(t, "req_drain"):
		if frame.GetOutboundStart() == nil {
			t.Fatalf("unexpected frame before handler released: %#v", frame)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("OutboundStart not published before connect")
	}

	select {
	case frame := <-h.e2cChan(t, "req_drain"):
		t.Fatalf("request completed before handler released: %#v", frame)
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce()

	frames := h.collectTerminal(t, "req_drain")
	if got := frames[len(frames)-1].GetEnd(); got == nil || !got.GetSuccess() {
		t.Fatalf("terminal frame = %#v, want successful EndFrame", frames[len(frames)-1])
	}
}

// loopHarness drives a live Worker over a real (fake) NATS broker, playing
// Control's side of the assignment/stream protocol.
type loopHarness struct {
	workerConn  *natsx.Connection
	controlConn *natsx.Connection
	worker      *Worker
	cancel      func()

	e2cSubs map[string]chan *strawpb.StreamFrame
}

func newLoopHarness(t *testing.T, executor *Executor, maxConcurrency uint32) *loopHarness {
	t.Helper()

	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	workerConn := loopConnect(t, srv.URL())
	controlConn := loopConnect(t, srv.URL())

	worker, err := NewWorker(workerConn, Identity{WorkerID: loopTestWorker}, executor, loopTestSession, maxConcurrency)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	stop := make(chan struct{})
	cancel := sync.OnceFunc(func() { close(stop) })

	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		_ = worker.Serve(stop)
	}()
	t.Cleanup(func() {
		cancel()
		<-serveDone
	})

	return &loopHarness{
		workerConn:  workerConn,
		controlConn: controlConn,
		worker:      worker,
		cancel:      cancel,
		e2cSubs:     make(map[string]chan *strawpb.StreamFrame),
	}
}

func (h *loopHarness) assign(t *testing.T, requestID string, req *strawpb.AssignRequest) *strawpb.AssignAck {
	t.Helper()

	// Subscribe and flush the request-scoped e2c subject before sending the
	// AssignRequest, mirroring Control's own ordering obligation.
	h.subscribeE2C(t, requestID)

	subject, err := natsx.AssignmentSubject(loopTestWorker, loopTestSession)
	if err != nil {
		t.Fatalf("AssignmentSubject() error = %v", err)
	}

	env := &strawpb.Envelope{
		RequestId:      requestID,
		TenantId:       loopTestTenant,
		DeadlineUnixMs: req.GetDeadlineUnixMs(),
		ProtocolMajor:  ProtocolMajor,
		Attempt:        req.GetAttempt(),
		Payload:        &strawpb.Envelope_AssignRequest{AssignRequest: req},
	}

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		t.Fatalf("MarshalEnvelope() error = %v", err)
	}

	var reply []byte

	deadline := time.Now().Add(2 * time.Second)
	for {
		msg, reqErr := h.controlConn.Request(subject, raw, 200*time.Millisecond)
		if reqErr == nil {
			reply = msg.Data

			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("assign request %s: %v", requestID, reqErr)
		}
	}

	replyEnv, err := natsx.UnmarshalEnvelope(reply)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope(reply) error = %v", err)
	}

	return replyEnv.GetAssignAck()
}

func (h *loopHarness) sendC2E(t *testing.T, requestID string, frame *strawpb.StreamFrame) {
	t.Helper()

	subject, err := natsx.StreamSubject(requestID, loopTestWorker, loopTestSession, natsx.DirectionControlToExecutor)
	if err != nil {
		t.Fatalf("StreamSubject(c2e) error = %v", err)
	}

	env := &strawpb.Envelope{
		RequestId:      requestID,
		TenantId:       loopTestTenant,
		ProtocolMajor:  ProtocolMajor,
		Attempt:        frame.GetAttempt(),
		DeadlineUnixMs: time.Now().Add(5 * time.Second).UnixMilli(),
		Payload:        &strawpb.Envelope_StreamFrame{StreamFrame: frame},
	}

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		t.Fatalf("MarshalEnvelope(c2e) error = %v", err)
	}

	err = h.controlConn.Publish(subject, raw)
	if err != nil {
		t.Fatalf("Publish(c2e) error = %v", err)
	}

	err = h.controlConn.Flush()
	if err != nil {
		t.Fatalf("Flush(c2e) error = %v", err)
	}
}

func (h *loopHarness) subscribeE2C(t *testing.T, requestID string) {
	t.Helper()

	subject, err := natsx.StreamSubject(requestID, loopTestWorker, loopTestSession, natsx.DirectionExecutorToControl)
	if err != nil {
		t.Fatalf("StreamSubject(e2c) error = %v", err)
	}

	ch := make(chan *strawpb.StreamFrame, streamFrameChannelBuffer)
	h.e2cSubs[requestID] = ch

	_, err = h.controlConn.Subscribe(subject, func(msg *nats.Msg) {
		ch <- decodeStreamFrame(msg.Data)
	})
	if err != nil {
		t.Fatalf("Subscribe(e2c) error = %v", err)
	}

	err = h.controlConn.Flush()
	if err != nil {
		t.Fatalf("Flush(e2c subscribe) error = %v", err)
	}
}

// e2cChan returns the collector channel for a request that has already been
// assigned (assign() subscribes it before sending AssignRequest).
func (h *loopHarness) e2cChan(t *testing.T, requestID string) chan *strawpb.StreamFrame {
	t.Helper()

	ch, ok := h.e2cSubs[requestID]
	if !ok {
		t.Fatalf("no e2c subscription for request %s", requestID)
	}

	return ch
}

// collectTerminal reads e2c frames for requestID until a terminal frame
// (EndFrame, ErrorFrame, or CancelledFrame) arrives, per docs/planning/09
// "Terminal Rule", and returns every frame observed in order.
func (h *loopHarness) collectTerminal(t *testing.T, requestID string) []*strawpb.StreamFrame {
	t.Helper()

	ch := h.e2cChan(t, requestID)

	var frames []*strawpb.StreamFrame

	deadline := time.After(5 * time.Second)

	for {
		select {
		case frame := <-ch:
			frames = append(frames, frame)

			switch frame.GetPayload().(type) {
			case *strawpb.StreamFrame_End, *strawpb.StreamFrame_Error, *strawpb.StreamFrame_Cancelled:
				return frames
			}
		case <-deadline:
			t.Fatalf("timed out waiting for terminal frame on %s, got %d frames", requestID, len(frames))
		}
	}
}

func loopConnect(t *testing.T, url string) *natsx.Connection {
	t.Helper()

	conn, err := natsx.Connect(natsx.ConnectOptions{
		Servers:         []string{url},
		ReconnectWait:   10 * time.Millisecond,
		PingInterval:    100 * time.Millisecond,
		MaxPingFailures: 1,
	})
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}

	t.Cleanup(conn.Close)

	return conn
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("condition not met within %s", timeout)
}
