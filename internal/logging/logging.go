// Package logging builds the structured JSON slog logger shared by the
// control and egress binaries, per docs/planning/23-observability.md: every
// record carries service, timestamp, and level; request_id, tenant_id,
// error_code, and worker_id are attached by call sites where available.
package logging

import (
	"io"
	"log/slog"
	"os"
)

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

// renameTimestamp renames slog's default "time" key to "timestamp" to match
// the field name required by docs/planning/23-observability.md.
func renameTimestamp(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		a.Key = "timestamp"
	}

	return a
}
