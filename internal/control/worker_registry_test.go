package control

import (
	"context"
	"testing"

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
