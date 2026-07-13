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
		{name: "duplicate", profiles: []string{"chrome_120", "chrome_120"}},
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
		"wrong deployment": {{DeploymentId: "other", PoolId: "residential"}},
	} {
		requestCopy := request(refs...)
		requestCopy.WorkerId = "invalid-" + strings.ReplaceAll(name, " ", "-")
		outcome, regErr := registry.Register(context.Background(), requestCopy)
		if regErr != nil || outcome.OK || outcome.Reason != rejectInvalidPool {
			t.Errorf("%s registration = %+v, %v; want %s", name, outcome, regErr, rejectInvalidPool)
		}
	}
}
