package natsx

import "testing"

func TestControlInboxPrefix(t *testing.T) {
	t.Parallel()
	if got := ControlInboxPrefix(); got != "_INBOX.ctl" {
		t.Fatalf("ControlInboxPrefix() = %q, want _INBOX.ctl", got)
	}
}

func TestWorkerInboxPrefix(t *testing.T) {
	t.Parallel()
	got, err := WorkerInboxPrefix("worker-1")
	if err != nil {
		t.Fatalf("WorkerInboxPrefix error: %v", err)
	}
	if got != "_INBOX.wrk.worker-1" {
		t.Fatalf("WorkerInboxPrefix() = %q, want _INBOX.wrk.worker-1", got)
	}
}

func TestWorkerInboxPrefixRejectsUnsafeToken(t *testing.T) {
	t.Parallel()
	_, err := WorkerInboxPrefix("bad.worker")
	if err == nil {
		t.Fatal("WorkerInboxPrefix accepted a dot-bearing worker_id, want error")
	}
}
