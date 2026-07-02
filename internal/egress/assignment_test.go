package egress

import (
	"testing"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

func TestEvaluateAssignmentPrecedence(t *testing.T) {
	t.Parallel()

	req := &strawpb.AssignRequest{Mode: strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP}
	if got := EvaluateAssignment(req, Capacity{Draining: true, MaxConcurrency: 0}); got != strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_DRAINING {
		t.Fatalf("draining decision = %v, want rejected_daining", got)
	}
	if got := EvaluateAssignment(req, Capacity{SupportedModes: []strawpb.RequestMode{strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL}}); got != strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_UNSUPPORTED {
		t.Fatalf("mode decision = %v, want rejected_unsupported", got)
	}
	if got := EvaluateAssignment(req, Capacity{MaxConcurrency: 1, ActiveRequests: 1}); got != strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_CAPACITY {
		t.Fatalf("capacity decision = %v, want rejected_capacity", got)
	}
	if got := EvaluateAssignment(req, Capacity{}); got != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		t.Fatalf("accept decision = %v, want accepted", got)
	}
}

func TestFakeExecutorScriptsLifecycleFrames(t *testing.T) {
	t.Parallel()

	f := NewFakeExecutor(7)
	frames := f.SuccessResponse("example.com", 443, 200, []byte("ok"))
	if len(frames) != 4 {
		t.Fatalf("len(frames) = %d, want 4", len(frames))
	}
	if frames[0].GetStreamSeq() != 1 || frames[3].GetStreamSeq() != 4 {
		t.Fatalf("frame seqs = %d..%d, want 1..4", frames[0].GetStreamSeq(), frames[3].GetStreamSeq())
	}
	if frames[3].GetEnd() == nil || !frames[3].GetEnd().GetSuccess() {
		t.Fatal("final frame is not a success EndFrame")
	}
}
