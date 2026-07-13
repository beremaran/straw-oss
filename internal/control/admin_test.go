package control

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/beremaran/straw-oss/internal/config"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
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
	out, err := workers.Register(context.Background(), &strawpb.RegisterRequest{WorkerId: sharedTestWorkerID, ExecutorType: errorCategoryEgress, ProtocolMajor: ProtocolMajor, MaxConcurrency: 2})
	if err != nil || !out.OK {
		t.Fatalf("register = %+v, %v", out, err)
	}
	_, _ = workers.Heartbeat(context.Background(), &strawpb.HeartbeatRequest{WorkerId: sharedTestWorkerID, SessionId: out.SessionID, Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, AvailableCapacity: 1, ActiveRequests: 1})
	current, _ := admin.Current()
	_, err = admin.SetWorker(current.Revision, sharedTestWorkerID, "drain", "operator")
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
	if !admin.CancelRequest(context.Background(), "req-1") || !cancelled {
		t.Fatal("CancelRequest did not invoke request cancellation")
	}
}

func TestAdminUpdateRejectsInvalidSnapshotWithoutPersistenceOrActivation(t *testing.T) {
	t.Parallel()
	fixture := newTestAdmin(t)
	current, _ := fixture.admin.Current()
	invalid := current.Snapshot.Clone()
	invalid.InjectionPolicies = []config.InjectionPolicy{{ID: "bad", Enabled: true, Operations: []config.InjectionOperation{{Op: "replace", HeaderName: "X-Test"}}}}

	_, err := fixture.admin.Update(current.Revision, invalid, "operator", "update")
	if !errors.Is(err, config.ErrInvalidSnapshot) {
		t.Fatalf("Update() error = %v, want invalid snapshot", err)
	}
	got, _ := fixture.admin.Current()
	if got.Revision != current.Revision || got.Snapshot.ConfigVersion != current.Snapshot.ConfigVersion {
		t.Fatalf("invalid update changed current record: before=%+v after=%+v", current, got)
	}
	history, _ := fixture.admin.History()
	if len(history) != 1 {
		t.Fatalf("invalid update changed history: %+v", history)
	}
	if fixture.admin.cache.Snapshot().ConfigVersion != current.Snapshot.ConfigVersion {
		t.Fatal("invalid update changed active cache")
	}
}

func TestAdminUpdatePersistsNormalizedRulesAndRollbackRemainsValid(t *testing.T) {
	t.Parallel()
	fixture := newTestAdmin(t)
	current, _ := fixture.admin.Current()
	next := current.Snapshot.Clone()
	next.DenyRules = []config.DenyRule{{ID: "blocked", RuleType: "host", Action: "deny", Enabled: true, RawPattern: "Blocked.Example."}}
	next.InjectionPolicies = []config.InjectionPolicy{{ID: "headers", Enabled: true, Operations: []config.InjectionOperation{{Op: "set", HeaderName: testTraceHeaderName, ValueBase64: base64.StdEncoding.EncodeToString([]byte("one"))}}}}

	updated, err := fixture.admin.Update(current.Revision, next, "operator", "update")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := updated.Snapshot.DenyRules[0].NormalizedHost; got != "blocked.example" {
		t.Fatalf("persisted normalized host = %q", got)
	}
	if got := updated.Snapshot.DenyRules[0].NormalizedCIDR; got != "" {
		t.Fatalf("stale normalized CIDR survived raw host change: %q", got)
	}

	rolled, err := fixture.admin.Rollback(updated.Revision, current.Snapshot.ConfigVersion, "operator")
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if rolled.Snapshot.ConfigVersion != updated.Snapshot.ConfigVersion+1 || len(rolled.Snapshot.DenyRules) != 0 {
		t.Fatalf("rollback = %+v, want valid prior snapshot as a new version", rolled)
	}
	err = config.ValidateSnapshot(rolled.Snapshot)
	if err != nil {
		t.Fatalf("rolled snapshot is not valid: %v", err)
	}
}

func TestAdminAcceptsExecutableSnapshot(t *testing.T) {
	t.Parallel()
	fixture := newTestAdmin(t)
	current, _ := fixture.admin.Current()
	next := current.Snapshot.Clone()
	next.FingerprintProfiles = []config.FingerprintProfile{{
		Name: fingerprintProfileChrome120, ScopeType: "global", SupportedByWorker: true, Enabled: true,
		ExecutorType: "egress", ProfileRef: fingerprintProfileChrome120, ContractRevision: "tls-client-v1.15.1-http1-http2",
	}}

	_, err := fixture.admin.Update(current.Revision, next, "operator", "update")
	if err != nil {
		t.Fatalf("Update() rejected executable snapshot: %v", err)
	}
}
