package main

// Integration test for the static-response example worker: registers it
// against the repo's test NATS harness (internal/testutil.FakeNATSServer,
// playing the Control role by hand), dispatches one decoded-HTTP assignment,
// and asserts the static page reaches the Control side of the harness
// (docs/public/architecture.md).

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/beremaran/straw-oss/internal/testutil"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
	sdkegress "github.com/beremaran/straw-sdk-go/egress"
)

const (
	testWorkerID     = "wrk_egress_static_test"
	testSessionID    = "sess_egress_static_test"
	testDeploymentID = "ten_egress_static_test"
	testStatus       = uint32(200)
)

func TestStaticExecutorServesOneAssignment(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeNATSServer(t, 2_000_000)

	controlConn := mustConnect(t, srv.URL())
	workerConn := mustConnect(t, srv.URL())

	registerAndHeartbeatOnControl(t, controlConn)

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	testWorkerRef := "egress-static-test-ref"
	id := sdkegress.Identity{WorkerID: testWorkerID, CredentialID: testWorkerRef, ExecutorType: executorType, PrivateKey: priv}
	caps := sdkegress.Capabilities{MaxConcurrency: 4, SoftwareVersion: "egress-static-example-test"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionID, err := sdkegress.Register(ctx, workerConn, id, caps)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if sessionID != testSessionID {
		t.Fatalf("sessionID = %q, want %q", sessionID, testSessionID)
	}

	executor := &staticExecutor{status: testStatus, body: []byte("hello-from-egress-static\n")}

	worker, err := sdkegress.NewWorker(sdkegress.WorkerOptions{Conn: workerConn, Identity: id, Executor: executor, SessionID: sessionID, MaxConcurrency: caps.MaxConcurrency})
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}

	stop := make(chan struct{})
	serveDone := make(chan error, 1)

	go func() { serveDone <- worker.Serve(stop) }()

	t.Cleanup(func() {
		close(stop)

		select {
		case err := <-serveDone:
			if err != nil {
				t.Errorf("Worker.Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Worker.Serve did not return after stop")
		}
	})

	assignSubject, err := sdkegress.AssignmentSubject(testWorkerID, sessionID)
	if err != nil {
		t.Fatalf("AssignmentSubject: %v", err)
	}

	frames := runAssignment(t, controlConn, assignSubject, sessionID, "req_egress_static_test")

	if got := frames[0].GetResponseStart().GetStatus(); got != testStatus {
		t.Fatalf("first frame ResponseStart status = %d, want %d", got, testStatus)
	}

	if got := string(frames[1].GetData().GetData()); got != "hello-from-egress-static\n" {
		t.Fatalf("second frame data = %q, want static body", got)
	}

	if frames[2].GetEnd() == nil || !frames[2].GetEnd().GetSuccess() {
		t.Fatalf("third frame = %#v, want a successful EndFrame", frames[2])
	}
}

func mustConnect(t *testing.T, url string) *nats.Conn {
	t.Helper()

	conn, err := nats.Connect(url, nats.MaxReconnects(0))
	if err != nil {
		t.Fatalf("connect fake NATS: %v", err)
	}

	t.Cleanup(conn.Close)

	return conn
}

// registerAndHeartbeatOnControl plays the Control side of registration and
// heartbeat: subscribe, validate the subject, and reply with a fixed session.
func registerAndHeartbeatOnControl(t *testing.T, controlConn *nats.Conn) {
	t.Helper()

	_, err := controlConn.Subscribe(sdkegress.RegistrationSubject(), func(msg *nats.Msg) {
		reply := &strawpb.Envelope{ProtocolMajor: sdkegress.ProtocolMajor, Payload: &strawpb.Envelope_RegisterAck{
			RegisterAck: &strawpb.RegisterAck{Ok: true, SessionId: testSessionID},
		}}

		raw, marshalErr := sdkegress.MarshalEnvelope(reply)
		if marshalErr != nil {
			return
		}

		_ = msg.Respond(raw)
	})
	if err != nil {
		t.Fatalf("subscribe registration: %v", err)
	}

	_, err = controlConn.Subscribe(sdkegress.HeartbeatSubject(), func(msg *nats.Msg) {
		reply := &strawpb.Envelope{ProtocolMajor: sdkegress.ProtocolMajor, Payload: &strawpb.Envelope_HeartbeatAck{
			HeartbeatAck: &strawpb.HeartbeatAck{Ok: true},
		}}

		raw, marshalErr := sdkegress.MarshalEnvelope(reply)
		if marshalErr != nil {
			return
		}

		_ = msg.Respond(raw)
	})
	if err != nil {
		t.Fatalf("subscribe heartbeat: %v", err)
	}

	err = controlConn.Flush()
	if err != nil {
		t.Fatalf("flush control subscriptions: %v", err)
	}
}

// runAssignment plays the Control side of one decoded-HTTP assignment: it
// subscribes to e2c before assigning (per docs/public/architecture.md
// subscription ordering), sends AssignRequest, waits for ACCEPTED, publishes
// RequestStart, and collects the response StreamFrames.
func runAssignment(t *testing.T, controlConn *nats.Conn, assignSubject, sessionID, requestID string) []*strawpb.StreamFrame {
	t.Helper()

	e2cSubject, err := sdkegress.StreamSubject(requestID, testWorkerID, sessionID, sdkegress.DirectionExecutorToControl)
	if err != nil {
		t.Fatalf("e2c StreamSubject: %v", err)
	}

	c2eSubject, err := sdkegress.StreamSubject(requestID, testWorkerID, sessionID, sdkegress.DirectionControlToExecutor)
	if err != nil {
		t.Fatalf("c2e StreamSubject: %v", err)
	}

	received := make(chan *strawpb.StreamFrame, 8)

	e2cSub, err := controlConn.Subscribe(e2cSubject, func(msg *nats.Msg) {
		env, decodeErr := sdkegress.UnmarshalEnvelope(msg.Data)
		if decodeErr != nil {
			return
		}

		received <- env.GetStreamFrame()
	})
	if err != nil {
		t.Fatalf("subscribe e2c: %v", err)
	}

	t.Cleanup(func() { _ = e2cSub.Unsubscribe() })

	err = controlConn.Flush()
	if err != nil {
		t.Fatalf("flush e2c subscription: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)

	assign := &strawpb.AssignRequest{
		Mode:                       strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		DeadlineUnixMs:             deadline.UnixMilli(),
		Attempt:                    1,
		InitialUploadCreditBytes:   1 << 20,
		InitialDownloadCreditBytes: 1 << 20,
	}

	assignEnv := &strawpb.Envelope{
		RequestId:      requestID,
		DeploymentId:   testDeploymentID,
		DeadlineUnixMs: deadline.UnixMilli(),
		ProtocolMajor:  sdkegress.ProtocolMajor,
		Attempt:        1,
		Payload:        &strawpb.Envelope_AssignRequest{AssignRequest: assign},
	}

	assignRaw, err := sdkegress.MarshalEnvelope(assignEnv)
	if err != nil {
		t.Fatalf("marshal AssignRequest: %v", err)
	}

	ack := waitForAssignAck(t, controlConn, assignSubject, assignRaw)
	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("AssignAck code = %v, want ACCEPTED", ack.GetCode())
	}

	start := &strawpb.StreamFrame{
		StreamSeq: 1,
		Attempt:   1,
		Payload: &strawpb.StreamFrame_RequestStart{RequestStart: &strawpb.RequestStart{
			Mode:              strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
			Method:            "GET",
			Url:               "http://example.invalid/egress-static-test",
			DeadlineUnixMs:    deadline.UnixMilli(),
			RedirectPolicy:    strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
			DestinationPolicy: &strawpb.DestinationPolicy{ResolutionMode: strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL},
		}},
	}

	startEnv := &strawpb.Envelope{
		RequestId:      requestID,
		DeploymentId:   testDeploymentID,
		DeadlineUnixMs: deadline.UnixMilli(),
		ProtocolMajor:  sdkegress.ProtocolMajor,
		Attempt:        1,
		Payload:        &strawpb.Envelope_StreamFrame{StreamFrame: start},
	}

	startRaw, err := sdkegress.MarshalEnvelope(startEnv)
	if err != nil {
		t.Fatalf("marshal RequestStart: %v", err)
	}

	err = controlConn.Publish(c2eSubject, startRaw)
	if err != nil {
		t.Fatalf("publish RequestStart: %v", err)
	}

	return collectUntilTerminal(t, received)
}

// waitForAssignAck retries the exact-session assign request until the
// executor's assignment subscription is live; the fake broker delivers
// reliably, so a request only fails while Worker.Serve hasn't subscribed yet.
func waitForAssignAck(t *testing.T, controlConn *nats.Conn, subject string, raw []byte) *strawpb.AssignAck {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := controlConn.Request(subject, raw, 100*time.Millisecond)
		if err == nil {
			env, decodeErr := sdkegress.UnmarshalEnvelope(msg.Data)
			if decodeErr != nil {
				t.Fatalf("unmarshal AssignAck: %v", decodeErr)
			}

			ack := env.GetAssignAck()
			if ack == nil {
				t.Fatalf("assign reply carried no AssignAck: %#v", env)
			}

			return ack
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("assign request to %s never got a response", subject)

	return nil
}

func collectUntilTerminal(t *testing.T, received <-chan *strawpb.StreamFrame) []*strawpb.StreamFrame {
	t.Helper()

	var frames []*strawpb.StreamFrame

	deadline := time.After(5 * time.Second)

	for {
		select {
		case frame := <-received:
			frames = append(frames, frame)

			switch frame.GetPayload().(type) {
			case *strawpb.StreamFrame_End, *strawpb.StreamFrame_Error, *strawpb.StreamFrame_Cancelled:
				return frames
			}
		case <-deadline:
			t.Fatalf("timed out waiting for terminal frame, got %d frames", len(frames))

			return frames
		}
	}
}
