package egress

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

// registrationNonceBytes is the size of the random nonce bound into every
// signed registration token (docs/planning/27 "Worker Credential Signing").
const registrationNonceBytes = 16

// ProtocolMajor is the worker protocol major version this worker speaks. It
// must match the Control-side constant for registration to succeed.
const ProtocolMajor uint32 = 1

// Identity holds the stable identity a worker registers with.
type Identity struct {
	WorkerID     string
	CredentialID string
	ExecutorType string
	PrivateKey   ed25519.PrivateKey
}

// Capabilities are the capability claims a worker advertises at registration.
// They must be within the worker credential's allowed scope or Control rejects
// the registration.
type Capabilities struct {
	AllowedPools          []*strawpb.RegisterRequest_PoolRef
	Tags                  []string
	Countries             []string
	Regions               []string
	IPTypes               []string
	SupportedIngressModes []string
	MaxConcurrency        uint32
	SoftwareVersion       string
	InitialDraining       bool
}

// BuildRegisterRequest assembles and signs a RegisterRequest for the worker.
// The returned message carries the ed25519 signature over its canonical
// payload in SignedToken, binding a fresh crypto/rand nonce and the current
// time as issued-at so Control can reject replays and stale requests
// (docs/planning/27-security-controls.md).
func BuildRegisterRequest(id Identity, caps Capabilities) (*strawpb.RegisterRequest, error) {
	pools := make([]*strawpb.RegisterRequest_PoolRef, 0, len(caps.AllowedPools))
	for _, p := range caps.AllowedPools {
		pools = append(pools, &strawpb.RegisterRequest_PoolRef{TenantId: p.GetTenantId(), PoolId: p.GetPoolId()})
	}

	nonce := make([]byte, registrationNonceBytes)

	_, err := rand.Read(nonce)
	if err != nil {
		return nil, fmt.Errorf("generate registration nonce: %w", err)
	}

	req := &strawpb.RegisterRequest{
		WorkerId:              id.WorkerID,
		ExecutorType:          id.ExecutorType,
		CredentialId:          id.CredentialID,
		ProtocolMajor:         ProtocolMajor,
		ProtocolMinor:         0,
		SoftwareVersion:       caps.SoftwareVersion,
		AllowedPools:          pools,
		Tags:                  caps.Tags,
		Countries:             caps.Countries,
		Regions:               caps.Regions,
		IpTypes:               caps.IPTypes,
		SupportedIngressModes: caps.SupportedIngressModes,
		MaxConcurrency:        caps.MaxConcurrency,
		InitialDraining:       caps.InitialDraining,
		Nonce:                 nonce,
		IssuedAtUnixMs:        time.Now().UnixMilli(),
	}
	req.SignedToken = strawpb.SignRegistration(id.PrivateKey, req)

	return req, nil
}

// InboxPrefix returns the scoped reply-inbox prefix this worker must configure
// on its NATS request/reply client (`_INBOX.wrk.<worker_id>`), per the ACL
// table in docs/planning/12-nats-protocol.md.
func (id Identity) InboxPrefix() (string, error) {
	prefix, err := natsx.WorkerInboxPrefix(id.WorkerID)
	if err != nil {
		return "", fmt.Errorf("worker inbox prefix: %w", err)
	}

	return prefix, nil
}

// BuildHeartbeat assembles a HeartbeatRequest for the given active session.
func BuildHeartbeat(id Identity, sessionID string, health strawpb.WorkerHealth, activeRequests, availableCapacity, maxConcurrency uint32, draining bool) *strawpb.HeartbeatRequest {
	return &strawpb.HeartbeatRequest{
		WorkerId:          id.WorkerID,
		SessionId:         sessionID,
		Health:            health,
		ActiveRequests:    activeRequests,
		AvailableCapacity: availableCapacity,
		MaxConcurrency:    maxConcurrency,
		Draining:          draining,
	}
}
