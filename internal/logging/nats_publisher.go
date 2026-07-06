package logging

import (
	"maps"
	"sync"
	"sync/atomic"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/natsx"
)

// DefaultNATSLogQueueEntries is the bounded Egress log telemetry queue size.
const DefaultNATSLogQueueEntries = 1024

// NATSLogPublisher publishes already-redacted slog records to Control without
// blocking the logging caller.
type NATSLogPublisher struct {
	conn    natsConn
	subject string
	ch      chan LogEvent
	done    chan struct{}
	once    sync.Once
	dropped atomic.Uint64
}

type natsConn interface {
	Publish(subject string, data []byte) error
}

// NewNATSLogPublisher builds a bounded, non-blocking NATS log recorder.
func NewNATSLogPublisher(conn natsConn, subject string, maxEntries int) *NATSLogPublisher {
	if conn == nil {
		return nil
	}

	if subject == "" {
		subject = natsx.LogTelemetrySubject()
	}

	if maxEntries <= 0 {
		maxEntries = DefaultNATSLogQueueEntries
	}

	p := &NATSLogPublisher{
		conn:    conn,
		subject: subject,
		ch:      make(chan LogEvent, maxEntries),
		done:    make(chan struct{}),
	}

	go p.run()

	return p
}

// Enqueue adds a log event or drops the oldest queued event when full.
func (p *NATSLogPublisher) Enqueue(event LogEvent) {
	if p == nil {
		return
	}

	select {
	case p.ch <- event:
	default:
		select {
		case <-p.ch:
		default:
		}

		p.dropped.Add(1)

		select {
		case p.ch <- event:
		default:
			p.dropped.Add(1)
		}
	}
}

// Dropped returns the number of events dropped before successful publish.
func (p *NATSLogPublisher) Dropped() uint64 {
	if p == nil {
		return 0
	}

	return p.dropped.Load()
}

// Close drains queued log events and stops the publisher goroutine.
func (p *NATSLogPublisher) Close() {
	if p == nil {
		return
	}

	p.once.Do(func() {
		close(p.ch)
		<-p.done
	})
}

func (p *NATSLogPublisher) run() {
	defer close(p.done)

	for event := range p.ch {
		raw, err := natsx.MarshalEnvelope(&strawpb.Envelope{
			RequestId:     event.RequestID,
			TenantId:      event.TenantID,
			TraceId:       event.TraceID,
			ProtocolMajor: 1,
			Payload:       &strawpb.Envelope_LogEvent{LogEvent: logEventToProto(event)},
		})
		if err != nil {
			p.dropped.Add(1)

			continue
		}

		err = p.conn.Publish(p.subject, raw)
		if err != nil {
			p.dropped.Add(1)
		}
	}
}

func logEventToProto(event LogEvent) *strawpb.LogEvent {
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	return &strawpb.LogEvent{
		TimestampUnixMs: timestamp.UTC().UnixMilli(),
		Service:         event.Service,
		Level:           event.Level,
		Message:         event.Message,
		RequestId:       event.RequestID,
		TenantId:        event.TenantID,
		TraceId:         event.TraceID,
		WorkerId:        event.WorkerID,
		ErrorCode:       event.ErrorCode,
		Extra:           cloneExtra(event.Extra),
	}
}

func cloneExtra(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]string, len(in))
	maps.Copy(out, in)

	return out
}

var _ LogEventRecorder = (*NATSLogPublisher)(nil)
