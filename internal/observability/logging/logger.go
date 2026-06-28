package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

type Config struct {
	Level   string
	Format  string
	Service string
	Version string
}

func SetupLogger(cfg Config) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Level),
	}

	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	handler = &TraceHandler{Handler: handler}

	logger := slog.New(handler)

	logger = logger.With(
		slog.String("service", cfg.Service),
		slog.String("version", cfg.Version),
	)

	slog.SetDefault(logger)

	return logger
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type TraceHandler struct {
	slog.Handler
}

func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID := span.SpanContext().TraceID().String()
		spanID := span.SpanContext().SpanID().String()
		r.AddAttrs(
			slog.String("trace_id", traceID),
			slog.String("span_id", spanID),
		)
	}
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}

	return h.Handler.Handle(ctx, r)
}
