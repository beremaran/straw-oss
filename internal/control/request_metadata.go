package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/beremaran/straw/v2/internal/logging"
)

const (
	requestMetadataNone         = "none"
	requestMetadataRedacted     = "[redacted]"
	requestMetadataDefaultTable = "request_events"
	requestMetadataDefaultDB    = "straw"
	requestMetadataHashPrefix   = "sha256:"
)

var errClickHouseInsertFailed = errors.New("clickhouse insert returned non-2xx status")

// RequestEvent matches the canonical ClickHouse request_events row shape.
type RequestEvent struct {
	Timestamp                   time.Time `json:"timestamp"`
	RequestID                   string    `json:"request_id"`
	TraceID                     string    `json:"trace_id"`
	TenantID                    string    `json:"tenant_id"`
	APIKeyID                    string    `json:"api_key_id"`
	IngressType                 string    `json:"ingress_type"`
	Method                      string    `json:"method"`
	TargetHost                  string    `json:"target_host"`
	TargetURL                   string    `json:"target_url"`
	RouteID                     string    `json:"route_id"`
	PoolID                      string    `json:"pool_id"`
	ExecutorType                string    `json:"executor_type"`
	SelectedExecutor            string    `json:"selected_executor"`
	RequestedFingerprintProfile string    `json:"requested_fingerprint_profile"`
	SelectedFingerprintProfile  string    `json:"selected_fingerprint_profile"`
	ExecutedFingerprintProfile  string    `json:"executed_fingerprint_profile"`
	Country                     string    `json:"country"`
	Region                      string    `json:"region"`
	IPType                      string    `json:"ip_type"`
	Tags                        []string  `json:"tags"`
	Attempt                     uint8     `json:"attempt"`
	UpstreamStatus              uint16    `json:"upstream_status"`
	ClientStatus                uint16    `json:"client_status"`
	ErrorCode                   string    `json:"error_code"`
	ErrorCategory               string    `json:"error_category"`
	TimeoutType                 string    `json:"timeout_type"`
	RequestSizeBytes            uint64    `json:"request_size_bytes"`
	ResponseSizeBytes           uint64    `json:"response_size_bytes"`
	RoutingMS                   uint32    `json:"routing_ms"`
	AssignmentMS                uint32    `json:"assignment_ms"`
	EgressMS                    uint32    `json:"egress_ms"`
	TotalMS                     uint32    `json:"total_ms"`
	CaptureDecision             string    `json:"capture_decision"`
}

// RequestMetadataRecorder accepts accepted requests without blocking the
// transport path.
type RequestMetadataRecorder interface {
	Enqueue(event RequestEvent)
}

// RequestMetadataWriter buffers request metadata and flushes it to ClickHouse
// asynchronously. If the sink is unavailable, the writer keeps the queue
// bounded and retries later; transport never waits on the sink.
type RequestMetadataWriter struct {
	q *asyncEventQueue[RequestEvent]
}

// RequestEventSink writes request metadata batches to ClickHouse.
type RequestEventSink interface {
	WriteRequestEvents(ctx context.Context, events []RequestEvent) error
}

// NewRequestMetadataWriter creates an async buffered writer.
func NewRequestMetadataWriter(sink RequestEventSink, maxEntries, batchSize int, flushInterval time.Duration) *RequestMetadataWriter {
	var write func(context.Context, []RequestEvent) error
	if sink != nil {
		write = sink.WriteRequestEvents
	}

	return &RequestMetadataWriter{q: newAsyncEventQueue(write, maxEntries, batchSize, flushInterval)}
}

// SetMetrics attaches the Prometheus metrics recorder used for
// straw_clickhouse_write_errors_total (docs/planning/23). It is not
// concurrency-safe with Flush/Enqueue and must be called before the writer
// is shared across goroutines (i.e. immediately after construction).
func (w *RequestMetadataWriter) SetMetrics(m *Metrics) {
	if w == nil {
		return
	}

	w.q.SetMetrics(m)
}

// QueueDepth returns the current buffered event count, for the
// straw_clickhouse_write_queue_depth gauge (docs/planning/23).
func (w *RequestMetadataWriter) QueueDepth() int {
	if w == nil {
		return 0
	}

	return w.q.QueueDepth()
}

// Enqueue adds an accepted request to the bounded queue.
func (w *RequestMetadataWriter) Enqueue(event RequestEvent) {
	if w == nil {
		return
	}

	w.q.Enqueue(event)
}

// Flush writes queued events in batches.
func (w *RequestMetadataWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}

	err := w.q.Flush(ctx)
	if err != nil {
		return fmt.Errorf("write request metadata batch: %w", err)
	}

	return nil
}

// Close stops the background flush loop and drains the queue once.
func (w *RequestMetadataWriter) Close() {
	if w == nil {
		return
	}

	w.q.Close()
}

// TenantPolicy carries the tenant-owned request timeout and metadata policy.
type TenantPolicy struct {
	DefaultTimeoutMs     uint64
	MaxTimeoutMs         uint64
	MetadataQueryStorage MetadataStoragePolicy
	MetadataPathStorage  MetadataStoragePolicy
}

func defaultTenantPolicy() TenantPolicy {
	return TenantPolicy{
		DefaultTimeoutMs:     defaultTenantDefaultTimeoutMs,
		MaxTimeoutMs:         defaultTenantMaxTimeoutMs,
		MetadataQueryStorage: defaultMetadataQueryStorage,
		MetadataPathStorage:  defaultMetadataPathStorage,
	}
}

func (p TenantPolicy) normalized() TenantPolicy {
	t := normalizeTenant(Tenant{
		DefaultTimeoutMs:     p.DefaultTimeoutMs,
		MaxTimeoutMs:         p.MaxTimeoutMs,
		MetadataQueryStorage: p.MetadataQueryStorage,
		MetadataPathStorage:  p.MetadataPathStorage,
	})

	return TenantPolicy{
		DefaultTimeoutMs:     t.DefaultTimeoutMs,
		MaxTimeoutMs:         t.MaxTimeoutMs,
		MetadataQueryStorage: t.MetadataQueryStorage,
		MetadataPathStorage:  t.MetadataPathStorage,
	}
}

func buildRequestEvent(requestID string, identity Identity, request *ValidatedRequest, policy TenantPolicy) RequestEvent {
	var requestSize uint64
	if request != nil {
		requestSize = uint64(len(request.BodyData))
	}

	policy = policy.normalized()

	event := RequestEvent{
		Timestamp:        time.Now().UTC(),
		RequestID:        requestID,
		TenantID:         identity.TenantID,
		APIKeyID:         identity.APIKeyID,
		IngressType:      IngressTypeREST,
		TargetHost:       "",
		TargetURL:        "",
		Attempt:          1,
		CaptureDecision:  requestMetadataNone,
		RequestSizeBytes: requestSize,
	}

	if request != nil {
		event.Method = request.Method

		if request.IngressType != "" {
			event.IngressType = request.IngressType
		}

		if request.CaptureDecision != "" {
			event.CaptureDecision = request.CaptureDecision
		}
	}

	if request != nil && request.URL != nil {
		event.TargetHost = request.URL.Hostname()
		if event.TargetHost == "" {
			event.TargetHost = request.URL.Host
		}

		event.TargetURL = sanitizeTargetURL(request.URL, policy)
	}

	if request != nil {
		event.Country = request.Routing.Country
		event.Region = request.Routing.Region
		event.IPType = request.Routing.IPType
		event.Tags = request.Routing.Tags
	}

	return event
}

// applyRequestOutcome finalizes a pre-built RequestEvent with the real
// dispatch result (docs/tasks/p0/32): on success it fills the actual
// upstream status, sizes, and per-phase timings; on failure it fills the
// canonical error_code/error_category/timeout_type instead of a synthetic
// 200, using whatever partial timing the dispatcher measured before it
// failed.
func applyRequestOutcome(event RequestEvent, resp SuccessResponse, perr *PipelineError) RequestEvent {
	if perr == nil {
		event.UpstreamStatus = uint16OrMax(resp.Status)
		event.ClientStatus = http.StatusOK
		event.ResponseSizeBytes = resp.ResponseSizeBytes
		event.RoutingMS = uint32OrMax(resp.Timing.RoutingMs)
		event.AssignmentMS = uint32OrMax(resp.Timing.AssignmentMs)
		event.EgressMS = uint32OrMax(resp.Timing.EgressMs)
		event.TotalMS = uint32OrMax(resp.Timing.TotalMs)
		event.RouteID = resp.RouteID
		event.PoolID = resp.PoolID
		event.SelectedExecutor = resp.SelectedExecutor
		event.ExecutorType = resp.ExecutorType

		return event
	}

	status := statusInternalServerError
	if entry, ok := ErrorRegistry[perr.Code]; ok {
		status = entry.HTTPStatus
		event.ErrorCode = entry.Code
		event.ErrorCategory = entry.Category
	}

	event.ClientStatus = uint16OrMax(status)
	event.TimeoutType = perr.TimeoutType
	event.RoutingMS = uint32OrMax(perr.RoutingMs)
	event.AssignmentMS = uint32OrMax(perr.AssignmentMs)
	event.EgressMS = uint32OrMax(perr.EgressMs)
	event.TotalMS = uint32OrMax(perr.TotalMs)
	event.RouteID = perr.RouteID
	event.PoolID = perr.PoolID
	event.SelectedExecutor = perr.SelectedExecutor
	event.ExecutorType = perr.ExecutorType

	return event
}

// uint16OrMax converts an int to uint16, clamping to [0, MaxUint16] instead
// of wrapping. HTTP status codes are always well within range; the clamp
// only guards against a negative or malformed value.
func uint16OrMax(v int) uint16 {
	if v < 0 {
		return 0
	}

	if v > math.MaxUint16 {
		return math.MaxUint16
	}

	return uint16(v)
}

// uint32OrMax converts an int64 millisecond duration to uint32, clamping to
// [0, MaxUint32] instead of wrapping.
func uint32OrMax(v int64) uint32 {
	if v < 0 {
		return 0
	}

	if v > math.MaxUint32 {
		return math.MaxUint32
	}

	return uint32(v)
}

func sanitizeTargetURL(u *url.URL, policy TenantPolicy) string {
	if u == nil {
		return ""
	}

	policy = policy.normalized()

	sanitized := *u
	sanitized.User = nil
	sanitized.Fragment = ""
	sanitized.RawQuery = metadataComponent(sanitized.RawQuery, policy.MetadataQueryStorage)

	if sanitized.Path == "" {
		sanitized.Path = "/"
	}

	sanitized.Path = metadataComponent(sanitized.Path, policy.MetadataPathStorage)

	return sanitized.String()
}

func metadataComponent(value string, policy MetadataStoragePolicy) string {
	switch policy {
	case MetadataStorageStore:
		return value
	case MetadataStorageHash:
		if value == "" {
			return ""
		}

		sum := sha256.Sum256([]byte(value))

		return requestMetadataHashPrefix + hex.EncodeToString(sum[:])
	case MetadataStorageDrop:
		return ""
	default:
		return ""
	}
}

func redactSensitiveHeaderValue(name, value string) string {
	if isSensitiveHeaderName(name) {
		return requestMetadataRedacted
	}

	return value
}

func isSensitiveHeaderName(name string) bool {
	lower := strings.ToLower(name)
	if lower == "" {
		return false
	}

	switch lower {
	case headerNameAuthorization, headerNameProxyAuthorization, "cookie", "set-cookie", "x-api-key":
		return true
	}

	if strings.HasPrefix(lower, "x-straw-") {
		return true
	}

	if strings.Contains(lower, "secret") {
		return true
	}

	return false
}

// HTTPClickHouseSink writes request metadata using ClickHouse's HTTP insert
// endpoint.
type HTTPClickHouseSink struct {
	client   *http.Client
	endpoint string
	database string
	table    string
	user     string
	pass     string
}

// NewHTTPClickHouseSink creates a sink for ClickHouse's HTTP interface.
func NewHTTPClickHouseSink(endpoint, database, user, pass string, client *http.Client) *HTTPClickHouseSink {
	if client == nil {
		client = http.DefaultClient
	}

	if database == "" {
		database = requestMetadataDefaultDB
	}

	return &HTTPClickHouseSink{
		client:   client,
		endpoint: endpoint,
		database: database,
		table:    requestMetadataDefaultTable,
		user:     user,
		pass:     pass,
	}
}

// WriteRequestEvents posts JSONEachRow records to ClickHouse's request_events
// table.
func (s *HTTPClickHouseSink) WriteRequestEvents(ctx context.Context, events []RequestEvent) error {
	return insertClickHouseRows(ctx, s, s.table, events)
}

// WriteWorkerEvents posts JSONEachRow records to ClickHouse's worker_events
// table (docs/tasks/p0/32).
func (s *HTTPClickHouseSink) WriteWorkerEvents(ctx context.Context, events []WorkerEvent) error {
	return insertClickHouseRows(ctx, s, workerEventsTable, events)
}

// WriteConfigAuditEvents posts JSONEachRow records to ClickHouse's
// config_audit_events table (docs/tasks/p0/32).
func (s *HTTPClickHouseSink) WriteConfigAuditEvents(ctx context.Context, events []ConfigAuditEvent) error {
	return insertClickHouseRows(ctx, s, configAuditEventsTable, events)
}

// WriteLogEvents posts JSONEachRow records to ClickHouse's log_events table.
func (s *HTTPClickHouseSink) WriteLogEvents(ctx context.Context, events []logging.LogEvent) error {
	return insertClickHouseRows(ctx, s, logEventsTable, events)
}

// insertClickHouseRows encodes rows as newline-delimited JSON and posts them
// to ClickHouse's HTTP insert endpoint for the given table. It is shared by
// every ClickHouse event sink so the request shape (query params, auth,
// status handling) is defined once.
func insertClickHouseRows[T any](ctx context.Context, s *HTTPClickHouseSink, table string, events []T) error {
	if s == nil || len(events) == 0 {
		return nil
	}

	var body bytes.Buffer

	enc := json.NewEncoder(&body)
	for _, event := range events {
		err := enc.Encode(event)
		if err != nil {
			return fmt.Errorf("encode clickhouse row: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, &body)
	if err != nil {
		return fmt.Errorf("build clickhouse request: %w", err)
	}

	query := req.URL.Query()
	query.Set("database", s.database)
	// RequestEvent.Timestamp (and the other event rows) serialize as RFC3339
	// (e.g. 2026-07-04T16:44:11.812Z), which ClickHouse's default DateTime64
	// parser rejects. best_effort accepts the ISO-8601 'T'/'Z' form so inserts
	// don't 400 and get silently dropped by the async writer's outage policy.
	query.Set("date_time_input_format", "best_effort")
	query.Set("query", "INSERT INTO "+table+" FORMAT JSONEachRow")
	req.URL.RawQuery = query.Encode()
	req.Header.Set(headerCanonicalContentType, "application/json")

	if s.user != "" {
		req.SetBasicAuth(s.user, s.pass)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post clickhouse rows: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s", errClickHouseInsertFailed, resp.Status)
	}

	return nil
}
