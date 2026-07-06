package egress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const (
	defaultHeartbeatInterval = 5 * time.Second
	registerTimeout          = 5 * time.Second
	heartbeatTimeout         = 5 * time.Second
	registerBackoffFloor     = 1 * time.Second
	registerBackoffMax       = 30 * time.Second
	registerBackoffFactor    = 2
)

// Register sends a worker registration request over NATS and returns the
// Control-assigned session id.
func Register(ctx context.Context, conn *natsx.Connection, id Identity, caps Capabilities) (string, error) {
	req, err := BuildRegisterRequest(id, caps)
	if err != nil {
		return "", fmt.Errorf("build register request: %w", err)
	}

	env := &strawpb.Envelope{
		ProtocolMajor: ProtocolMajor,
		ProtocolMinor: 0,
		Payload:       &strawpb.Envelope_RegisterRequest{RegisterRequest: req},
	}

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		return "", fmt.Errorf("marshal envelope: %w", err)
	}

	reply, err := request(ctx, conn, natsx.RegistrationSubject(), raw, registerTimeout)
	if err != nil {
		return "", err
	}

	resp, err := natsx.UnmarshalEnvelope(reply)
	if err != nil {
		return "", fmt.Errorf("unmarshal envelope: %w", err)
	}

	ack := resp.GetRegisterAck()

	var sessionID string

	if ack == nil {
		return "", errRegisterAckMissing
	}

	if !ack.GetOk() {
		return "", fmt.Errorf("registration rejected: %w", errRegistrationRejected)
	}

	sessionID = ack.GetSessionId()
	if sessionID == "" {
		return "", errRegistrationNoSession
	}

	return sessionID, nil
}

// Heartbeat sends a worker heartbeat request over NATS.
func Heartbeat(ctx context.Context, conn *natsx.Connection, id Identity, sessionID string, health strawpb.WorkerHealth, activeRequests, availableCapacity, maxConcurrency uint32, draining bool) error {
	hb := BuildHeartbeat(id, sessionID, health, activeRequests, availableCapacity, maxConcurrency, draining)
	hb.WorkerTimestampMs = time.Now().UTC().UnixMilli()

	env := &strawpb.Envelope{
		ProtocolMajor: ProtocolMajor,
		ProtocolMinor: 0,
		Payload:       &strawpb.Envelope_HeartbeatRequest{HeartbeatRequest: hb},
	}

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	reply, err := request(ctx, conn, natsx.HeartbeatSubject(), raw, heartbeatTimeout)
	if err != nil {
		return err
	}

	resp, err := natsx.UnmarshalEnvelope(reply)
	if err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	ack := resp.GetHeartbeatAck()

	if ack == nil {
		return errHeartbeatAckMissing
	}

	if !ack.GetOk() {
		return fmt.Errorf("heartbeat rejected: %w", errHeartbeatRejected)
	}

	return nil
}

// Run keeps the worker registered and heartbeating, and runs the live
// assignment execution loop, until ctx is canceled. Registration retries
// with bounded backoff while Control is unreachable, and a heartbeat NACK
// (e.g. a Control restart wiping its in-memory registry) drains the dead
// session and re-registers. On shutdown it sends a draining heartbeat before
// telling the assignment loop to stop accepting new work and drain in-flight
// requests (docs/planning/29 "Worker Graceful Shutdown"). If ready is
// non-nil, it is set true once registration succeeds and false again once
// the session is lost or draining begins, so an egress /readyz endpoint
// (docs/planning/23) can reflect the run loop's live state.
func Run(ctx context.Context, conn *natsx.Connection, id Identity, caps Capabilities, executor *Executor, heartbeatInterval time.Duration, ready *atomic.Bool) error {
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultHeartbeatInterval
	}

	setReady(ready, false)

	for {
		sessionID, err := registerWithRetry(ctx, conn, id, caps, registerBackoffFloor, registerBackoffMax)
		if err != nil {
			return err
		}

		setReady(ready, true)

		sessionLost, err := runSession(ctx, conn, id, caps, executor, sessionID, heartbeatInterval, ready)

		setReady(ready, false)

		if err != nil {
			return err
		}

		if !sessionLost {
			return nil
		}

		slog.Warn("control rejected worker session, re-registering", "worker_id", id.WorkerID, "session_id", sessionID)
	}
}

// registerWithRetry retries Register with bounded exponential backoff until
// it succeeds or ctx is canceled. Every attempt rebuilds and re-signs the
// request, so retries carry fresh nonces and issued-at timestamps and pass
// Control's replay protection.
// ponytail: retries every failure (including rejections) and has no jitter —
// P0 runs a handful of workers; classify permanent rejections as fatal and
// add jitter if a fleet ever makes retry storms matter.
func registerWithRetry(ctx context.Context, conn *natsx.Connection, id Identity, caps Capabilities, backoffMin, backoffMax time.Duration) (string, error) {
	backoff := backoffMin

	for {
		sessionID, err := Register(ctx, conn, id, caps)
		if err == nil {
			return sessionID, nil
		}

		if ctx.Err() != nil {
			return "", fmt.Errorf("register: %w", ctx.Err())
		}

		slog.Warn("registration failed, retrying", "worker_id", id.WorkerID, "backoff", backoff.String(), "error", err)

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("register: %w", ctx.Err())
		case <-time.After(backoff):
		}

		backoff = min(backoff*registerBackoffFactor, backoffMax)
	}
}

// runSession serves one registered session until ctx is canceled (graceful
// shutdown, returns false) or Control rejects a heartbeat for it (returns
// true so Run re-registers). Either way the assignment loop stops accepting
// new work and drains in-flight requests before returning.
func runSession(ctx context.Context, conn *natsx.Connection, id Identity, caps Capabilities, executor *Executor, sessionID string, heartbeatInterval time.Duration, ready *atomic.Bool) (bool, error) {
	worker, err := NewWorker(conn, id, executor, sessionID, caps.MaxConcurrency)
	if err != nil {
		return false, err
	}

	stop := make(chan struct{})
	stopServing := sync.OnceFunc(func() { close(stop) })

	serveDone := make(chan error, 1)

	go func() { serveDone <- worker.Serve(stop) }()

	defer func() {
		stopServing()
		<-serveDone
	}()

	return runHeartbeatLoop(ctx, conn, id, sessionID, caps, worker, heartbeatInterval, ready), nil
}

// runHeartbeatLoop sends an immediate heartbeat and then periodic ones until
// ctx is canceled, then sends a final draining heartbeat (docs/planning/29
// "Worker Graceful Shutdown" step 1) and clears ready. It returns true as
// soon as Control rejects a heartbeat, meaning the session is no longer
// recognized and the worker must re-register; transport errors are ignored
// and the next tick retries.
func runHeartbeatLoop(ctx context.Context, conn *natsx.Connection, id Identity, sessionID string, caps Capabilities, worker *Worker, heartbeatInterval time.Duration, ready *atomic.Bool) bool {
	sendHeartbeat := func(hbCtx context.Context, draining bool) error {
		active := worker.ActiveRequests()

		return Heartbeat(hbCtx, conn, id, sessionID, strawpb.WorkerHealth_WORKER_HEALTH_READY, active, capacityFromConcurrency(active, caps.MaxConcurrency), caps.MaxConcurrency, draining)
	}

	if errors.Is(sendHeartbeat(ctx, false), errHeartbeatRejected) {
		return true
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			setReady(ready, false)

			_ = sendHeartbeat(context.WithoutCancel(ctx), true)

			return false
		case <-ticker.C:
			if errors.Is(sendHeartbeat(ctx, false), errHeartbeatRejected) {
				return true
			}
		}
	}
}

// setReady stores v in ready if ready is non-nil.
func setReady(ready *atomic.Bool, v bool) {
	if ready != nil {
		ready.Store(v)
	}
}

func request(ctx context.Context, conn *natsx.Connection, subject string, raw []byte, timeout time.Duration) ([]byte, error) {
	if ctx != nil {
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return nil, fmt.Errorf("context error: %w", ctxErr)
		}
	}

	if conn == nil {
		return nil, errConnRequired
	}

	msg, err := conn.Request(subject, raw, timeout)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", subject, err)
	}

	return msg.Data, nil
}

func capacityFromConcurrency(activeRequests, maxConcurrency uint32) uint32 {
	if maxConcurrency == 0 {
		return 0
	}

	if activeRequests >= maxConcurrency {
		return 0
	}

	return maxConcurrency - activeRequests
}

// Error values for registration and heartbeat.
var (
	errRegisterAckMissing    = errors.New("register ack missing")
	errRegistrationRejected  = errors.New("registration rejected")
	errRegistrationNoSession = errors.New("registration accepted without session id")
	errHeartbeatAckMissing   = errors.New("heartbeat ack missing")
	errHeartbeatRejected     = errors.New("heartbeat rejected")
	errConnRequired          = errors.New("nats connection is required")
)
