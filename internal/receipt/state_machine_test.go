package receipt

import (
	"errors"
	"testing"
)

func TestReceiptLifecycleTransitions(t *testing.T) {
	t.Parallel()

	states := []string{StateUploading, StateVerifying, StateVerified, StateAssigned, StateConsumed, StateRejected, StateCancelled, StateExpired}
	want := map[[2]string]bool{
		{StateUploading, StateVerifying}: true, {StateUploading, StateCancelled}: true, {StateUploading, StateExpired}: true,
		{StateVerifying, StateVerified}: true, {StateVerifying, StateRejected}: true, {StateVerifying, StateCancelled}: true, {StateVerifying, StateExpired}: true,
		{StateVerified, StateAssigned}: true, {StateVerified, StateCancelled}: true, {StateVerified, StateExpired}: true,
		{StateAssigned, StateVerified}: true, {StateAssigned, StateConsumed}: true, {StateAssigned, StateExpired}: true,
	}
	for _, from := range states {
		for _, to := range states {
			if got := canTransition(from, to); got != want[[2]string{from, to}] {
				t.Fatalf("transition %s -> %s = %v, want %v", from, to, got, want[[2]string{from, to}])
			}
		}
	}
}

func TestReceiptTerminalStatesRejectTransitions(t *testing.T) {
	t.Parallel()
	for _, state := range []string{StateConsumed, StateRejected, StateCancelled, StateExpired} {
		record := Record{State: state}
		err := transition(&record, StateUploading)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("terminal state %s returned %v", state, err)
		}
	}
}
