package egress

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/testutil"
)

func runtimeTestIdentity(t *testing.T, workerID string) Identity {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	return Identity{WorkerID: workerID, CredentialID: "cred_runtime", ExecutorType: "egress", PrivateKey: priv}
}

func respondEnvelope(t *testing.T, msg *nats.Msg, env *strawpb.Envelope) {
	t.Helper()

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		t.Errorf("MarshalEnvelope: %v", err)

		return
	}

	respondErr := msg.Respond(raw)
	if respondErr != nil {
		t.Errorf("Respond: %v", respondErr)
	}
}

func assertDistinctNonces(t *testing.T, nonces [][]byte) {
	t.Helper()

	for i := range nonces {
		if len(nonces[i]) == 0 {
			t.Fatalf("registration attempt %d carried an empty nonce", i+1)
		}

		for j := i + 1; j < len(nonces); j++ {
			if bytes.Equal(nonces[i], nonces[j]) {
				t.Fatalf("registration attempts %d and %d reused the same nonce", i+1, j+1)
			}
		}
	}
}

func TestRegisterWithRetrySucceedsAfterTransientFailures(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	workerConn := loopConnect(t, srv.URL())
	controlConn := loopConnect(t, srv.URL())

	var (
		mu       sync.Mutex
		nonces   [][]byte
		attempts int
	)

	_, err := controlConn.Subscribe(natsx.RegistrationSubject(), func(msg *nats.Msg) {
		env, envErr := natsx.UnmarshalEnvelope(msg.Data)
		if envErr != nil {
			return
		}

		mu.Lock()
		attempts++
		n := attempts
		nonces = append(nonces, append([]byte(nil), env.GetRegisterRequest().GetNonce()...))
		mu.Unlock()

		reply := &strawpb.Envelope{ProtocolMajor: ProtocolMajor}
		if n >= 3 {
			reply.Payload = &strawpb.Envelope_RegisterAck{RegisterAck: &strawpb.RegisterAck{Ok: true, SessionId: "sess_retry"}}
		}

		respondEnvelope(t, msg, reply)
	})
	if err != nil {
		t.Fatalf("subscribe register: %v", err)
	}

	flushErr := controlConn.Flush()
	if flushErr != nil {
		t.Fatalf("flush: %v", flushErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionID, err := registerWithRetry(ctx, workerConn, runtimeTestIdentity(t, "retry_worker"), Capabilities{MaxConcurrency: 1}, time.Millisecond, 4*time.Millisecond)
	if err != nil {
		t.Fatalf("registerWithRetry: %v", err)
	}

	if sessionID != "sess_retry" {
		t.Fatalf("sessionID = %q, want sess_retry", sessionID)
	}

	mu.Lock()
	defer mu.Unlock()

	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	assertDistinctNonces(t, nonces)
}

func TestRegisterWithRetryStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	workerConn := loopConnect(t, srv.URL())
	controlConn := loopConnect(t, srv.URL())

	_, err := controlConn.Subscribe(natsx.RegistrationSubject(), func(msg *nats.Msg) {
		// Reply without a RegisterAck so every attempt fails fast.
		respondEnvelope(t, msg, &strawpb.Envelope{ProtocolMajor: ProtocolMajor})
	})
	if err != nil {
		t.Fatalf("subscribe register: %v", err)
	}

	flushErr := controlConn.Flush()
	if flushErr != nil {
		t.Fatalf("flush: %v", flushErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Backoff far longer than the ctx timeout: the retry loop must abort
	// from inside its backoff wait, not after it.
	_, err = registerWithRetry(ctx, workerConn, runtimeTestIdentity(t, "cancel_worker"), Capabilities{MaxConcurrency: 1}, time.Hour, time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("registerWithRetry error = %v, want context.DeadlineExceeded", err)
	}
}

func TestRunReregistersWhenControlForgetsSession(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeNATSServer(t, 2_000_000)
	workerConn := loopConnect(t, srv.URL())
	controlConn := loopConnect(t, srv.URL())

	var (
		mu             sync.Mutex
		nonces         [][]byte
		registrations  int
		liveHeartbeats int
	)

	_, err := controlConn.Subscribe(natsx.RegistrationSubject(), func(msg *nats.Msg) {
		env, envErr := natsx.UnmarshalEnvelope(msg.Data)
		if envErr != nil {
			return
		}

		mu.Lock()
		registrations++

		sessionID := "sess_dead"
		if registrations > 1 {
			sessionID = "sess_live"
		}

		nonces = append(nonces, append([]byte(nil), env.GetRegisterRequest().GetNonce()...))
		mu.Unlock()

		respondEnvelope(t, msg, &strawpb.Envelope{
			ProtocolMajor: ProtocolMajor,
			Payload:       &strawpb.Envelope_RegisterAck{RegisterAck: &strawpb.RegisterAck{Ok: true, SessionId: sessionID}},
		})
	})
	if err != nil {
		t.Fatalf("subscribe register: %v", err)
	}

	// Control "restarted": the first session is unknown, so its heartbeats
	// are NACKed; the re-registered session heartbeats fine.
	_, err = controlConn.Subscribe(natsx.HeartbeatSubject(), func(msg *nats.Msg) {
		env, envErr := natsx.UnmarshalEnvelope(msg.Data)
		if envErr != nil {
			return
		}

		ack := &strawpb.HeartbeatAck{Ok: false, Error: "unknown_worker_session"}
		if env.GetHeartbeatRequest().GetSessionId() == "sess_live" {
			ack = &strawpb.HeartbeatAck{Ok: true}

			mu.Lock()
			liveHeartbeats++
			mu.Unlock()
		}

		respondEnvelope(t, msg, &strawpb.Envelope{
			ProtocolMajor: ProtocolMajor,
			Payload:       &strawpb.Envelope_HeartbeatAck{HeartbeatAck: ack},
		})
	})
	if err != nil {
		t.Fatalf("subscribe heartbeat: %v", err)
	}

	flushErr := controlConn.Flush()
	if flushErr != nil {
		t.Fatalf("flush: %v", flushErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := &atomic.Bool{}
	runDone := make(chan error, 1)

	go func() {
		runDone <- Run(ctx, workerConn, runtimeTestIdentity(t, "forget_worker"), Capabilities{MaxConcurrency: 1}, NewExecutor(ExecutorOptions{}), 20*time.Millisecond, ready)
	}()

	waitForCondition(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return registrations >= 2 && liveHeartbeats >= 1 && ready.Load()
	})

	cancel()

	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Fatalf("Run returned error = %v", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}

	if ready.Load() {
		t.Fatal("ready still true after Run returned")
	}

	mu.Lock()
	defer mu.Unlock()

	assertDistinctNonces(t, nonces)
}
