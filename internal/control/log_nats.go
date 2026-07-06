package control

import (
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/logging"
	"github.com/beremaran/straw/v2/internal/natsx"
)

// SetupLogEventSubscription wires transient Egress log telemetry into the
// Control-owned ClickHouse log event writer.
func SetupLogEventSubscription(conn *natsx.Connection, writer *LogEventWriter) error {
	if conn == nil {
		return fmt.Errorf("setup log events: %w", errSetupConnRequired)
	}

	if writer == nil {
		return nil
	}

	_, err := conn.QueueSubscribe(natsx.LogTelemetrySubject(), "control", func(msg *nats.Msg) {
		handleLogEvent(writer, msg)
	})
	if err != nil {
		return fmt.Errorf("subscribe log events: %w", err)
	}

	err = conn.Flush()
	if err != nil {
		return fmt.Errorf("flush log event subscription: %w", err)
	}

	return nil
}

func handleLogEvent(writer *LogEventWriter, msg *nats.Msg) {
	env, err := natsx.UnmarshalEnvelope(msg.Data)
	if err != nil {
		slog.Warn("invalid log event envelope", "error", err)

		return
	}

	event := env.GetLogEvent()
	if event == nil {
		return
	}

	writer.Enqueue(logEventFromProto(env, event))
}

func logEventFromProto(env *strawpb.Envelope, event *strawpb.LogEvent) logging.LogEvent {
	timestamp := time.UnixMilli(event.GetTimestampUnixMs()).UTC()
	if event.GetTimestampUnixMs() == 0 {
		timestamp = time.Now().UTC()
	}

	return logging.LogEvent{
		Timestamp: timestamp,
		Service:   event.GetService(),
		Level:     event.GetLevel(),
		Message:   event.GetMessage(),
		RequestID: firstNonEmpty(event.GetRequestId(), env.GetRequestId()),
		TenantID:  firstNonEmpty(event.GetTenantId(), env.GetTenantId()),
		TraceID:   firstNonEmpty(event.GetTraceId(), env.GetTraceId()),
		WorkerID:  event.GetWorkerId(),
		ErrorCode: event.GetErrorCode(),
		Extra:     cloneLogExtra(event.GetExtra()),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func cloneLogExtra(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]string, len(in))
	maps.Copy(out, in)

	return out
}
