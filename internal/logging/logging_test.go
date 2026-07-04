package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestHandlerEmitsSingleLineJSONWithRequiredKeys(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(NewHandler(&buf)).With("service", "control")
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

	if record["service"] != "control" {
		t.Errorf("service = %v, want %q", record["service"], "control")
	}

	if record["request_id"] != "req-1" {
		t.Errorf("request_id = %v, want %q", record["request_id"], "req-1")
	}

	if record["tenant_id"] != "tenant-1" {
		t.Errorf("tenant_id = %v, want %q", record["tenant_id"], "tenant-1")
	}
}
