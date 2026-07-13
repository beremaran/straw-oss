package egress

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/beremaran/straw-oss/internal/fingerprint"
	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
	sdkegress "github.com/beremaran/straw-sdk-go/egress"
)

// ProtocolMajor is the worker protocol major version this worker speaks.
const ProtocolMajor = sdkegress.ProtocolMajor

// Identity holds the stable identity a worker registers with.
type Identity = sdkegress.Identity

// Capabilities are the capability claims a worker advertises at registration.
type Capabilities = sdkegress.Capabilities

// Capacity describes the executor's current admission state when an AssignRequest arrives.
type Capacity = sdkegress.Capacity

const chrome120FingerprintProfile = "chrome_120"

// SupportedFingerprintProfiles returns the exact named profiles executable by
// the official Egress worker.
func SupportedFingerprintProfiles() []string {
	return fingerprint.Names()
}

// BuildRegisterRequest assembles and signs a RegisterRequest for the worker.
func BuildRegisterRequest(id Identity, caps Capabilities) (*strawpb.RegisterRequest, error) {
	req, err := sdkegress.BuildRegisterRequest(id, caps)
	if err != nil {
		return nil, fmt.Errorf("sdk build register request: %w", err)
	}

	return req, nil
}

// BuildHeartbeat assembles a HeartbeatRequest for the given active session.
func BuildHeartbeat(id Identity, sessionID string, health strawpb.WorkerHealth, activeRequests, availableCapacity, maxConcurrency uint32, draining bool) *strawpb.HeartbeatRequest {
	return sdkegress.BuildHeartbeat(id, sessionID, health, activeRequests, availableCapacity, maxConcurrency, draining)
}

// EvaluateAssignment implements the executor-side admission decision for an AssignRequest.
func EvaluateAssignment(req *strawpb.AssignRequest, capacity Capacity) strawpb.AssignAckCode {
	return sdkegress.EvaluateAssignment(req, capacity)
}

// Register sends a worker registration request over NATS and returns the Control-assigned session id.
func Register(ctx context.Context, conn *natsx.Connection, id Identity, caps Capabilities) (string, error) {
	sessionID, err := sdkegress.Register(ctx, conn, id, caps)
	if err != nil {
		return "", fmt.Errorf("sdk register: %w", err)
	}

	return sessionID, nil
}

// Heartbeat sends a worker heartbeat request over NATS.
func Heartbeat(ctx context.Context, conn *natsx.Connection, id Identity, sessionID string, health strawpb.WorkerHealth, activeRequests, availableCapacity, maxConcurrency uint32, draining bool) error {
	err := sdkegress.Heartbeat(ctx, conn, id, sessionID, health, activeRequests, availableCapacity, maxConcurrency, draining)
	if err != nil {
		return fmt.Errorf("sdk heartbeat: %w", err)
	}

	return nil
}

// Run delegates the session and decoded assignment runtime to sdk/egress.
func Run(ctx context.Context, conn *natsx.Connection, id Identity, caps Capabilities, executor *Executor, heartbeatInterval time.Duration, ready *atomic.Bool) error {
	err := sdkegress.Run(ctx, conn, id, caps, heartbeatInterval, ready, func(sessionID string, maxConcurrency uint32) (sdkegress.AssignmentServer, error) {
		return sdkegress.NewWorker(sdkegress.WorkerOptions{
			Conn:           conn,
			Identity:       sdkegress.Identity(id),
			Executor:       executor,
			BodyRefs:       bodyRefAdapter{executor: executor},
			Tunnels:        tunnelAdapter{executor: executor},
			SessionID:      sessionID,
			MaxConcurrency: maxConcurrency,
			SupportedModes: []strawpb.RequestMode{strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP, strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL},
		})
	})
	if err != nil {
		return fmt.Errorf("sdk run: %w", err)
	}

	return nil
}

type bodyRefAdapter struct {
	executor *Executor
}

func (a bodyRefAdapter) DownloadBodyRef(ctx context.Context, frame *strawpb.BodyRefFrame) ([]byte, *strawpb.ErrorFrame) {
	body, failure := a.executor.downloadBodyRef(ctx, frame)
	if failure == nil {
		return body, nil
	}

	details := map[string]string{errorFactDetailKey: failure.fact}
	if failure.timeoutType != strawpb.TimeoutType_TIMEOUT_TYPE_UNSPECIFIED {
		details["timeout_type"] = failure.timeoutType.String()
	}

	return nil, &strawpb.ErrorFrame{Code: failure.code, Details: details}
}

type tunnelAdapter struct {
	executor *Executor
}

func (a tunnelAdapter) OpenTunnel(ctx context.Context, start *strawpb.RequestStart) (net.Conn, sdkegress.TunnelTarget, *strawpb.ErrorFrame) {
	conn, target, failure := a.executor.openTunnel(ctx, start)
	out := sdkegress.TunnelTarget{Host: target.host, Port: target.port}

	if failure == nil {
		return conn, out, nil
	}

	details := map[string]string{errorFactDetailKey: failure.fact}
	if failure.timeoutType != strawpb.TimeoutType_TIMEOUT_TYPE_UNSPECIFIED {
		details["timeout_type"] = failure.timeoutType.String()
	}

	return nil, out, &strawpb.ErrorFrame{Code: failure.code, Details: details}
}
