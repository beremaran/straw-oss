package control

import (
	"context"
	"fmt"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/objectstore"
)

const payloadCaptureEventsTable = "payload_capture_events"

// PayloadCaptureEvent matches the canonical ClickHouse payload_capture_events
// row shape (docs/planning/22). Bodies are never stored inline: captured bodies
// live in object storage and only their reference key is written here, so a
// large body can never bloat a ClickHouse row (docs/planning/19 Storage). The
// header strings and redacted_fields already come from the capture engine's
// redacted output; this writer transports, it never un-redacts.
type PayloadCaptureEvent struct {
	CapturedAt      time.Time `json:"captured_at"`
	RequestID       string    `json:"request_id"`
	TenantID        string    `json:"tenant_id"`
	CaptureScope    string    `json:"capture_scope"`
	CaptureDecision string    `json:"capture_decision"`
	RequestHeaders  string    `json:"request_headers"`
	ResponseHeaders string    `json:"response_headers"`
	RequestBodyRef  string    `json:"request_body_ref"`
	ResponseBodyRef string    `json:"response_body_ref"`
	RedactedFields  []string  `json:"redacted_fields"`
	Truncated       uint8     `json:"truncated"`
}

// PayloadCaptureEventSink writes payload_capture_events batches to ClickHouse.
type PayloadCaptureEventSink interface {
	WritePayloadCaptureEvents(ctx context.Context, events []PayloadCaptureEvent) error
}

// WritePayloadCaptureEvents posts JSONEachRow records to ClickHouse's
// payload_capture_events table (docs/planning/22).
func (s *HTTPClickHouseSink) WritePayloadCaptureEvents(ctx context.Context, events []PayloadCaptureEvent) error {
	return insertClickHouseRows(ctx, s, payloadCaptureEventsTable, events)
}

// PayloadCaptureRecorder accepts capture rows without blocking the transport
// path. PayloadCaptureEventWriter is the production implementation.
type PayloadCaptureRecorder interface {
	Enqueue(event PayloadCaptureEvent)
}

// PayloadCaptureEventWriter buffers payload_capture_events and flushes them to
// ClickHouse asynchronously, using the same bounded, outage-tolerant queue as
// the other ClickHouse rows (docs/planning/22: transport never blocks on
// ClickHouse).
type PayloadCaptureEventWriter struct {
	q *asyncEventQueue[PayloadCaptureEvent]
}

// NewPayloadCaptureEventWriter creates an async buffered writer for
// payload_capture_events.
func NewPayloadCaptureEventWriter(sink PayloadCaptureEventSink, maxEntries, batchSize int, flushInterval time.Duration) *PayloadCaptureEventWriter {
	var write func(context.Context, []PayloadCaptureEvent) error
	if sink != nil {
		write = sink.WritePayloadCaptureEvents
	}

	return &PayloadCaptureEventWriter{q: newAsyncEventQueue(write, maxEntries, batchSize, flushInterval)}
}

// SetMetrics attaches the Prometheus metrics recorder. Not concurrency-safe
// with Flush/Enqueue; call immediately after construction.
func (w *PayloadCaptureEventWriter) SetMetrics(m *Metrics) {
	if w == nil {
		return
	}

	w.q.SetMetrics(m)
}

// QueueDepth returns the current buffered event count.
func (w *PayloadCaptureEventWriter) QueueDepth() int {
	if w == nil {
		return 0
	}

	return w.q.QueueDepth()
}

// Enqueue adds a capture row to the bounded queue.
func (w *PayloadCaptureEventWriter) Enqueue(event PayloadCaptureEvent) {
	if w == nil {
		return
	}

	w.q.Enqueue(event)
}

// Flush writes queued events in batches.
func (w *PayloadCaptureEventWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}

	return w.q.Flush(ctx)
}

// Close stops the background flush loop and drains the queue once.
func (w *PayloadCaptureEventWriter) Close() {
	if w == nil {
		return
	}

	w.q.Close()
}

// CaptureBodyStore uploads captured bodies to object storage by reference and
// deletes them on cleanup. S3RequestBodyRefStore satisfies it; only the object
// key from the returned frame is retained as the ClickHouse body reference.
type CaptureBodyStore interface {
	UploadRequestBody(ctx context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error)
	UploadResponseBody(ctx context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error)
	DeleteRequestBody(ctx context.Context, frame *strawpb.BodyRefFrame) error
	DeleteResponseBody(ctx context.Context, frame *strawpb.BodyRefFrame) error
}

// PayloadCaptureMeta identifies the request a capture belongs to. CaptureScope
// is the canonical payload_capture_events.capture_scope column (the scope at
// which capture was authorized); it is carried verbatim.
type PayloadCaptureMeta struct {
	TenantID     string
	RequestID    string
	CaptureScope string
}

// PayloadCaptureStore persists one capture result: it stores captured bodies in
// object storage by reference (never inline in ClickHouse) and enqueues the
// metadata row. Body upload is synchronous so a storage outage surfaces to the
// caller and no row is written pointing at a missing object; on a partial
// failure any already-uploaded body is deleted so no object is orphaned.
type PayloadCaptureStore struct {
	bodies   CaptureBodyStore
	recorder PayloadCaptureRecorder
}

// NewPayloadCaptureStore builds a store over an object-storage body writer and
// a ClickHouse capture recorder.
func NewPayloadCaptureStore(bodies CaptureBodyStore, recorder PayloadCaptureRecorder) *PayloadCaptureStore {
	return &PayloadCaptureStore{bodies: bodies, recorder: recorder}
}

// Store persists result for the request in meta. Bodies present in result are
// uploaded to object storage and only their keys are recorded; headers,
// redacted-field names, the decision, and the truncation flag land in the
// ClickHouse row. It returns an object-storage error (wrapping
// objectstore.ErrUnavailable) without enqueuing a row when a body upload fails.
func (s *PayloadCaptureStore) Store(ctx context.Context, meta PayloadCaptureMeta, result CaptureResult) error {
	return s.StoreWithRefs(ctx, meta, result, "", "")
}

// StoreWithRefs persists result while reusing already-created body object keys.
func (s *PayloadCaptureStore) StoreWithRefs(ctx context.Context, meta PayloadCaptureMeta, result CaptureResult, requestBodyRef, responseBodyRef string) error {
	if s == nil {
		return nil
	}

	if (len(result.RequestBody) > 0 || len(result.ResponseBody) > 0) && s.bodies == nil {
		return fmt.Errorf("store capture body: %w", objectstore.ErrUnavailable)
	}

	event := PayloadCaptureEvent{
		CapturedAt:      result.CapturedAt,
		RequestID:       meta.RequestID,
		TenantID:        meta.TenantID,
		CaptureScope:    meta.CaptureScope,
		CaptureDecision: string(result.Decision),
		RequestHeaders:  result.RequestHeaders,
		ResponseHeaders: result.ResponseHeaders,
		RedactedFields:  result.RedactedFields,
		Truncated:       boolToUint8(result.Truncated),
	}

	// ClickHouse Array(String) via JSONEachRow rejects a JSON null, so a
	// no-redaction capture must serialize as [] rather than the nil slice's null.
	if event.RedactedFields == nil {
		event.RedactedFields = []string{}
	}

	bodyRefs, err := s.storeBodies(ctx, meta, result)
	if err != nil {
		return err
	}

	event.RequestBodyRef = firstNonEmpty(requestBodyRef, bodyRefs.request)
	event.ResponseBodyRef = firstNonEmpty(responseBodyRef, bodyRefs.response)

	if s.recorder != nil {
		s.recorder.Enqueue(event)
	}

	return nil
}

type payloadCaptureBodyRefs struct {
	request  string
	response string
}

func (s *PayloadCaptureStore) storeBodies(ctx context.Context, meta PayloadCaptureMeta, result CaptureResult) (payloadCaptureBodyRefs, error) {
	var refs payloadCaptureBodyRefs

	var reqFrame *strawpb.BodyRefFrame

	if len(result.RequestBody) > 0 {
		frame, err := s.bodies.UploadRequestBody(ctx, meta.TenantID, meta.RequestID, result.RequestBody)
		if err != nil {
			return refs, fmt.Errorf("store capture request body: %w", err)
		}

		reqFrame = frame
		refs.request = frame.GetS3().GetObjectKey()
	}

	if len(result.ResponseBody) == 0 {
		return refs, nil
	}

	frame, err := s.bodies.UploadResponseBody(ctx, meta.TenantID, meta.RequestID, result.ResponseBody)
	if err != nil {
		// The request body already landed; delete it so the failed capture
		// leaves no orphaned object behind.
		if reqFrame != nil {
			_ = s.bodies.DeleteRequestBody(ctx, reqFrame)
		}

		return refs, fmt.Errorf("store capture response body: %w", err)
	}

	refs.response = frame.GetS3().GetObjectKey()

	return refs, nil
}

var _ CaptureBodyStore = (*S3RequestBodyRefStore)(nil)

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}

	return 0
}
