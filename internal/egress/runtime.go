package egress

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const (
	defaultHeartbeatInterval = 5 * time.Second
	registerTimeout          = 5 * time.Second
	heartbeatTimeout         = 5 * time.Second
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
// assignment execution loop, until ctx is canceled. On shutdown it sends a
// draining heartbeat before telling the assignment loop to stop accepting
// new work and drain in-flight requests (docs/planning/29 "Worker Graceful
// Shutdown").
func Run(ctx context.Context, conn *natsx.Connection, id Identity, caps Capabilities, executor *Executor, heartbeatInterval time.Duration) error {
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultHeartbeatInterval
	}

	sessionID, err := Register(ctx, conn, id, caps)
	if err != nil {
		return err
	}

	worker, err := NewWorker(conn, id, executor, sessionID, caps.MaxConcurrency)
	if err != nil {
		return err
	}

	stop := make(chan struct{})

	stopServing := sync.OnceFunc(func() { close(stop) })
	defer stopServing()

	serveDone := make(chan error, 1)

	go func() { serveDone <- worker.Serve(stop) }()

	active := worker.ActiveRequests()

	err = Heartbeat(ctx, conn, id, sessionID, strawpb.WorkerHealth_WORKER_HEALTH_READY, active, capacityFromConcurrency(active, caps.MaxConcurrency), caps.MaxConcurrency, false)
	if err != nil {
		stopServing()
		<-serveDone

		return err
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			active = worker.ActiveRequests()
			_ = Heartbeat(context.WithoutCancel(ctx), conn, id, sessionID, strawpb.WorkerHealth_WORKER_HEALTH_READY, active, capacityFromConcurrency(active, caps.MaxConcurrency), caps.MaxConcurrency, true)

			stopServing()
			<-serveDone

			return nil
		case <-ticker.C:
			active = worker.ActiveRequests()
			_ = Heartbeat(ctx, conn, id, sessionID, strawpb.WorkerHealth_WORKER_HEALTH_READY, active, capacityFromConcurrency(active, caps.MaxConcurrency), caps.MaxConcurrency, false)
		}
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
