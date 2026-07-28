package control

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
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

func TestSharedHeartbeatRetainsProtocolAndProxyClaim(t *testing.T) {
	t.Parallel()

	const (
		poolID  = "proxy-pool"
		proxyID = "proxy-profile"
	)

	state := newFakeRuntimeState()
	registry := NewSharedWorkerRegistry(context.Background(), DefaultWorkerTimings(), nil, state, 30*time.Second)
	snapshot := config.NewSnapshot(1)
	snapshot.ExecutorPools = []config.ExecutorPool{{
		ID: poolID, ExecutorType: errorCategoryEgress, Enabled: true,
		UpstreamProxy: &config.ExecutorPoolUpstreamProxy{ID: proxyID, TrustedRemoteResolution: true},
	}}
	registry.ApplySnapshot(snapshot)

	outcome, err := registry.Register(context.Background(), &strawpb.RegisterRequest{
		WorkerId: sharedTestWorkerID, ExecutorType: errorCategoryEgress,
		ProtocolMajor: ProtocolMajor, ProtocolMinor: 2, MaxConcurrency: 2,
		AllowedPools: []*strawpb.RegisterRequest_PoolRef{{PoolId: poolID, UpstreamProxyId: proxyID}},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Register() = %+v, %v", outcome, err)
	}

	ok, err := registry.Heartbeat(context.Background(), &strawpb.HeartbeatRequest{
		WorkerId: sharedTestWorkerID, SessionId: outcome.SessionID,
		Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, MaxConcurrency: 2, AvailableCapacity: 2,
	})
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = %v, %v", ok, err)
	}

	stored := state.sessions[sharedTestWorkerID]
	if stored.SupportedProtocolMinor != 2 || stored.ProtocolMinor != 2 || len(stored.Pools) != 1 || stored.Pools[0].UpstreamProxyID != proxyID {
		t.Fatalf("shared heartbeat row = %+v", stored)
	}
	candidates := registry.CandidatesForPool(config.DefaultDeploymentID, poolID)
	if len(candidates) != 1 || candidates[0].SupportedProtocolMinor != 2 || candidates[0].ProtocolMinor != 2 || candidates[0].UpstreamProxyID != proxyID {
		t.Fatalf("shared candidates = %+v", candidates)
	}
}

func TestOldSharedWorkerRowIsDirectEligibleAndProxyIneligible(t *testing.T) {
	t.Parallel()

	const (
		directPoolID = "direct-pool"
		proxyPoolID  = "proxy-pool"
		proxyID      = "proxy-profile"
	)

	now := time.Now()
	legacy := sharedWorker{
		SessionID: "legacy-session", ExecutorType: errorCategoryEgress,
		Pools:        []AllowedPool{{PoolID: directPoolID}, {PoolID: proxyPoolID}},
		IngressModes: []string{IngressTypeREST}, MaxConcurrency: 1, AvailableCapacity: 1,
		Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, RegisteredAt: now, LastHeartbeat: now, HasHeartbeat: true,
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy row: %v", err)
	}
	if bytes.Contains(raw, []byte("protocol_minor")) || bytes.Contains(raw, []byte("upstream_proxy_id")) {
		t.Fatalf("zero-value compatibility fields were serialized: %s", raw)
	}

	var decoded sharedWorker
	err = json.Unmarshal(raw, &decoded)
	if err != nil {
		t.Fatalf("unmarshal legacy row: %v", err)
	}
	state := newFakeRuntimeState()
	state.sessions[sharedTestWorkerID] = decoded
	registry := NewSharedWorkerRegistry(context.Background(), DefaultWorkerTimings(), func() time.Time { return now }, state, 30*time.Second)
	rule := func(poolID string) *StaticRuleProvider {
		return NewStaticRuleProvider([]RoutingRule{{ID: poolID, DeploymentID: config.DefaultDeploymentID, Enabled: true, TargetPoolID: poolID}})
	}

	direct := NewRouter(
		rule(directPoolID),
		NewStaticPoolPolicyProvider([]PoolPolicy{{DeploymentID: config.DefaultDeploymentID, PoolID: directPoolID, Enabled: true}}),
		registry, NewStickyStore(nil), nil,
	).Evaluate(RouteRequest{DeploymentID: config.DefaultDeploymentID, IngressType: IngressTypeREST})
	if !direct.OK || direct.WorkerID != sharedTestWorkerID || direct.ProtocolMinor != 0 {
		t.Fatalf("legacy direct route = %+v", direct)
	}

	proxy := NewRouter(
		rule(proxyPoolID),
		NewStaticPoolPolicyProvider([]PoolPolicy{{DeploymentID: config.DefaultDeploymentID, PoolID: proxyPoolID, Enabled: true, UpstreamProxyID: proxyID, TrustedRemoteResolution: true}}),
		registry, NewStickyStore(nil), nil,
	).Evaluate(RouteRequest{DeploymentID: config.DefaultDeploymentID, IngressType: IngressTypeREST})
	if proxy.ErrorCode != RouteErrUnavailable {
		t.Fatalf("legacy proxy route = %+v, want %s", proxy, RouteErrUnavailable)
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
	sticky   map[string]string
}

func newFakeRuntimeState() *fakeRuntimeState {
	return &fakeRuntimeState{sessions: make(map[string]sharedWorker), owners: make(map[string]string), sticky: make(map[string]string)}
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

func (f *fakeRuntimeState) getSticky(_ context.Context, deployment, session string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	worker, ok := f.sticky[deployment+"\x00"+session]

	return worker, ok, nil
}

func (f *fakeRuntimeState) setSticky(_ context.Context, deployment, session, worker string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sticky[deployment+"\x00"+session] = worker

	return nil
}

func TestSharedStickyBackendPinsAcrossControls(t *testing.T) {
	t.Parallel()
	state := newFakeRuntimeState()
	backendA := NewRedisStickyBackend(context.Background(), state)
	backendB := NewRedisStickyBackend(context.Background(), state)
	rules := []RoutingRule{{ID: "sticky-route", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID, StickySessionTTLSeconds: 60}}
	candidates := testRoutingCandidates{routingPoolID: {routingCandidate("worker-a"), routingCandidate("worker-b")}}
	policies := NewStaticPoolPolicyProvider([]PoolPolicy{{DeploymentID: routingDeploymentID, PoolID: routingPoolID, Enabled: true}})
	request := RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST, StickySessionID: routingSessionID}

	first := NewRouter(NewStaticRuleProvider(rules), policies, candidates, backendA, nil).Evaluate(request)
	second := NewRouter(NewStaticRuleProvider(rules), policies, candidates, backendB, nil).Evaluate(request)
	if !first.OK || !second.OK || !first.Sticky || !second.Sticky || first.WorkerID != second.WorkerID {
		t.Fatalf("shared sticky outcomes = %+v and %+v", first, second)
	}
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
