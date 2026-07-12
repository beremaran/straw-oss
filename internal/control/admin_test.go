package control

import (
	"context"
	"errors"
	"testing"

	strawpb "github.com/beremaran/straw-oss/v2/api/proto/straw/v1"
	"github.com/beremaran/straw-oss/v2/internal/config"
)

func adminTestSnapshot() config.Snapshot {
	s := config.NewSnapshot(1)
	s.ExecutorPools = []config.ExecutorPool{{ID: config.DefaultPoolID, ExecutorType: errorCategoryEgress, Enabled: true}}
	s.RoutingRules = []config.RoutingRule{{ID: "default", Enabled: true, TargetPoolID: config.DefaultPoolID}}

	return s
}

type testAdminFixture struct {
	admin    *AdminService
	store    *MemoryConfigStore
	workers  *WorkerRegistry
	inflight *InFlightRegistry
}

func newTestAdmin(t *testing.T) testAdminFixture {
	t.Helper()
	snapshot := adminTestSnapshot()
	store := NewMemoryConfigStore(snapshot)
	workers := NewDeploymentWorkerRegistry(DefaultWorkerTimings(), nil)
	inflight := NewInFlightRegistry()
	service, err := NewAdminService(store, NewConfigCache(snapshot), workers, inflight, nil)
	if err != nil {
		t.Fatal(err)
	}

	return testAdminFixture{admin: service, store: store, workers: workers, inflight: inflight}
}

func TestAdminUpdateUsesOptimisticConcurrencyAndKeepsHistory(t *testing.T) {
	t.Parallel()
	admin := newTestAdmin(t).admin
	current, _ := admin.Current()
	next := current.Snapshot.Clone()
	next.MaxTimeoutMs = 120_000
	updated, err := admin.Update(current.Revision, next, "operator@example", "update")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Snapshot.ConfigVersion != 2 || updated.Snapshot.MaxTimeoutMs != 120_000 {
		t.Fatalf("updated = %+v", updated)
	}
	_, err = admin.Update(current.Revision, next, "other", "update")
	if !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	history, _ := admin.History()
	if len(history) != 2 || history[0].Actor != "operator@example" {
		t.Fatalf("history = %+v", history)
	}
}

func TestAdminRollbackCreatesNewVersion(t *testing.T) {
	t.Parallel()
	admin := newTestAdmin(t).admin
	current, _ := admin.Current()
	next := current.Snapshot.Clone()
	next.MaxTimeoutMs = 10_000
	next.DefaultTimeoutMs = 5_000
	updated, err := admin.Update(current.Revision, next, "one", "update")
	if err != nil {
		t.Fatal(err)
	}
	rolled, err := admin.Rollback(updated.Revision, 1, "two")
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Snapshot.ConfigVersion != 3 || rolled.Snapshot.MaxTimeoutMs != current.Snapshot.MaxTimeoutMs {
		t.Fatalf("rollback = %+v", rolled)
	}
}

func TestWorkerLifecycleStopsNewRoutingWithoutCancellingActiveRequests(t *testing.T) {
	t.Parallel()
	fixture := newTestAdmin(t)
	admin, workers := fixture.admin, fixture.workers
	out, err := workers.Register(context.Background(), &strawpb.RegisterRequest{WorkerId: "worker-1", ExecutorType: errorCategoryEgress, ProtocolMajor: ProtocolMajor, MaxConcurrency: 2})
	if err != nil || !out.OK {
		t.Fatalf("register = %+v, %v", out, err)
	}
	_, _ = workers.Heartbeat(&strawpb.HeartbeatRequest{WorkerId: "worker-1", SessionId: out.SessionID, Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, AvailableCapacity: 1, ActiveRequests: 1})
	current, _ := admin.Current()
	_, err = admin.SetWorker(current.Revision, "worker-1", "drain", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if got := workers.CandidatesForPool(config.DefaultDeploymentID, config.DefaultPoolID); len(got) != 0 {
		t.Fatalf("draining candidates = %+v", got)
	}
	info := workers.Workers()
	if len(info) != 1 || info[0].State != RuntimeDraining || info[0].ActiveRequests != 1 {
		t.Fatalf("worker info = %+v", info)
	}
}

func TestAdminCancellationInvokesRegisteredCancel(t *testing.T) {
	t.Parallel()
	fixture := newTestAdmin(t)
	admin, inflight := fixture.admin, fixture.inflight
	cancelled := false
	inflight.Register(context.Background(), "req-1", config.DefaultDeploymentID, func() { cancelled = true })
	if !admin.CancelRequest("req-1") || !cancelled {
		t.Fatal("CancelRequest did not invoke request cancellation")
	}
}
