package control

import (
	"context"
	"sync"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

// captureWorkerEventRecorder is a fake WorkerEventRecorder for asserting
// which worker_events rows (docs/implementation-history.md#p0-32) a registry transition emits.
type captureWorkerEventRecorder struct {
	mu     sync.Mutex
	events []WorkerEvent
}

func (r *captureWorkerEventRecorder) Enqueue(event WorkerEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
}

func (r *captureWorkerEventRecorder) all() []WorkerEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]WorkerEvent(nil), r.events...)
}

func TestWorkerRegistryEmitsRegisterEvent(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	recorder := &captureWorkerEventRecorder{}
	h.reg.SetEventRecorder(recorder)

	sess := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))

	events := recorder.all()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].EventType != workerEventRegister {
		t.Fatalf("event_type = %q, want %q", events[0].EventType, workerEventRegister)
	}
	if events[0].WorkerID != workerRegTestWorker1 {
		t.Fatalf("worker_id = %q, want %q", events[0].WorkerID, workerRegTestWorker1)
	}
	if events[0].SessionID != sess {
		t.Fatalf("session_id = %q, want %q", events[0].SessionID, sess)
	}
	if events[0].ExecutorType != workerRegTestEgress {
		t.Fatalf("executor_type = %q, want %q", events[0].ExecutorType, workerRegTestEgress)
	}
	if events[0].Timestamp.IsZero() {
		t.Fatal("timestamp is zero")
	}
}

func TestWorkerRegistryEmitsHeartbeatEvent(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	recorder := &captureWorkerEventRecorder{}
	h.reg.SetEventRecorder(recorder)

	sess := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))

	ok, err := h.reg.Heartbeat(&strawpb.HeartbeatRequest{
		WorkerId:          workerRegTestWorker1,
		SessionId:         sess,
		Health:            strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED,
		ActiveRequests:    3,
		AvailableCapacity: 7,
		MaxConcurrency:    10,
	})
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = (%v, %v), want (true, nil)", ok, err)
	}

	events := recorder.all()
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2 (register + heartbeat)", len(events))
	}

	hbEvent := events[1]
	if hbEvent.EventType != workerEventHeartbeat {
		t.Fatalf("event_type = %q, want %q", hbEvent.EventType, workerEventHeartbeat)
	}
	if hbEvent.Health != "degraded" {
		t.Fatalf("health = %q, want degraded", hbEvent.Health)
	}
	if hbEvent.ActiveRequests != 3 {
		t.Fatalf("active_requests = %d, want 3", hbEvent.ActiveRequests)
	}
	if hbEvent.AvailableCapacity != 7 {
		t.Fatalf("available_capacity = %d, want 7", hbEvent.AvailableCapacity)
	}
}

func TestWorkerRegistryEmitsDisableAndDrainEvents(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	recorder := &captureWorkerEventRecorder{}
	h.reg.SetEventRecorder(recorder)

	h.reg.SetGlobalAdmin(workerRegTestWorker1, AdminDisabled)
	h.reg.SetGlobalDrain(workerRegTestWorker1, true)
	h.reg.SetTenantAdmin(workerRegTestWorker1, workerRegTestTenantA, AdminDisabled)
	h.reg.SetTenantDrain(workerRegTestWorker1, workerRegTestTenantA, true)

	events := recorder.all()
	if len(events) != 4 {
		t.Fatalf("events len = %d, want 4", len(events))
	}

	wantTypes := []string{workerEventDisable, workerEventDrain, workerEventTenantDisable, workerEventTenantDrain}
	for i, want := range wantTypes {
		if events[i].EventType != want {
			t.Fatalf("events[%d].EventType = %q, want %q", i, events[i].EventType, want)
		}
	}

	if events[2].TenantID != workerRegTestTenantA {
		t.Fatalf("tenant_disable tenant_id = %q, want %q", events[2].TenantID, workerRegTestTenantA)
	}
	if events[0].TenantID != "" {
		t.Fatalf("global disable tenant_id = %q, want empty", events[0].TenantID)
	}
}

func TestWorkerEventWriterOutageDoesNotBlockRegistration(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	writer := NewWorkerEventWriter(failingWorkerEventSink{}, 10, 10, time.Hour)
	t.Cleanup(writer.Close)
	h.reg.SetEventRecorder(writer)

	out, err := h.reg.Register(context.Background(), h.signedRegister(workerRegTestWorker1))
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !out.OK {
		t.Fatalf("Register rejected: %s", out.Reason)
	}

	err = writer.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want outage error")
	}
}

type failingWorkerEventSink struct{}

func (failingWorkerEventSink) WriteWorkerEvents(context.Context, []WorkerEvent) error {
	return errClickHouseInsertFailed
}
