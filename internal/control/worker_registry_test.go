package control

import (
	"context"
	"testing"

	strawpb "github.com/beremaran/straw-oss/v2/api/proto/straw/v1"
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
