// Package logging builds the structured JSON slog logger shared by the
// control and egress binaries, per docs/planning/23-observability.md: every
// record carries service, timestamp, and level; request_id, tenant_id,
// error_code, and worker_id are attached by call sites where available.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"
)

const redactedValue = "[redacted]"

// LogEvent matches the canonical ClickHouse log_events row shape.
type LogEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	Service   string            `json:"service"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	TenantID  string            `json:"tenant_id"`
	TraceID   string            `json:"trace_id"`
	WorkerID  string            `json:"worker_id"`
	ErrorCode string            `json:"error_code"`
	Extra     map[string]string `json:"extra"`
}

// LogEventRecorder accepts log rows without blocking the caller.
type LogEventRecorder interface {
	Enqueue(event LogEvent)
}

// NewHandler builds the JSON handler used by New, writing to w. Exposed
// separately so tests can assert on the emitted JSON without writing to
// stdout.
func NewHandler(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: renameTimestamp})
}

// New builds a JSON slog.Logger for service, writing to stdout.
func New(service string) *slog.Logger {
	return slog.New(NewHandler(os.Stdout)).With("service", service)
}

// NewTeeHandler mirrors structured logs to ClickHouse while preserving the
// existing stdout JSON handler.
func NewTeeHandler(next slog.Handler, recorder LogEventRecorder) slog.Handler {
	return teeHandler{next: next, recorder: recorder}
}

// renameTimestamp renames slog's default "time" key to "timestamp" to match
// the field name required by docs/planning/23-observability.md.
func renameTimestamp(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		a.Key = "timestamp"
	}

	return a
}

type teeHandler struct {
	next     slog.Handler
	recorder LogEventRecorder
	attrs    []slog.Attr
	groups   []string
}

func (h teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h teeHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.next.Handle(ctx, r)
	if h.recorder == nil {
		if err != nil {
			return fmt.Errorf("handle log record: %w", err)
		}

		return nil
	}

	fields := make(map[string]string)
	for _, attr := range h.attrs {
		addLogAttr(fields, attr)
	}

	r.Attrs(func(attr slog.Attr) bool {
		addLogAttr(fields, attr)

		return true
	})

	event := LogEvent{
		Timestamp: fieldTime(fields, "timestamp", r.Time),
		Service:   takeField(fields, "service"),
		Level:     r.Level.String(),
		Message:   r.Message,
		RequestID: takeField(fields, "request_id"),
		TenantID:  takeField(fields, "tenant_id"),
		TraceID:   takeField(fields, "trace_id"),
		WorkerID:  takeField(fields, "worker_id"),
		ErrorCode: takeField(fields, "error_code"),
		Extra:     fields,
	}

	h.recorder.Enqueue(event)

	if err != nil {
		return fmt.Errorf("handle log record: %w", err)
	}

	return nil
}

func (h teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.next.WithAttrs(attrs)
	copied := append([]slog.Attr(nil), h.attrs...)
	copied = append(copied, attrs...)

	return teeHandler{next: next, recorder: h.recorder, attrs: copied, groups: h.groups}
}

func (h teeHandler) WithGroup(name string) slog.Handler {
	next := h.next.WithGroup(name)
	groups := append([]string(nil), h.groups...)
	groups = append(groups, name)

	return teeHandler{next: next, recorder: h.recorder, attrs: h.attrs, groups: groups}
}

func addLogAttr(fields map[string]string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Key == "" {
		return
	}

	if isSensitiveLogKey(attr.Key) {
		fields[attr.Key] = redactedValue

		return
	}

	value := attr.Value.String()
	if isSensitiveLogValue(value) {
		value = redactedValue
	}

	fields[attr.Key] = value
}

func takeField(fields map[string]string, key string) string {
	value := fields[key]
	delete(fields, key)

	return value
}

func fieldTime(fields map[string]string, key string, fallback time.Time) time.Time {
	delete(fields, key)

	if fallback.IsZero() {
		return time.Now().UTC()
	}

	return fallback.UTC()
}

func isSensitiveLogKey(key string) bool {
	lower := strings.ToLower(key)

	for _, token := range []string{"authorization", "cookie", "secret", "password", "private_key", "api_key", "credential", "token"} {
		if strings.Contains(lower, token) {
			return true
		}
	}

	return false
}

func isSensitiveLogValue(value string) bool {
	lower := strings.ToLower(value)

	if strings.Contains(lower, "private key") || strings.Contains(lower, "straw.v1.") || strings.Contains(lower, "_inbox.") {
		return true
	}

	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	if u.User != nil {
		return true
	}

	for key := range u.Query() {
		switch strings.ToLower(key) {
		case "signature", "x-amz-signature", "x-amz-credential", "x-goog-signature", "x-goog-credential":
			return true
		}
	}

	return false
}
