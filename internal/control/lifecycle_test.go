package control

import (
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

func TestAssignmentLifecycle(t *testing.T) {
	t.Parallel()

	a := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, false)

	if got := a.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_CAPACITY); got {
		t.Fatal("reject ack accepted, want false")
	}
	if a.State() != LifecycleTerminated || a.Terminal() != TerminalError {
		t.Fatalf("reject outcome = state %v terminal %v, want terminated/error", a.State(), a.Terminal())
	}

	b := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, false)
	if !b.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED) {
		t.Fatal("accepted ack rejected")
	}
	if !b.MarkRequestStart() {
		t.Fatal("MarkRequestStart() failed after accepted ack")
	}
	if b.CanFallback() {
		t.Fatal("non-replayable fallback should be forbidden after RequestStart")
	}
	if send, ok := b.Cancel(); !ok || !send {
		t.Fatalf("Cancel() = send=%v ok=%v, want true/true after RequestStart", send, ok)
	}
	if b.Terminal() != TerminalCancelled {
		t.Fatalf("Cancel() terminal = %v, want cancelled", b.Terminal())
	}
}

func TestAssignmentFallbackBoundaryAndAdminCancel(t *testing.T) {
	t.Parallel()

	a := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, true)
	if !a.CanFallback() {
		t.Fatal("fallback should be allowed before RequestStart")
	}
	if send, ok := a.Cancel(); !ok || send {
		t.Fatalf("Cancel() before RequestStart = send=%v ok=%v, want false/true", send, ok)
	}

	err := AuthorizeAdminCancel(Identity{ScopeType: ScopeTenant, TenantID: "ten_a", Role: RoleTenantAdmin}, "ten_a")
	if err != nil {
		t.Fatalf("tenant admin cancel own tenant error = %v", err)
	}
	err := AuthorizeAdminCancel(Identity{ScopeType: ScopeTenant, TenantID: "ten_a", Role: RoleTenantAdmin}, "ten_b")
	if !errors.Is(err, ErrInsufficientPermissions) {
		t.Fatalf("cross-tenant cancel error = %v, want ErrInsufficientPermissions", err)
	}
	err := AuthorizeAdminCancel(Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "ten_b")
	if err != nil {
		t.Fatalf("system admin cancel error = %v", err)
	}
}

func TestValidateExecutorErrorMapsOutOfSetCodes(t *testing.T) {
	t.Parallel()

	if mapped, violation := ValidateExecutorError(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE); violation || mapped != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE {
		t.Fatalf("in-set mapping = %v/%v, want passthrough without violation", mapped, violation)
	}
	if mapped, violation := ValidateExecutorError(strawpb.ErrorCode_ERROR_CODE_ROUTE_NO_MATCH); !violation || mapped != strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR {
		t.Fatalf("out-of-set mapping = %v/%v, want executor_internal_error/violation", mapped, violation)
	}
}

func TestAckDeadlineUsesEarlierClock(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	if got := ackDeadline(now, 10*time.Second, now.Add(5*time.Second)); !got.Equal(now.Add(5 * time.Second)) {
		t.Fatalf("ackDeadline() = %v, want total deadline", got)
	}
	if got := ackDeadline(now, 10*time.Second, time.Time{}); !got.Equal(now.Add(10 * time.Second)) {
		t.Fatalf("ackDeadline() = %v, want ack deadline", got)
	}
}
