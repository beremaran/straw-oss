package natsx

import (
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

func TestStreamValidatorRules(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	v := NewStreamValidator(3, 5, time.Second, func() time.Time { return now })

	if got := v.Accept(&strawpb.StreamFrame{StreamSeq: 2, Attempt: 3, Payload: &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}}}); got != FrameSequenceGap {
		t.Fatalf("gap outcome = %v, want FrameSequenceGap", got)
	}
	frame := &strawpb.StreamFrame{
		StreamSeq: 1,
		Attempt:   3,
		Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 0, Data: []byte("abc")}},
	}
	if got := v.Accept(frame); got != FrameAccepted {
		t.Fatalf("data accept outcome = %v, want accepted", got)
	}
	if got := v.Accept(frame); got != FrameDuplicate {
		t.Fatalf("duplicate outcome = %v, want duplicate", got)
	}
	if v.RemainingCredit() != 2 || v.ExpectedSeq() != 2 {
		t.Fatalf("state after data = credit %d seq %d, want 2/2", v.RemainingCredit(), v.ExpectedSeq())
	}
	if got := v.Accept(&strawpb.StreamFrame{StreamSeq: 2, Attempt: 3, Payload: &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}}}); got != FrameAccepted || !v.Terminated() {
		t.Fatalf("terminal outcome = %v terminated=%v, want accepted/true", got, v.Terminated())
	}
	if got := v.Accept(&strawpb.StreamFrame{StreamSeq: 3, Attempt: 3, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 3, Data: []byte("z")}}}); got != FrameAfterTerminal {
		t.Fatalf("late frame outcome = %v, want FrameAfterTerminal", got)
	}
}

func TestStreamValidatorCreditOffsetAndIdle(t *testing.T) {
	t.Parallel()

	now := time.Unix(200, 0)
	current := now
	v := NewStreamValidator(1, 1, time.Second, func() time.Time { return current })

	if got := v.Accept(&strawpb.StreamFrame{StreamSeq: 1, Attempt: 1, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 1, Data: []byte("x")}}}); got != FrameOffsetMismatch {
		t.Fatalf("offset mismatch outcome = %v, want FrameOffsetMismatch", got)
	}
	if got := v.Accept(&strawpb.StreamFrame{StreamSeq: 1, Attempt: 1, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 0, Data: []byte("xy")}}}); got != FrameCreditExhausted {
		t.Fatalf("credit exhaustion outcome = %v, want FrameCreditExhausted", got)
	}
	current = current.Add(2 * time.Second)
	if !v.IdleExpired() {
		t.Fatal("IdleExpired() = false, want true after timeout")
	}
}
