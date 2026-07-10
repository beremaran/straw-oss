package control

import (
	"errors"
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
	if !a.CanFallback() {
		t.Fatal("reject before RequestStart should allow fallback")
	}

	b := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, false)
	if !b.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED) {
		t.Fatal("accepted ack rejected")
	}
	if b.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_ERROR) {
		t.Fatal("duplicate assignment ack should be ignored")
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

func TestAssignmentPreStartFailuresAllowFallback(t *testing.T) {
	t.Parallel()

	timeout := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, false)
	if !timeout.OnAckTimeout() {
		t.Fatal("OnAckTimeout() = false, want true")
	}
	if !timeout.CanFallback() {
		t.Fatal("assignment timeout before RequestStart should allow fallback")
	}

	lost := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, false)
	if !lost.SynthesizeTerminal(strawpb.ErrorCode_ERROR_CODE_WORKER_DISCONNECTED) {
		t.Fatal("SynthesizeTerminal() = false, want true")
	}
	if !lost.CanFallback() {
		t.Fatal("worker loss before RequestStart should allow fallback")
	}

	deadline := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, true)
	if !deadline.SynthesizeTerminal(strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED) {
		t.Fatal("deadline SynthesizeTerminal() = false, want true")
	}
	if deadline.CanFallback() {
		t.Fatal("total deadline terminal should not allow fallback")
	}
}

func TestAssignmentWorkerLossBeforeRequestStartAllowsFallback(t *testing.T) {
	t.Parallel()

	a := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, false)
	if !a.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED) {
		t.Fatal("accepted ack rejected")
	}
	if !a.SynthesizeTerminal(strawpb.ErrorCode_ERROR_CODE_WORKER_DISCONNECTED) {
		t.Fatal("pre-start worker loss did not synthesize terminal outcome")
	}
	if !a.CanFallback() {
		t.Fatal("worker loss before RequestStart should allow fallback")
	}
}

func TestAssignmentOutboundStartDoesNotRelaxFallbackBoundary(t *testing.T) {
	t.Parallel()

	nonReplayable := NewAssignment("req_1", "ten_a", "worker_1", "sess_1", 1, false)
	if !nonReplayable.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED) || !nonReplayable.MarkRequestStart() {
		t.Fatal("failed to start non-replayable assignment")
	}
	if nonReplayable.CanFallback() {
		t.Fatal("non-replayable request must not fallback after RequestStart, even before OutboundStart")
	}

	replayable := NewAssignment("req_2", "ten_a", "worker_1", "sess_1", 1, true)
	if !replayable.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED) || !replayable.MarkRequestStart() {
		t.Fatal("failed to start replayable assignment")
	}
	if !replayable.CanFallback() {
		t.Fatal("replayable request may fallback after RequestStart until client response starts")
	}
	replayable.MarkClientResponded()
	if replayable.CanFallback() {
		t.Fatal("client-visible response forbids fallback; OutboundStart is not a separate P1 replay boundary")
	}
}

func TestFingerprintProfileExecutedMismatchRejected(t *testing.T) {
	t.Parallel()

	if !validateExecutedFingerprint("chrome_120", "chrome_120") {
		t.Fatal("matching executed profile was rejected")
	}
	if validateExecutedFingerprint("chrome_120", "firefox_121") {
		t.Fatal("mismatched executed profile was accepted")
	}
	if validateExecutedFingerprint("chrome_120", "") {
		t.Fatal("missing executed profile was accepted for named selection")
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

	replayable := NewAssignment("req_2", "ten_a", "worker_1", "sess_1", 1, true)
	if !replayable.OnAssignAck(strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED) || !replayable.MarkRequestStart() {
		t.Fatal("failed to start replayable assignment")
	}
	if !replayable.CanFallback() {
		t.Fatal("replayable request before client response should allow fallback after RequestStart")
	}
	replayable.MarkClientResponded()
	if replayable.CanFallback() {
		t.Fatal("client-visible response should forbid replay fallback")
	}

	err := AuthorizeAdminCancel(Identity{ScopeType: ScopeTenant, TenantID: adminTestTenantA, Role: RoleTenantAdmin}, adminTestTenantA)
	if err != nil {
		t.Fatalf("tenant admin cancel own tenant error = %v", err)
	}
	err = AuthorizeAdminCancel(Identity{ScopeType: ScopeTenant, TenantID: adminTestTenantA, Role: RoleTenantAdmin}, adminTestTenantB)
	if !errors.Is(err, ErrInsufficientPermissions) {
		t.Fatalf("cross-tenant cancel error = %v, want ErrInsufficientPermissions", err)
	}
	err = AuthorizeAdminCancel(Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, "ten_b")
	if err != nil {
		t.Fatalf("system admin cancel error = %v", err)
	}
}

func TestValidateExecutorErrorMapsOutOfSetCodes(t *testing.T) {
	t.Parallel()

	if mapped, violation := ValidateExecutorError(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE); violation || mapped != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE {
		t.Fatalf("in-set mapping = %v/%v, want passthrough without violation", mapped, violation)
	}
	if mapped, violation := ValidateExecutorError(strawpb.ErrorCode_ERROR_CODE_UNSPECIFIED); !violation || mapped != strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR {
		t.Fatalf("unspecified mapping = %v/%v, want executor_internal_error/violation", mapped, violation)
	}
	if mapped, violation := ValidateExecutorError(strawpb.ErrorCode_ERROR_CODE_ROUTE_NO_MATCH); !violation || mapped != strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR {
		t.Fatalf("out-of-set mapping = %v/%v, want executor_internal_error/violation", mapped, violation)
	}
}

func TestAckDeadlineUsesEarlierClock(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	if _, got := ackTimeout(now, 10*time.Second, now.Add(10*time.Second)); got != strawpb.TimeoutType_TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT {
		t.Fatalf("ackTimeout simultaneous type = %v, want total deadline timeout", got)
	}
	if got := ackDeadline(now, 10*time.Second, now.Add(5*time.Second)); !got.Equal(now.Add(5 * time.Second)) {
		t.Fatalf("ackDeadline() = %v, want total deadline", got)
	}
	if got := ackDeadline(now, 10*time.Second, time.Time{}); !got.Equal(now.Add(10 * time.Second)) {
		t.Fatalf("ackDeadline() = %v, want ack deadline", got)
	}
}
