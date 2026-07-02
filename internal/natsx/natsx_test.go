package natsx

import (
	"strings"
	"testing"
)

func TestSubjects(t *testing.T) {
	t.Parallel()

	assignment, err := AssignmentSubject("worker-01", "session_02")
	if err != nil {
		t.Fatalf("AssignmentSubject() error = %v", err)
	}
	if want := "straw.v1.executor.worker-01.session_02.assign"; assignment != want {
		t.Fatalf("AssignmentSubject() = %q, want %q", assignment, want)
	}

	c2e, err := StreamSubject("request-99", "worker-01", "session_02", DirectionControlToExecutor)
	if err != nil {
		t.Fatalf("StreamSubject(c2e) error = %v", err)
	}
	if want := "straw.v1.req.request-99.worker-01.session_02.c2e"; c2e != want {
		t.Fatalf("StreamSubject(c2e) = %q, want %q", c2e, want)
	}

	e2c, err := TerminalSubject("request-99", "worker-01", "session_02", DirectionExecutorToControl)
	if err != nil {
		t.Fatalf("TerminalSubject() error = %v", err)
	}
	if want := "straw.v1.req.request-99.worker-01.session_02.e2c"; e2c != want {
		t.Fatalf("TerminalSubject() = %q, want %q", e2c, want)
	}
}

func TestValidateSubjectToken(t *testing.T) {
	t.Parallel()

	if err := validateSubjectToken("worker-01"); err != nil {
		t.Fatalf("validateSubjectToken() error = %v", err)
	}

	for _, token := range []string{"", "worker.01", "worker 01", "worker*01", "worker>01"} {
		if err := validateSubjectToken(token); err == nil {
			t.Fatalf("validateSubjectToken(%q) = nil, want error", token)
		}
	}
}

func TestValidateMaxPayload(t *testing.T) {
	t.Parallel()

	maxPayload := uint64(1_120_000)
	if err := ValidateMaxPayload(&maxPayload, 1_048_576, 1_000_000, 1_000_000); err != nil {
		t.Fatalf("ValidateMaxPayload() error = %v", err)
	}

	tooSmall := uint64(1_100_000)
	err := ValidateMaxPayload(&tooSmall, 1_048_576, 1_000_000, 1_000_000)
	if err == nil {
		t.Fatal("ValidateMaxPayload() = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "safety margin") {
		t.Fatalf("ValidateMaxPayload() error = %q, want safety margin message", got)
	}
}

func TestValidateServers(t *testing.T) {
	t.Parallel()

	if err := ValidateServers([]string{"nats://nats:4222"}); err != nil {
		t.Fatalf("ValidateServers() error = %v", err)
	}

	if err := ValidateServers(nil); err == nil {
		t.Fatal("ValidateServers(nil) = nil, want error")
	}
}
