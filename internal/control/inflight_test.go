package control

import (
	"errors"
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
	r.Register("req_1", inflightTestTenantA, func() { called = true })

	err := r.Cancel(Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_1")
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
	r.Register("req_1", inflightTestTenantB, func() { called = true })

	err := r.Cancel(Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_1")
	if err != nil || !called {
		t.Fatalf("Cancel() error = %v called = %v, want nil/true", err, called)
	}
}

func TestInFlightRegistryTenantAdminCancelsOwnTenant(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register("req_1", inflightTestTenantA, func() { called = true })

	identity := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleTenantAdmin}

	err := r.Cancel(identity, "req_1")
	if err != nil || !called {
		t.Fatalf("Cancel() error = %v called = %v, want nil/true", err, called)
	}
}

func TestInFlightRegistryOperatorCancelsOwnTenant(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register("req_1", inflightTestTenantA, func() { called = true })

	identity := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleOperator}

	err := r.Cancel(identity, "req_1")
	if err != nil || !called {
		t.Fatalf("Cancel() error = %v called = %v, want nil/true", err, called)
	}
}

func TestInFlightRegistryForeignTenantInsufficientPermissions(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register("req_1", inflightTestTenantB, func() { called = true })

	identity := Identity{ScopeType: ScopeTenant, TenantID: inflightTestTenantA, Role: RoleTenantAdmin}

	err := r.Cancel(identity, "req_1")
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

	err := r.Cancel(identity, "req_unknown")
	if !errors.Is(err, ErrInsufficientPermissions) {
		t.Fatalf("Cancel() error = %v, want ErrInsufficientPermissions (same as foreign tenant)", err)
	}
}

func TestInFlightRegistryUnknownRequestPlatformScope(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	identity := Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}

	err := r.Cancel(identity, "req_unknown")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrRequestNotFound", err)
	}
}

func TestInFlightRegistryDeregisterRemovesEntry(t *testing.T) {
	t.Parallel()

	r := NewInFlightRegistry()

	called := false
	r.Register("req_1", inflightTestTenantA, func() { called = true })
	r.Deregister("req_1")

	identity := Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}

	err := r.Cancel(identity, "req_1")
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

	r.Register("req_1", inflightTestTenantA, func() {})
	r.Deregister("req_1")

	err := r.Cancel(Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "req_1")
	if !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("Cancel() on nil registry error = %v, want ErrRequestNotFound", err)
	}
}
