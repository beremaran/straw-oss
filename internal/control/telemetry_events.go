package control

import (
	"context"
	"strings"
	"sync"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/logging"
)

const (
	workerEventsTable      = "worker_events"
	configAuditEventsTable = "config_audit_events"
	logEventsTable         = "log_events"

	workerEventRegister      = "register"
	workerEventHeartbeat     = "heartbeat"
	workerEventDisable       = "disable"
	workerEventEnable        = "enable"
	workerEventDrain         = "drain"
	workerEventUndrain       = "undrain"
	workerEventTenantDisable = "tenant_disable"
	workerEventTenantEnable  = "tenant_enable"
	workerEventTenantDrain   = "tenant_drain"
	workerEventTenantUndrain = "tenant_undrain"
)

// asyncEventQueue is a bounded, batch-flushing async writer. It backs every
// ClickHouse event sink so the drop-oldest
// batching and outage-tolerance behavior (docs/planning/22: "Request
// transport does not block on ClickHouse") is defined once instead of once
// per table.
type asyncEventQueue[T any] struct {
	write         func(ctx context.Context, batch []T) error
	maxEntries    int
	batchSize     int
	flushInterval time.Duration

	mu      sync.Mutex
	queue   []T
	metrics *Metrics

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// LogEventSink writes log_events batches to ClickHouse.
type LogEventSink interface {
	WriteLogEvents(ctx context.Context, events []logging.LogEvent) error
}

// LogEventWriter buffers log_events and flushes them to ClickHouse
// asynchronously, using the same bounded queue as the other ClickHouse rows.
type LogEventWriter struct {
	q *asyncEventQueue[logging.LogEvent]
}

// NewLogEventWriter creates an async buffered writer for log_events.
func NewLogEventWriter(sink LogEventSink, maxEntries, batchSize int, flushInterval time.Duration) *LogEventWriter {
	var write func(context.Context, []logging.LogEvent) error
	if sink != nil {
		write = sink.WriteLogEvents
	}

	return &LogEventWriter{q: newAsyncEventQueue(write, maxEntries, batchSize, flushInterval)}
}

// SetMetrics attaches the Prometheus metrics recorder. Not concurrency-safe
// with Flush/Enqueue; call immediately after construction.
func (w *LogEventWriter) SetMetrics(m *Metrics) {
	if w == nil {
		return
	}

	w.q.SetMetrics(m)
}

// QueueDepth returns the current buffered event count.
func (w *LogEventWriter) QueueDepth() int {
	if w == nil {
		return 0
	}

	return w.q.QueueDepth()
}

// Enqueue adds a log row to the bounded queue.
func (w *LogEventWriter) Enqueue(event logging.LogEvent) {
	if w == nil {
		return
	}

	w.q.Enqueue(event)
}

// Flush writes queued events in batches.
func (w *LogEventWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}

	return w.q.Flush(ctx)
}

// Close stops the background flush loop and drains the queue once.
func (w *LogEventWriter) Close() {
	if w == nil {
		return
	}

	w.q.Close()
}

func newAsyncEventQueue[T any](write func(context.Context, []T) error, maxEntries, batchSize int, flushInterval time.Duration) *asyncEventQueue[T] {
	if maxEntries <= 0 {
		maxEntries = 1
	}

	if batchSize <= 0 {
		batchSize = 1
	}

	if batchSize > maxEntries {
		batchSize = maxEntries
	}

	if flushInterval <= 0 {
		flushInterval = time.Second
	}

	q := &asyncEventQueue[T]{
		write:         write,
		maxEntries:    maxEntries,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}

	if write != nil {
		go q.run()
	}

	return q
}

func (q *asyncEventQueue[T]) SetMetrics(m *Metrics) {
	if q == nil {
		return
	}

	q.metrics = m
}

func (q *asyncEventQueue[T]) QueueDepth() int {
	if q == nil {
		return 0
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.queue)
}

func (q *asyncEventQueue[T]) Enqueue(event T) {
	if q == nil || q.write == nil {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) >= q.maxEntries {
		copy(q.queue, q.queue[1:])

		var zero T

		q.queue[len(q.queue)-1] = zero
		q.queue = q.queue[:len(q.queue)-1]
	}

	q.queue = append(q.queue, event)
}

func (q *asyncEventQueue[T]) Flush(ctx context.Context) error {
	if q == nil || q.write == nil {
		return nil
	}

	for {
		batch := q.nextBatch()
		if len(batch) == 0 {
			return nil
		}

		err := q.write(ctx, batch)
		if err != nil {
			q.requeueFront(batch)
			q.metrics.IncClickHouseWriteError()

			return err
		}
	}
}

func (q *asyncEventQueue[T]) Close() {
	if q == nil || q.write == nil {
		return
	}

	q.stopOnce.Do(func() {
		close(q.stopCh)
	})

	<-q.doneCh
}

func (q *asyncEventQueue[T]) run() {
	defer close(q.doneCh)

	ticker := time.NewTicker(q.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = q.Flush(context.Background())
		case <-q.stopCh:
			_ = q.Flush(context.Background())

			return
		}
	}
}

func (q *asyncEventQueue[T]) nextBatch() []T {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) == 0 {
		return nil
	}

	if len(q.queue) < q.batchSize {
		batch := append([]T(nil), q.queue...)
		q.queue = q.queue[:0]

		return batch
	}

	batch := append([]T(nil), q.queue[:q.batchSize]...)
	q.queue = append([]T(nil), q.queue[q.batchSize:]...)

	return batch
}

func (q *asyncEventQueue[T]) requeueFront(batch []T) {
	if len(batch) == 0 {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if len(batch) >= q.maxEntries {
		q.queue = append([]T(nil), batch[:q.maxEntries]...)

		return
	}

	space := q.maxEntries - len(batch)
	merged := make([]T, 0, q.maxEntries)
	merged = append(merged, batch...)

	if len(q.queue) > space {
		merged = append(merged, q.queue[:space]...)
	} else {
		merged = append(merged, q.queue...)
	}

	q.queue = merged
}

// ---- worker_events ----

// WorkerEvent matches the canonical ClickHouse worker_events row shape
// (docs/planning/22).
type WorkerEvent struct {
	Timestamp         time.Time `json:"timestamp"`
	TenantID          string    `json:"tenant_id"`
	WorkerID          string    `json:"worker_id"`
	SessionID         string    `json:"session_id"`
	ExecutorType      string    `json:"executor_type"`
	EventType         string    `json:"event_type"`
	Health            string    `json:"health"`
	ActiveRequests    uint32    `json:"active_requests"`
	MaxConcurrency    uint32    `json:"max_concurrency"`
	AvailableCapacity uint32    `json:"available_capacity"`
	Draining          uint8     `json:"draining"`
	Reason            string    `json:"reason"`
}

// WorkerEventSink writes worker_events batches to ClickHouse.
type WorkerEventSink interface {
	WriteWorkerEvents(ctx context.Context, events []WorkerEvent) error
}

// WorkerEventRecorder accepts worker state transitions without blocking the
// registration/heartbeat path.
type WorkerEventRecorder interface {
	Enqueue(event WorkerEvent)
}

// WorkerEventWriter buffers worker_events and flushes them to ClickHouse
// asynchronously, matching RequestMetadataWriter's outage/bounded-queue
// behavior.
type WorkerEventWriter struct {
	q *asyncEventQueue[WorkerEvent]
}

// NewWorkerEventWriter creates an async buffered writer for worker_events.
func NewWorkerEventWriter(sink WorkerEventSink, maxEntries, batchSize int, flushInterval time.Duration) *WorkerEventWriter {
	var write func(context.Context, []WorkerEvent) error
	if sink != nil {
		write = sink.WriteWorkerEvents
	}

	return &WorkerEventWriter{q: newAsyncEventQueue(write, maxEntries, batchSize, flushInterval)}
}

// SetMetrics attaches the Prometheus metrics recorder. Not concurrency-safe
// with Flush/Enqueue; call immediately after construction.
func (w *WorkerEventWriter) SetMetrics(m *Metrics) {
	if w == nil {
		return
	}

	w.q.SetMetrics(m)
}

// QueueDepth returns the current buffered event count.
func (w *WorkerEventWriter) QueueDepth() int {
	if w == nil {
		return 0
	}

	return w.q.QueueDepth()
}

// Enqueue adds a worker transition to the bounded queue.
func (w *WorkerEventWriter) Enqueue(event WorkerEvent) {
	if w == nil {
		return
	}

	w.q.Enqueue(event)
}

// Flush writes queued events in batches.
func (w *WorkerEventWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}

	return w.q.Flush(ctx)
}

// Close stops the background flush loop and drains the queue once.
func (w *WorkerEventWriter) Close() {
	if w == nil {
		return
	}

	w.q.Close()
}

func workerHealthLabel(h strawpb.WorkerHealth) string {
	name := strings.TrimPrefix(h.String(), "WORKER_HEALTH_")

	return strings.ToLower(name)
}

func drainingFlag(draining bool) uint8 {
	if draining {
		return 1
	}

	return 0
}

// ---- config_audit_events ----

// ConfigAuditEvent matches the canonical ClickHouse config_audit_events row
// shape (docs/planning/22). Old/NewValueJSON must already be redacted by the
// caller (docs/planning/27 secret classification); this writer never
// redacts, it only transports.
type ConfigAuditEvent struct {
	Timestamp     time.Time `json:"timestamp"`
	TenantID      string    `json:"tenant_id"`
	ActorType     string    `json:"actor_type"`
	ActorID       string    `json:"actor_id"`
	ConfigType    string    `json:"config_type"`
	ResourceID    string    `json:"resource_id"`
	Action        string    `json:"action"`
	ConfigVersion uint64    `json:"config_version"`
	FieldPath     string    `json:"field_path"`
	OldValueJSON  string    `json:"old_value_json"`
	NewValueJSON  string    `json:"new_value_json"`
}

// ConfigAuditEventSink writes config_audit_events batches to ClickHouse.
type ConfigAuditEventSink interface {
	WriteConfigAuditEvents(ctx context.Context, events []ConfigAuditEvent) error
}

// ConfigAuditRecorder accepts config audit rows without blocking the config
// write path.
type ConfigAuditRecorder interface {
	Enqueue(event ConfigAuditEvent)
}

// ConfigAuditEventWriter buffers config_audit_events and flushes them to
// ClickHouse asynchronously, mirroring the Postgres config_audit_source
// writes those events shadow.
type ConfigAuditEventWriter struct {
	q *asyncEventQueue[ConfigAuditEvent]
}

// NewConfigAuditEventWriter creates an async buffered writer for
// config_audit_events.
func NewConfigAuditEventWriter(sink ConfigAuditEventSink, maxEntries, batchSize int, flushInterval time.Duration) *ConfigAuditEventWriter {
	var write func(context.Context, []ConfigAuditEvent) error
	if sink != nil {
		write = sink.WriteConfigAuditEvents
	}

	return &ConfigAuditEventWriter{q: newAsyncEventQueue(write, maxEntries, batchSize, flushInterval)}
}

// SetMetrics attaches the Prometheus metrics recorder. Not concurrency-safe
// with Flush/Enqueue; call immediately after construction.
func (w *ConfigAuditEventWriter) SetMetrics(m *Metrics) {
	if w == nil {
		return
	}

	w.q.SetMetrics(m)
}

// QueueDepth returns the current buffered event count.
func (w *ConfigAuditEventWriter) QueueDepth() int {
	if w == nil {
		return 0
	}

	return w.q.QueueDepth()
}

// Enqueue adds a config audit row to the bounded queue.
func (w *ConfigAuditEventWriter) Enqueue(event ConfigAuditEvent) {
	if w == nil {
		return
	}

	w.q.Enqueue(event)
}

// Flush writes queued events in batches.
func (w *ConfigAuditEventWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}

	return w.q.Flush(ctx)
}

// Close stops the background flush loop and drains the queue once.
func (w *ConfigAuditEventWriter) Close() {
	if w == nil {
		return
	}

	w.q.Close()
}
