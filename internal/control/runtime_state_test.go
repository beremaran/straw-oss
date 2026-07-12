package control

import (
	"context"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw-oss/v2/api/proto/straw/v1"
)

const sharedTestWorkerID = "worker-1"

func TestSharedRegistriesRouteSameWorkerFleet(t *testing.T) {
	t.Parallel()
	state := newFakeRuntimeState()
	a := NewSharedWorkerRegistry(context.Background(), DefaultWorkerTimings(), nil, state, 30*time.Second)
	b := NewSharedWorkerRegistry(context.Background(), DefaultWorkerTimings(), nil, state, 30*time.Second)
	out, err := a.Register(context.Background(), &strawpb.RegisterRequest{WorkerId: sharedTestWorkerID, ExecutorType: "egress", ProtocolMajor: ProtocolMajor, MaxConcurrency: 2})
	if err != nil || !out.OK {
		t.Fatalf("Register() = %+v, %v", out, err)
	}
	ok, err := b.Heartbeat(context.Background(), &strawpb.HeartbeatRequest{WorkerId: sharedTestWorkerID, SessionId: out.SessionID, Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, MaxConcurrency: 2, AvailableCapacity: 2})
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = %v, %v", ok, err)
	}
	for name, registry := range map[string]*WorkerRegistry{"registering Control": a, "heartbeat Control": b} {
		candidates := registry.CandidatesForPool("default", "default")
		if len(candidates) != 1 || candidates[0].WorkerID != sharedTestWorkerID || candidates[0].SessionID != out.SessionID {
			t.Fatalf("%s candidates = %+v", name, candidates)
		}
	}
}

func TestSharedRegistryFencesReplacedWorkerSession(t *testing.T) {
	t.Parallel()
	state := newFakeRuntimeState()
	a := NewSharedWorkerRegistry(context.Background(), DefaultWorkerTimings(), nil, state, 30*time.Second)
	b := NewSharedWorkerRegistry(context.Background(), DefaultWorkerTimings(), nil, state, 30*time.Second)
	first, _ := a.Register(context.Background(), &strawpb.RegisterRequest{WorkerId: sharedTestWorkerID, ProtocolMajor: ProtocolMajor})
	second, _ := b.Register(context.Background(), &strawpb.RegisterRequest{WorkerId: sharedTestWorkerID, ProtocolMajor: ProtocolMajor})
	if first.SessionID == second.SessionID {
		t.Fatal("replacement reused fencing session")
	}
	ok, err := a.Heartbeat(context.Background(), &strawpb.HeartbeatRequest{WorkerId: sharedTestWorkerID, SessionId: first.SessionID})
	if err != nil || ok {
		t.Fatalf("stale Heartbeat() = %v, %v", ok, err)
	}
}

func TestSharedInFlightRoutesCancellationToOwningControl(t *testing.T) {
	t.Parallel()
	state := newFakeRuntimeState()
	owner := NewSharedInFlightRegistry(state, "control-a", time.Minute, nil)
	publisher := &recordingPublisher{}
	peer := NewSharedInFlightRegistry(state, "control-b", time.Minute, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner.Register(ctx, "req-1", "default", cancel)

	if !peer.Cancel(context.Background(), "req-1") {
		t.Fatal("peer did not route cancellation")
	}
	if publisher.subject != remoteCancelSubject("control-a") || string(publisher.data) != "req-1" {
		t.Fatalf("published cancellation = %q %q", publisher.subject, publisher.data)
	}
}

type fakeRuntimeState struct {
	mu       sync.Mutex
	sessions map[string]sharedWorker
	owners   map[string]string
}

func newFakeRuntimeState() *fakeRuntimeState {
	return &fakeRuntimeState{sessions: make(map[string]sharedWorker), owners: make(map[string]string)}
}
func (f *fakeRuntimeState) Ping(context.Context) error { return nil }
func (f *fakeRuntimeState) putWorker(_ context.Context, id string, w sharedWorker, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[id] = w

	return nil
}

func (f *fakeRuntimeState) heartbeatWorker(_ context.Context, id, session string, w sharedWorker, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	old, ok := f.sessions[id]
	if !ok || old.SessionID != session {
		return false, nil
	}
	f.sessions[id] = w

	return true, nil
}

func (f *fakeRuntimeState) workers(context.Context) (map[string]sharedWorker, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]sharedWorker, len(f.sessions))
	maps.Copy(out, f.sessions)

	return out, nil
}

func (f *fakeRuntimeState) recordWorkerFailure(context.Context, string, string, WorkerTimings) error {
	return nil
}

func (f *fakeRuntimeState) workerCoolingDown(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *fakeRuntimeState) getSticky(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

func (f *fakeRuntimeState) setSticky(context.Context, string, string, string, time.Duration) error {
	return nil
}

func (f *fakeRuntimeState) claimRequest(_ context.Context, requestID, deploymentID, owner string, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.owners[requestID]; exists {
		return false, nil
	}
	f.owners[requestID] = owner + "|" + deploymentID

	return true, nil
}

func (f *fakeRuntimeState) renewRequest(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}

func (f *fakeRuntimeState) releaseRequest(_ context.Context, requestID, owner string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.owners[requestID]
	if ok && strings.HasPrefix(value, owner+"|") {
		delete(f.owners, requestID)
	}

	return nil
}
func (f *fakeRuntimeState) requests(context.Context) ([]InFlightRequest, error) { return nil, nil }
func (f *fakeRuntimeState) requestOwner(_ context.Context, requestID string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.owners[requestID]
	owner, _, _ := strings.Cut(value, "|")

	return owner, ok, nil
}

type recordingPublisher struct {
	subject string
	data    []byte
}

func (p *recordingPublisher) Publish(subject string, data []byte) error {
	p.subject = subject
	p.data = append([]byte(nil), data...)

	return nil
}

func (f *fakeRuntimeState) touchInstance(context.Context, string, string, time.Duration) error {
	return nil
}
