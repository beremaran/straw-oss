package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

const loggingTestService = "control"

func TestHandlerEmitsSingleLineJSONWithRequiredKeys(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(NewHandler(&buf)).With("service", loggingTestService)
	logger.Info("worker discovery subscribed", "request_id", "req-1", "tenant_id", "tenant-1")

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

	if record["request_id"] != "req-1" {
		t.Errorf("request_id = %v, want %q", record["request_id"], "req-1")
	}

	if record["tenant_id"] != "tenant-1" {
		t.Errorf("tenant_id = %v, want %q", record["tenant_id"], "tenant-1")
	}
}

func TestTeeHandlerMapsAndRedactsLogEvents(t *testing.T) {
	var buf bytes.Buffer
	recorder := &recordingLogRecorder{}
	logger := slog.New(NewTeeHandler(NewHandler(&buf), recorder)).With("service", loggingTestService, "tenant_id", "tenant-1")

	logger.Error("request failed", "request_id", "req-1", "trace_id", "trace-1", "error_code", "route_no_match", "api_key_secret", "straw_secret", "pool_id", "pool-1")

	if len(recorder.events) != 1 {
		t.Fatalf("events = %d, want 1", len(recorder.events))
	}

	event := recorder.events[0]
	if event.Service != loggingTestService || event.Level != slog.LevelError.String() || event.Message != "request failed" {
		t.Fatalf("event basics = %+v", event)
	}
	if event.RequestID != "req-1" || event.TenantID != "tenant-1" || event.TraceID != "trace-1" || event.ErrorCode != "route_no_match" {
		t.Fatalf("event contextual fields = %+v", event)
	}
	if event.Extra["api_key_secret"] != redactedValue {
		t.Fatalf("api_key_secret = %q, want redacted", event.Extra["api_key_secret"])
	}
	if event.Extra["pool_id"] != "pool-1" {
		t.Fatalf("pool_id extra = %q, want pool-1", event.Extra["pool_id"])
	}
}

type recordingLogRecorder struct {
	events []LogEvent
}

func (r *recordingLogRecorder) Enqueue(event LogEvent) {
	r.events = append(r.events, event)
}

var _ LogEventRecorder = (*recordingLogRecorder)(nil)
