// Package logging builds the structured JSON logger shared by Straw services.
package logging

import (
	"io"
	"log/slog"
	"os"
)

// NewHandler builds Straw's JSON log handler, writing to w.
func NewHandler(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: renameTimestamp})
}

// New builds a JSON logger for service, writing to stdout.
func New(service string) *slog.Logger {
	return slog.New(NewHandler(os.Stdout)).With("service", service)
}

func renameTimestamp(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.TimeKey {
		attr.Key = "timestamp"
	}

	return attr
}
