package control

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
	"github.com/beremaran/straw-oss/internal/natsx"
)

const workerNATSInvalidMessage = "invalid_nats_message"

var (
	errSetupConnRequired = errors.New("nats connection is required")
	errSetupRegRequired  = errors.New("worker registry is required")
)

// SetupWorkerDiscoverySubscriptions wires the live NATS worker discovery
// subjects into the registry and flushes the subscriptions before returning.
func SetupWorkerDiscoverySubscriptions(ctx context.Context, conn *natsx.Connection, reg *WorkerRegistry) error {
	if conn == nil {
		return fmt.Errorf("setup worker discovery: %w", errSetupConnRequired)
	}

	if reg == nil {
		return fmt.Errorf("setup worker discovery: %w", errSetupRegRequired)
	}

	var err error

	_, err = conn.QueueSubscribe(natsx.RegistrationSubject(), "control", func(msg *nats.Msg) {
		replyWorkerRegister(ctx, reg, msg)
	})
	if err != nil {
		return fmt.Errorf("subscribe register: %w", err)
	}

	_, err = conn.QueueSubscribe(natsx.HeartbeatSubject(), "control", func(msg *nats.Msg) {
		replyWorkerHeartbeat(ctx, reg, msg)
	})
	if err != nil {
		return fmt.Errorf("subscribe heartbeat: %w", err)
	}

	err = conn.Flush()
	if err != nil {
		return fmt.Errorf("flush worker discovery subscriptions: %w", err)
	}

	return nil
}

func replyWorkerRegister(ctx context.Context, reg *WorkerRegistry, msg *nats.Msg) {
	ack := &strawpb.RegisterAck{Ok: false, Error: workerNATSInvalidMessage}

	env, err := natsx.UnmarshalEnvelope(msg.Data)
	if err == nil {
		handleRegister(ctx, reg, ack, env)
	}

	reply := buildReplyEnvelope(env, &strawpb.Envelope{
		ProtocolMajor: ProtocolMajor,
		ProtocolMinor: 0,
		Payload:       &strawpb.Envelope_RegisterAck{RegisterAck: ack},
	})

	replyEnvelope(msg, reply)
}

func handleRegister(ctx context.Context, reg *WorkerRegistry, ack *strawpb.RegisterAck, env *strawpb.Envelope) {
	req := env.GetRegisterRequest()
	if req == nil {
		return
	}

	outcome, regErr := reg.Register(ctx, req)
	if regErr != nil {
		ack.Error = errorCodeControlInternalError

		return
	}

	if outcome.OK {
		ack.Ok = true
		ack.SessionId = outcome.SessionID
		ack.Error = ""
	} else {
		ack.Error = outcome.Reason
	}
}

func replyWorkerHeartbeat(ctx context.Context, reg *WorkerRegistry, msg *nats.Msg) {
	ack := &strawpb.HeartbeatAck{Ok: false, Error: workerNATSInvalidMessage}

	env, err := natsx.UnmarshalEnvelope(msg.Data)
	if err == nil {
		handleHeartbeat(ctx, reg, ack, env)
	}

	reply := buildReplyEnvelope(env, &strawpb.Envelope{
		ProtocolMajor: ProtocolMajor,
		ProtocolMinor: 0,
		Payload:       &strawpb.Envelope_HeartbeatAck{HeartbeatAck: ack},
	})

	replyEnvelope(msg, reply)
}

func handleHeartbeat(ctx context.Context, reg *WorkerRegistry, ack *strawpb.HeartbeatAck, env *strawpb.Envelope) {
	req := env.GetHeartbeatRequest()
	if req == nil {
		return
	}

	ok, regErr := reg.Heartbeat(ctx, req)
	if regErr != nil {
		ack.Error = errorCodeControlInternalError

		return
	}

	if ok {
		ack.Ok = true
		ack.Error = ""
	} else {
		ack.Error = "unknown_worker_session"
	}
}

func buildReplyEnvelope(env *strawpb.Envelope, reply *strawpb.Envelope) *strawpb.Envelope {
	if env == nil {
		return reply
	}

	reply.RequestId = env.GetRequestId()
	reply.TenantId = env.GetTenantId()
	reply.TraceId = env.GetTraceId()
	reply.DeadlineUnixMs = env.GetDeadlineUnixMs()
	reply.Attempt = env.GetAttempt()

	reply.TraceContext = append([]byte(nil), env.GetTraceContext()...)
	if env.GetProtocolMinor() != 0 {
		reply.ProtocolMinor = env.GetProtocolMinor()
	}

	return reply
}

func replyEnvelope(msg *nats.Msg, env *strawpb.Envelope) {
	var err error

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		return
	}

	err = msg.Respond(raw)
	if err != nil {
		return
	}
}
