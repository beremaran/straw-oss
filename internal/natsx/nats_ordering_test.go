package natsx

import (
	"errors"
	"testing"
)

var errNoReadySubscriber = errors.New("no ready subscriber")

type orderingHarness struct {
	ready map[string]bool
	seen  map[string]int
}

func newOrderingHarness() *orderingHarness {
	return &orderingHarness{
		ready: make(map[string]bool),
		seen:  make(map[string]int),
	}
}

func (h *orderingHarness) subscribe(subject string) {
	h.ready[subject] = false
}

func (h *orderingHarness) flush(subject string) {
	h.ready[subject] = true
}

func (h *orderingHarness) publish(subject string) error {
	if !h.ready[subject] {
		return errNoReadySubscriber
	}

	h.seen[subject]++

	return nil
}

func TestNATSOrderingRequiresFlushedStreamSubscription(t *testing.T) {
	t.Parallel()

	h := newOrderingHarness()
	subject, err := StreamSubject("req_1", "worker_1", "sess_1", DirectionControlToExecutor)
	if err != nil {
		t.Fatalf("StreamSubject() error = %v", err)
	}

	err = h.publish(subject)
	if !errors.Is(err, errNoReadySubscriber) {
		t.Fatalf("publish before subscribe error = %v, want errNoReadySubscriber", err)
	}

	h.subscribe(subject)
	err = h.publish(subject)
	if !errors.Is(err, errNoReadySubscriber) {
		t.Fatalf("publish before flush error = %v, want errNoReadySubscriber", err)
	}

	h.flush(subject)
	err = h.publish(subject)
	if err != nil {
		t.Fatalf("publish after flush error = %v", err)
	}
	if h.seen[subject] != 1 {
		t.Fatalf("delivered count = %d, want 1", h.seen[subject])
	}
}
