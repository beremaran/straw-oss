package control

import (
	"context"
	"strings"
	"testing"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/fingerprint"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const localWorkerID = "local-worker"

func TestDeploymentWorkerRegistryRoutesUnprovisionedWorker(t *testing.T) {
	t.Parallel()

	registry := NewDeploymentWorkerRegistry(DefaultWorkerTimings(), nil)
	outcome, err := registry.Register(context.Background(), &strawpb.RegisterRequest{
		WorkerId: localWorkerID, ExecutorType: errorCategoryEgress, ProtocolMajor: ProtocolMajor,
		ProtocolMinor: 1, MaxConcurrency: 4,
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Register() = %+v, %v; want success", outcome, err)
	}

	ok, err := registry.Heartbeat(context.Background(), &strawpb.HeartbeatRequest{
		WorkerId: localWorkerID, SessionId: outcome.SessionID,
		Health: strawpb.WorkerHealth_WORKER_HEALTH_READY,
	})
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = %v, %v; want success", ok, err)
	}

	candidates := registry.CandidatesForPool("default", "default")
	if len(candidates) != 1 || candidates[0].WorkerID != localWorkerID {
		t.Fatalf("CandidatesForPool() = %+v, want local-worker", candidates)
	}
}

func TestWorkerRegistrationAcceptsOnlyExactCatalogueCapabilities(t *testing.T) {
	t.Parallel()

	registry := NewDeploymentWorkerRegistry(DefaultWorkerTimings(), nil)
	request := func(id string, profiles []string) *strawpb.RegisterRequest {
		return &strawpb.RegisterRequest{
			WorkerId: id, ExecutorType: errorCategoryEgress, ProtocolMajor: ProtocolMajor,
			ProtocolMinor: 1, MaxConcurrency: 4, SupportedFingerprintProfiles: profiles,
		}
	}

	outcome, err := registry.Register(context.Background(), request("complete-catalogue", fingerprint.Names()))
	if err != nil || !outcome.OK {
		t.Fatalf("complete catalogue registration = %+v, %v; want success", outcome, err)
	}

	for _, test := range []struct {
		name     string
		profiles []string
	}{
		{name: "unknown", profiles: []string{"firefox_999"}},
		{name: "duplicate", profiles: []string{fingerprintProfileChrome120, fingerprintProfileChrome120}},
	} {
		outcome, err = registry.Register(context.Background(), request(test.name, test.profiles))
		if err != nil || outcome.OK || outcome.Reason != rejectCapabilityScope {
			t.Errorf("%s registration = %+v, %v; want capability rejection", test.name, outcome, err)
		}
	}
}

func TestWorkerRegistrationValidatesConfiguredPoolMembership(t *testing.T) {
	t.Parallel()

	registry := NewDeploymentWorkerRegistry(DefaultWorkerTimings(), nil)
	snapshot := config.NewSnapshot(1)
	snapshot.ExecutorPools = []config.ExecutorPool{
		{ID: config.DefaultPoolID, ExecutorType: errorCategoryEgress, Enabled: true},
		{ID: routingIPType, ExecutorType: errorCategoryEgress, Enabled: true},
	}
	registry.ApplySnapshot(snapshot)

	request := func(refs ...*strawpb.RegisterRequest_PoolRef) *strawpb.RegisterRequest {
		return &strawpb.RegisterRequest{WorkerId: "pool-worker", ExecutorType: errorCategoryEgress, ProtocolMajor: ProtocolMajor, AllowedPools: refs}
	}

	accepted, err := registry.Register(context.Background(), request(
		&strawpb.RegisterRequest_PoolRef{PoolId: config.DefaultPoolID},
		&strawpb.RegisterRequest_PoolRef{PoolId: routingIPType},
	))
	if err != nil || !accepted.OK {
		t.Fatalf("multi-pool registration = %+v, %v", accepted, err)
	}

	for name, refs := range map[string][]*strawpb.RegisterRequest_PoolRef{
		"unknown":          {{PoolId: "missing"}},
		"duplicate":        {{PoolId: routingIPType}, {PoolId: routingIPType}},
		"wrong deployment": {{DeploymentId: "other", PoolId: routingIPType}},
	} {
		requestCopy := request(refs...)
		requestCopy.WorkerId = "invalid-" + strings.ReplaceAll(name, " ", "-")
		outcome, regErr := registry.Register(context.Background(), requestCopy)
		if regErr != nil || outcome.OK || outcome.Reason != rejectInvalidPool {
			t.Errorf("%s registration = %+v, %v; want %s", name, outcome, regErr, rejectInvalidPool)
		}
	}
}

func TestWorkerRegistrationNegotiatesMinorAndValidatesProxyClaims(t *testing.T) {
	t.Parallel()

	const (
		directPoolID = "direct-pool"
		proxyPoolID  = "proxy-pool"
		proxyID      = "proxy-profile"
	)

	registry := NewDeploymentWorkerRegistry(DefaultWorkerTimings(), nil)
	snapshot := config.NewSnapshot(1)
	snapshot.ExecutorPools = []config.ExecutorPool{
		{ID: directPoolID, ExecutorType: errorCategoryEgress, Enabled: true},
		{ID: proxyPoolID, ExecutorType: errorCategoryEgress, Enabled: true, UpstreamProxy: &config.ExecutorPoolUpstreamProxy{ID: proxyID, TrustedRemoteResolution: true}},
	}
	registry.ApplySnapshot(snapshot)

	tests := []struct {
		name            string
		minor           uint32
		poolID          string
		upstreamProxyID string
		wantOK          bool
		wantReason      string
	}{
		{name: "minor-0-direct", minor: 0, poolID: directPoolID, wantOK: true},
		{name: "minor-1-direct", minor: 1, poolID: directPoolID, wantOK: true},
		{name: "minor-1-proxy", minor: 1, poolID: proxyPoolID, upstreamProxyID: proxyID, wantReason: rejectIncompatibleProto},
		{name: "minor-2-proxy", minor: 2, poolID: proxyPoolID, upstreamProxyID: proxyID, wantOK: true},
		{name: "future-minor", minor: ControlSupportedMinor + 1, poolID: directPoolID, wantReason: rejectIncompatibleProto},
		{name: "direct-with-proxy-claim", minor: 2, poolID: directPoolID, upstreamProxyID: proxyID, wantReason: rejectInvalidPool},
		{name: "proxy-with-empty-claim", minor: 2, poolID: proxyPoolID, wantReason: rejectInvalidPool},
		{name: "proxy-with-wrong-claim", minor: 2, poolID: proxyPoolID, upstreamProxyID: "stale-profile", wantReason: rejectInvalidPool},
	}

	for i, test := range tests {
		request := &strawpb.RegisterRequest{
			WorkerId: "protocol-worker-" + string(rune('a'+i)), ExecutorType: errorCategoryEgress,
			ProtocolMajor: ProtocolMajor, ProtocolMinor: test.minor, MaxConcurrency: 1,
			AllowedPools: []*strawpb.RegisterRequest_PoolRef{{PoolId: test.poolID, UpstreamProxyId: test.upstreamProxyID}},
		}
		outcome, err := registry.Register(context.Background(), request)
		if err != nil || outcome.OK != test.wantOK || outcome.Reason != test.wantReason {
			t.Errorf("%s registration = %+v, %v; want ok=%v reason=%q", test.name, outcome, err, test.wantOK, test.wantReason)
		}
		if !test.wantOK {
			continue
		}

		session := registry.workers[request.WorkerId]
		if session.supportedProtocolMinor != test.minor || session.protocolMinor != min(ControlSupportedMinor, test.minor) {
			t.Errorf("%s session minors = supported %d negotiated %d", test.name, session.supportedProtocolMinor, session.protocolMinor)
		}

		ok, heartbeatErr := registry.Heartbeat(context.Background(), &strawpb.HeartbeatRequest{
			WorkerId: request.WorkerId, SessionId: outcome.SessionID,
			Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, AvailableCapacity: 1,
		})
		if heartbeatErr != nil || !ok {
			t.Fatalf("%s heartbeat = %v, %v", test.name, ok, heartbeatErr)
		}
		var candidate *PoolCandidate
		for _, current := range registry.CandidatesForPool(config.DefaultDeploymentID, test.poolID) {
			if current.WorkerID == request.WorkerId {
				candidate = &current

				break
			}
		}
		if candidate == nil || candidate.UpstreamProxyID != test.upstreamProxyID || candidate.SupportedProtocolMinor != test.minor || candidate.ProtocolMinor != min(ControlSupportedMinor, test.minor) {
			t.Fatalf("%s candidate = %+v", test.name, candidate)
		}
	}
}
