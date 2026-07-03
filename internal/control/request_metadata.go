package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	requestMetadataIngressType  = "rest"
	requestMetadataNone         = "none"
	requestMetadataRedacted     = "[redacted]"
	requestMetadataDefaultTable = "request_events"
	requestMetadataDefaultDB    = "straw"
)

var errClickHouseInsertFailed = errors.New("clickhouse insert returned non-2xx status")

// RequestEvent matches the canonical ClickHouse request_events row shape.
type RequestEvent struct {
	Timestamp         time.Time `json:"timestamp"`
	RequestID         string    `json:"request_id"`
	TraceID           string    `json:"trace_id"`
	TenantID          string    `json:"tenant_id"`
	APIKeyID          string    `json:"api_key_id"`
	IngressType       string    `json:"ingress_type"`
	Method            string    `json:"method"`
	TargetHost        string    `json:"target_host"`
	TargetURL         string    `json:"target_url"`
	RouteID           string    `json:"route_id"`
	PoolID            string    `json:"pool_id"`
	ExecutorType      string    `json:"executor_type"`
	SelectedExecutor  string    `json:"selected_executor"`
	Country           string    `json:"country"`
	Region            string    `json:"region"`
	IPType            string    `json:"ip_type"`
	Tags              []string  `json:"tags"`
	Attempt           uint8     `json:"attempt"`
	UpstreamStatus    uint16    `json:"upstream_status"`
	ClientStatus      uint16    `json:"client_status"`
	ErrorCode         string    `json:"error_code"`
	ErrorCategory     string    `json:"error_category"`
	TimeoutType       string    `json:"timeout_type"`
	RequestSizeBytes  uint64    `json:"request_size_bytes"`
	ResponseSizeBytes uint64    `json:"response_size_bytes"`
	RoutingMS         uint32    `json:"routing_ms"`
	AssignmentMS      uint32    `json:"assignment_ms"`
	EgressMS          uint32    `json:"egress_ms"`
	TotalMS           uint32    `json:"total_ms"`
	CaptureDecision   string    `json:"capture_decision"`
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
	sink          RequestEventSink
	maxEntries    int
	batchSize     int
	flushInterval time.Duration

	mu    sync.Mutex
	queue []RequestEvent

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// RequestEventSink writes request metadata batches to ClickHouse.
type RequestEventSink interface {
	WriteRequestEvents(ctx context.Context, events []RequestEvent) error
}

// NewRequestMetadataWriter creates an async buffered writer.
func NewRequestMetadataWriter(sink RequestEventSink, maxEntries, batchSize int, flushInterval time.Duration) *RequestMetadataWriter {
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

	w := &RequestMetadataWriter{
		sink:          sink,
		maxEntries:    maxEntries,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}

	if sink != nil {
		go w.run()
	}

	return w
}

// Enqueue adds an accepted request to the bounded queue.
func (w *RequestMetadataWriter) Enqueue(event RequestEvent) {
	if w == nil || w.sink == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.queue) >= w.maxEntries {
		copy(w.queue, w.queue[1:])
		w.queue[len(w.queue)-1] = RequestEvent{}
		w.queue = w.queue[:len(w.queue)-1]
	}

	w.queue = append(w.queue, event)
}

// Flush writes queued events in batches.
func (w *RequestMetadataWriter) Flush(ctx context.Context) error {
	if w == nil || w.sink == nil {
		return nil
	}

	for {
		batch := w.nextBatch()
		if len(batch) == 0 {
			return nil
		}

		err := w.sink.WriteRequestEvents(ctx, batch)
		if err != nil {
			w.requeueFront(batch)

			return fmt.Errorf("write request metadata batch: %w", err)
		}
	}
}

// Close stops the background flush loop and drains the queue once.
func (w *RequestMetadataWriter) Close() {
	if w == nil || w.sink == nil {
		return
	}

	w.stopOnce.Do(func() {
		close(w.stopCh)
	})

	<-w.doneCh
}

func (w *RequestMetadataWriter) run() {
	defer close(w.doneCh)

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = w.Flush(context.Background())
		case <-w.stopCh:
			_ = w.Flush(context.Background())

			return
		}
	}
}

func (w *RequestMetadataWriter) nextBatch() []RequestEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.queue) == 0 {
		return nil
	}

	if len(w.queue) < w.batchSize {
		batch := append([]RequestEvent(nil), w.queue...)
		w.queue = w.queue[:0]

		return batch
	}

	batch := append([]RequestEvent(nil), w.queue[:w.batchSize]...)
	w.queue = append([]RequestEvent(nil), w.queue[w.batchSize:]...)

	return batch
}

func (w *RequestMetadataWriter) requeueFront(batch []RequestEvent) {
	if len(batch) == 0 {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(batch) >= w.maxEntries {
		w.queue = append([]RequestEvent(nil), batch[:w.maxEntries]...)

		return
	}

	space := w.maxEntries - len(batch)
	merged := make([]RequestEvent, 0, w.maxEntries)
	merged = append(merged, batch...)

	if len(w.queue) > space {
		merged = append(merged, w.queue[:space]...)
	} else {
		merged = append(merged, w.queue...)
	}

	w.queue = merged
}

// buildRequestEvent creates the canonical request metadata record from the
// validated request and resolved identity.
func buildRequestEvent(requestID string, identity Identity, request *ValidatedRequest) RequestEvent {
	var requestSize uint64
	if request != nil {
		requestSize = uint64(len(request.BodyData))
	}

	event := RequestEvent{
		Timestamp:        time.Now().UTC(),
		RequestID:        requestID,
		TenantID:         identity.TenantID,
		APIKeyID:         identity.APIKeyID,
		IngressType:      requestMetadataIngressType,
		TargetHost:       "",
		TargetURL:        "",
		Attempt:          1,
		UpstreamStatus:   http.StatusOK,
		ClientStatus:     http.StatusOK,
		CaptureDecision:  requestMetadataNone,
		RequestSizeBytes: requestSize,
	}

	if request != nil {
		event.Method = request.Method
	}

	if request != nil && request.URL != nil {
		event.TargetHost = request.URL.Hostname()
		if event.TargetHost == "" {
			event.TargetHost = request.URL.Host
		}

		event.TargetURL = sanitizeTargetURL(request.URL)
	}

	return event
}

func sanitizeTargetURL(u *url.URL) string {
	if u == nil {
		return ""
	}

	sanitized := *u
	sanitized.User = nil
	sanitized.RawQuery = ""
	sanitized.Fragment = ""

	if sanitized.Path == "" {
		sanitized.Path = "/"
	}

	return sanitized.String()
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
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key":
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

// WriteRequestEvents posts JSONEachRow records to ClickHouse.
func (s *HTTPClickHouseSink) WriteRequestEvents(ctx context.Context, events []RequestEvent) error {
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
	query.Set("query", "INSERT INTO "+s.table+" FORMAT JSONEachRow")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Content-Type", "application/json")

	if s.user != "" {
		req.SetBasicAuth(s.user, s.pass)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post clickhouse request events: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s", errClickHouseInsertFailed, resp.Status)
	}

	return nil
}
