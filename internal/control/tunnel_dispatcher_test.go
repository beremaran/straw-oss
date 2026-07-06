package control

import (
	"io"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

func TestTunnelClientEOFSendsCancel(t *testing.T) {
	t.Parallel()

	var sent []*strawpb.StreamFrame
	state := tunnelStreamState{
		c2e: &c2eStreamSender{
			seq:       7,
			publishFn: func(frame *strawpb.StreamFrame) { sent = append(sent, frame) },
		},
	}

	done, perr := state.clientEvent(io.EOF)
	if !done || perr == nil || perr.Code != Cancelled {
		t.Fatalf("clientEvent(io.EOF) = (%v, %+v), want cancelled terminal", done, perr)
	}
	if len(sent) != 1 {
		t.Fatalf("sent frames = %d, want 1", len(sent))
	}
	cancel := sent[0].GetCancel()
	if cancel == nil || cancel.GetReason() != "client_disconnect" {
		t.Fatalf("sent frame = %#v, want client_disconnect cancel", sent[0])
	}
	if sent[0].GetStreamSeq() != 7 {
		t.Fatalf("cancel seq = %d, want 7", sent[0].GetStreamSeq())
	}
}

func TestTunnelUploadGateWaitsForCreditGrant(t *testing.T) {
	t.Parallel()

	gate := newTunnelUploadGate(1)
	defer gate.close()

	done := make(chan bool, 1)
	go func() {
		done <- gate.take(2)
	}()

	select {
	case <-done:
		t.Fatal("take returned before enough upload credit was granted")
	case <-time.After(20 * time.Millisecond):
	}

	gate.grant(1)

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("take returned false after grant")
		}
	case <-time.After(time.Second):
		t.Fatal("take did not resume after upload credit grant")
	}
}

func TestTunnelIdleTimeoutSendsCancel(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	validator := natsx.NewStreamValidator(defaultRequestAttempt, 1, time.Millisecond, func() time.Time {
		return now
	})
	now = now.Add(2 * time.Millisecond)

	var sent []*strawpb.StreamFrame
	state := tunnelStreamState{
		c2e: &c2eStreamSender{
			seq:       3,
			publishFn: func(frame *strawpb.StreamFrame) { sent = append(sent, frame) },
		},
	}

	done, perr := state.idleEvent(validator)
	if !done || perr == nil || perr.Code != TimeoutExceeded || perr.TimeoutType != timeoutTypeIdle {
		t.Fatalf("idleEvent() = (%v, %+v), want idle timeout", done, perr)
	}
	if len(sent) != 1 {
		t.Fatalf("sent frames = %d, want 1", len(sent))
	}
	cancel := sent[0].GetCancel()
	if cancel == nil || cancel.GetReason() != "idle_timeout" {
		t.Fatalf("sent frame = %#v, want idle_timeout cancel", sent[0])
	}
}
