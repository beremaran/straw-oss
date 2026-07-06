package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

const (
	loggingTestService  = "control"
	loggingTestRequest  = "req-1"
	loggingTestTenant   = "tenant-1"
	loggingTestTrace    = "trace-1"
	loggingTestPoolID   = "pool-1"
	loggingTestError    = "route_no_match"
	loggingTestWorkerID = "worker-1"
)

func TestHandlerEmitsSingleLineJSONWithRequiredKeys(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(NewHandler(&buf)).With("service", loggingTestService)
	logger.Info("worker discovery subscribed", "request_id", loggingTestRequest, "tenant_id", loggingTestTenant)

	output := strings.TrimRight(buf.String(), "\n")
	if strings.Contains(output, "\n") {
		t.Fatalf("expected a single JSON line, got multiple: %q", output)
	}

	var record map[string]any

	err := json.Unmarshal([]byte(output), &record)
	if err != nil {
		t.Fatalf("log line is not valid JSON: %v (line: %q)", err, output)
	}

	for _, key := range []string{"service", "timestamp", "level", "msg"} {
		if _, ok := record[key]; !ok {
			t.Errorf("log record missing required key %q: %v", key, record)
		}
	}

	if record["service"] != loggingTestService {
		t.Errorf("service = %v, want %q", record["service"], loggingTestService)
	}

	if record["request_id"] != loggingTestRequest {
		t.Errorf("request_id = %v, want %q", record["request_id"], loggingTestRequest)
	}

	if record["tenant_id"] != loggingTestTenant {
		t.Errorf("tenant_id = %v, want %q", record["tenant_id"], loggingTestTenant)
	}
}

func TestTeeHandlerMapsAndRedactsLogEvents(t *testing.T) {
	var buf bytes.Buffer
	recorder := &recordingLogRecorder{}
	logger := slog.New(NewTeeHandler(NewHandler(&buf), recorder)).With("service", loggingTestService, "tenant_id", loggingTestTenant)

	logger.Error(
		"request failed",
		"request_id", loggingTestRequest,
		"trace_id", loggingTestTrace,
		"error_code", loggingTestError,
		"api_key_secret", "straw_secret",
		"pool_id", loggingTestPoolID,
		"subject", "straw.v1.req.req-1.worker-1.sess-1.e2c",
		"url", "https://user:pass@example.com/proxy",
		"signed", "https://bucket.example/object?X-Amz-Signature=abc",
	)

	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}

	event := recorder.events[0]
	if event.Service != loggingTestService || event.Level != slog.LevelError.String() || event.Message != "request failed" {
		t.Fatalf("event basics = %+v", event)
	}
	if event.RequestID != loggingTestRequest || event.TenantID != loggingTestTenant || event.TraceID != loggingTestTrace || event.ErrorCode != loggingTestError {
		t.Fatalf("event contextual fields = %+v", event)
	}
	if event.Extra["api_key_secret"] != redactedValue {
		t.Fatalf("api_key_secret = %q, want redacted", event.Extra["api_key_secret"])
	}
	for _, key := range []string{"subject", "url", "signed"} {
		if event.Extra[key] != redactedValue {
			t.Fatalf("%s = %q, want redacted", key, event.Extra[key])
		}
	}
	if event.Extra["pool_id"] != loggingTestPoolID {
		t.Fatalf("pool_id extra = %q, want %q", event.Extra["pool_id"], loggingTestPoolID)
	}
}

type recordingLogRecorder struct {
	events []LogEvent
}

func (r *recordingLogRecorder) Enqueue(event LogEvent) {
	r.events = append(r.events, event)
}

var _ LogEventRecorder = (*recordingLogRecorder)(nil)
