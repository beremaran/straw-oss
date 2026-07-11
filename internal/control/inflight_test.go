package control

import (
	"context"
	"errors"
	"sync"
	"testing"
)

const (
	inflightTestTenantA = "ten_a"
	inflightTestTenantB = "ten_b"
)

func TestInFlightRegistryCancelInvokesCancelFunc(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register(context.Background(), "req_1", inflightTestTenantA, func() { called = true })

	err := r.Cancel(context.Background(), Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_1")
	if err != nil {
		t.Fatalf("Cancel() error = %v, want nil", err)
	}

	if !called {
		t.Fatal("Cancel() did not invoke the registered cancel func")
	}
}

func TestInFlightRegistrySystemAdminCancelsAnyTenant(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register(context.Background(), "req_1", inflightTestTenantB, func() { called = true })

	err := r.Cancel(context.Background(), Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_1")
	if err != nil || !called {
		t.Fatalf("Cancel() error = %v called = %v, want nil/true", err, called)
	}
}

func TestInFlightRegistryTenantAdminCancelsOwnTenant(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register(context.Background(), "req_1", inflightTestTenantA, func() { called = true })

	identity := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleTenantAdmin}

	err := r.Cancel(context.Background(), identity, "req_1")
	if err != nil || !called {
		t.Fatalf("Cancel() error = %v called = %v, want nil/true", err, called)
	}
}

func TestInFlightRegistryOperatorCancelsOwnTenant(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register(context.Background(), "req_1", inflightTestTenantA, func() { called = true })

	identity := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleOperator}

	err := r.Cancel(context.Background(), identity, "req_1")
	if err != nil || !called {
		t.Fatalf("Cancel() error = %v called = %v, want nil/true", err, called)
	}
}

func TestInFlightRegistryForeignTenantInsufficientPermissions(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register(context.Background(), "req_1", inflightTestTenantB, func() { called = true })

	identity := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleTenantAdmin}

	err := r.Cancel(context.Background(), identity, "req_1")
	if !errors.Is(err, ErrInsufficientPermissions) {
		t.Fatalf("Cancel() error = %v, want ErrInsufficientPermissions", err)
	}

	if called {
		t.Fatal("Cancel() invoked cancel func for a foreign-tenant request")
	}
}

func TestInFlightRegistryUnknownRequestTenantScopeNoDisclosure(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	identity := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleTenantAdmin}

	err := r.Cancel(context.Background(), identity, "req_unknown")
	if !errors.Is(err, ErrInsufficientPermissions) {
		t.Fatalf("Cancel() error = %v, want ErrInsufficientPermissions (same as foreign tenant)", err)
	}
}

func TestInFlightRegistryUnknownRequestPlatformScope(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	identity := Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}

	err := r.Cancel(context.Background(), identity, "req_unknown")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrRequestNotFound", err)
	}
}

func TestInFlightRegistryDeregisterRemovesEntry(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register(context.Background(), "req_1", inflightTestTenantA, func() { called = true })
	r.Deregister(context.Background(), "req_1")

	identity := Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}

	err := r.Cancel(context.Background(), identity, "req_1")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrRequestNotFound after deregister", err)
	}

	if called {
		t.Fatal("Cancel() invoked cancel func after deregister")
	}
}

func TestInFlightRegistryNilSafe(t *testing.T) {
	t.Parallel()

	var r *InFlightRegistry

	r.Register(context.Background(), "req_1", inflightTestTenantA, func() {})
	r.Deregister(context.Background(), "req_1")

	err := r.Cancel(context.Background(), Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_1")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("Cancel() on nil registry error = %v, want ErrRequestNotFound", err)
	}
}

// fakeCluster simulates the shared Redis runtime-state tier for the
// cross-instance in-flight tests: an ownership map (Record/Clear/Lookup) plus a
// cancel signal broadcast to every attached replica, mirroring how the
// Redis-backed coordinator + pub/sub subscriber (docs/implementation-history.md#p1-23) behave. It
// also counts Lookup and SignalCancel calls so a test can prove the local fast
// path never reaches the shared backend.
type fakeCluster struct {
	mu       sync.Mutex
	owners   map[string]string
	replicas []*InFlightRegistry
	lookups  int
	signals  int
}

func newFakeCluster() *fakeCluster {
	return &fakeCluster{owners: make(map[string]string)}
}

// attach registers r as a replica and wires it to the shared backend.
func (c *fakeCluster) attach(r *InFlightRegistry) {
	c.mu.Lock()
	c.replicas = append(c.replicas, r)
	c.mu.Unlock()

	r.SetCrossInstance(&fakeCoordinator{cluster: c})
}

type fakeCoordinator struct{ cluster *fakeCluster }

func (f *fakeCoordinator) Record(_ context.Context, requestID, tenantID string) {
	f.cluster.mu.Lock()
	defer f.cluster.mu.Unlock()

	f.cluster.owners[requestID] = tenantID
}

func (f *fakeCoordinator) Clear(_ context.Context, requestID string) {
	f.cluster.mu.Lock()
	defer f.cluster.mu.Unlock()

	delete(f.cluster.owners, requestID)
}

func (f *fakeCoordinator) Lookup(_ context.Context, requestID string) (string, bool) {
	f.cluster.mu.Lock()
	defer f.cluster.mu.Unlock()

	f.cluster.lookups++
	tenantID, ok := f.cluster.owners[requestID]

	return tenantID, ok
}

func (f *fakeCoordinator) SignalCancel(_ context.Context, requestID string) error {
	f.cluster.mu.Lock()
	f.cluster.signals++
	replicas := append([]*InFlightRegistry(nil), f.cluster.replicas...)
	f.cluster.mu.Unlock()

	// Broadcast to every replica; only the owner acts (as pub/sub does).
	for _, r := range replicas {
		r.cancelLocal(requestID)
	}

	return nil
}

func TestInFlightRegistryCrossInstanceCancelReachesOwner(t *testing.T) {
	t.Parallel()

	cluster := newFakeCluster()
	instanceA := NewInFlightRegistry()
	instanceB := NewInFlightRegistry()
	cluster.attach(instanceA)
	cluster.attach(instanceB)

	// The request is in-flight on instance B only.
	cancelled := false
	instanceB.Register(context.Background(), "req_x", inflightTestTenantA, func() { cancelled = true })

	// The admin cancel is delivered to instance A, which does not own it.
	err := instanceA.Cancel(context.Background(), Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_x")
	if err != nil {
		t.Fatalf("cross-instance Cancel() error = %v, want nil", err)
	}

	if !cancelled {
		t.Fatal("cross-instance Cancel() did not tear down the request owned by instance B")
	}

	if cluster.signals != 1 {
		t.Fatalf("cross-instance signals = %d, want exactly 1 (no duplicate teardown)", cluster.signals)
	}
}

func TestInFlightRegistryLocalFastPathSkipsBackend(t *testing.T) {
	t.Parallel()

	cluster := newFakeCluster()
	r := NewInFlightRegistry()
	cluster.attach(r)

	cancelled := false
	r.Register(context.Background(), "req_local", inflightTestTenantA, func() { cancelled = true })

	err := r.Cancel(context.Background(), Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_local")
	if err != nil || !cancelled {
		t.Fatalf("local Cancel() err=%v cancelled=%v, want nil/true", err, cancelled)
	}

	// The locally-owned request must cancel in-process without consulting or
	// signaling the shared backend.
	if cluster.lookups != 0 || cluster.signals != 0 {
		t.Fatalf("local fast path touched shared backend: lookups=%d signals=%d, want 0/0", cluster.lookups, cluster.signals)
	}
}

func TestInFlightRegistryCrossInstanceUnknownReturnsNotFound(t *testing.T) {
	t.Parallel()

	cluster := newFakeCluster()
	instanceA := NewInFlightRegistry()
	instanceB := NewInFlightRegistry()
	cluster.attach(instanceA)
	cluster.attach(instanceB)

	// req_ghost is in-flight on no instance.
	err := instanceA.Cancel(context.Background(), Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_ghost")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("unknown cross-instance Cancel() error = %v, want ErrRequestNotFound", err)
	}

	if cluster.signals != 0 {
		t.Fatalf("unknown request must not signal teardown: signals = %d, want 0", cluster.signals)
	}
}

func TestInFlightRegistryCrossInstanceForeignTenantDenied(t *testing.T) {
	t.Parallel()

	cluster := newFakeCluster()
	instanceA := NewInFlightRegistry()
	instanceB := NewInFlightRegistry()
	cluster.attach(instanceA)
	cluster.attach(instanceB)

	cancelled := false
	instanceB.Register(context.Background(), "req_y", inflightTestTenantB, func() { cancelled = true })

	// A tenant-scoped admin from a different tenant must not cancel it, and the
	// authorization model is unchanged from P0 task 27.
	tenantAdmin := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleTenantAdmin}

	err := instanceA.Cancel(context.Background(), tenantAdmin, "req_y")
	if !errors.Is(err, ErrInsufficientPermissions) {
		t.Fatalf("foreign-tenant cross-instance Cancel() error = %v, want ErrInsufficientPermissions", err)
	}

	if cancelled || cluster.signals != 0 {
		t.Fatalf("foreign-tenant cancel must not tear down: cancelled=%v signals=%d", cancelled, cluster.signals)
	}
}
