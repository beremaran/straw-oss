package endpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/beremaran/straw/internal/observability/logging"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const bypassForwardingKey contextKey = "bypass_forwarding"

// ForwardingHandler wraps a base slog.Handler and publishes log entries to NATS
// when enabled and a broker connection is available.
type ForwardingHandler struct {
	slog.Handler
	endpointID string
	mu         *sync.RWMutex
	brokerRef  *broker.MessageBroker
	enabled    bool
	attrs      map[string]any
	group      string
}

func (h *ForwardingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}

func (h *ForwardingHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.Handler.Handle(ctx, record)

	if !h.enabled {
		return err
	}

	h.mu.RLock()
	br := *h.brokerRef
	h.mu.RUnlock()

	if br == nil {
		return err
	}

	if ctx.Value(bypassForwardingKey) != nil {
		return err
	}

	bypassCtx := context.WithValue(ctx, bypassForwardingKey, true)
	attrs, traceID, requestID := h.extractLogMetadata(ctx, record)

	entry := protocol.LogEntry{
		EndpointID: h.endpointID,
		ObservedAt: record.Time,
		Level:      record.Level.String(),
		Message:    record.Message,
		Attrs:      attrs,
		TraceID:    traceID,
		RequestID:  requestID,
	}

	data, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		return err
	}

	subject := fmt.Sprintf("endpoint.logs.%s", h.endpointID)
	_ = br.Publish(bypassCtx, subject, data)

	return err
}

func (h *ForwardingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make(map[string]any, len(h.attrs)+len(attrs))
	for k, v := range h.attrs {
		newAttrs[k] = v
	}
	for _, attr := range attrs {
		key := attr.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		newAttrs[key] = attr.Value.Any()
	}

	return &ForwardingHandler{
		Handler:    h.Handler.WithAttrs(attrs),
		endpointID: h.endpointID,
		mu:         h.mu,
		brokerRef:  h.brokerRef,
		enabled:    h.enabled,
		attrs:      newAttrs,
		group:      h.group,
	}
}

func (h *ForwardingHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}

	return &ForwardingHandler{
		Handler:    h.Handler.WithGroup(name),
		endpointID: h.endpointID,
		mu:         h.mu,
		brokerRef:  h.brokerRef,
		enabled:    h.enabled,
		attrs:      h.attrs,
		group:      newGroup,
	}
}

func (h *ForwardingHandler) extractLogMetadata(ctx context.Context, record slog.Record) (map[string]any, string, string) {
	attrs := make(map[string]any, len(h.attrs)+record.NumAttrs())
	for k, v := range h.attrs {
		attrs[k] = v
	}

	record.Attrs(func(attr slog.Attr) bool {
		key := attr.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		attrs[key] = attr.Value.Any()

		return true
	})

	var traceID, requestID string
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID = span.SpanContext().TraceID().String()
	}

	if reqID, ok := ctx.Value(logging.RequestIDKey).(string); ok {
		requestID = reqID
	}

	return attrs, traceID, requestID
}
