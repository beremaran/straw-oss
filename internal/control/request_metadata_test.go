package control

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	requestMetadataTestRequestID       = "req_1"
	requestMetadataTestNextRequestID   = "req_2"
	requestMetadataTestTargetURL       = "https://example.com/path"
	requestMetadataTestRequestEnvelope = `{"method":"GET","url":"https://example.com/path?token=secret"}`
	testAuthorizationHeader            = "Authorization"
	requestMetadataTestKeyActor        = "key_actor"
	requestMetadataTestTenantActor     = "ten_actor"
)

type recordingRequestEventSink struct {
	mu      sync.Mutex
	batches [][]RequestEvent
	err     error
}

func (s *recordingRequestEventSink) WriteRequestEvents(_ context.Context, events []RequestEvent) error {
	if s.err != nil {
		return s.err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	batch := append([]RequestEvent(nil), events...)
	s.batches = append(s.batches, batch)

	return nil
}

func (s *recordingRequestEventSink) events() []RequestEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []RequestEvent
	for _, batch := range s.batches {
		out = append(out, batch...)
	}

	return out
}

type captureRequestMetadataRecorder struct {
	mu     sync.Mutex
	events []RequestEvent
}

func (r *captureRequestMetadataRecorder) Enqueue(event RequestEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
}

func (r *captureRequestMetadataRecorder) last() (RequestEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.events) == 0 {
		return RequestEvent{}, false
	}

	return r.events[len(r.events)-1], true
}

func TestSanitizeTargetURLDropsQuery(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://example.com:8443/path/to/resource?token=secret&verbose=true")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	got := sanitizeTargetURL(u)
	want := "https://example.com:8443/path/to/resource"
	if got != want {
		t.Fatalf("sanitizeTargetURL() = %q, want %q", got, want)
	}
}

func TestRedactSensitiveHeaderValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: testAuthorizationHeader, value: "Bearer secret", want: requestMetadataRedacted},
		{name: "Cookie", value: "session=secret", want: requestMetadataRedacted},
		{name: "Proxy-Authorization", value: "Basic secret", want: requestMetadataRedacted},
		{name: "Set-Cookie", value: "session=secret", want: requestMetadataRedacted},
		{name: "X-Api-Key", value: "secret", want: requestMetadataRedacted},
		{name: "X-Straw-Injection-Secret", value: "secret", want: requestMetadataRedacted},
		{name: "X-Trace-ID", value: "trace-123", want: "trace-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := redactSensitiveHeaderValue(tt.name, tt.value)
			if got != tt.want {
				t.Fatalf("redactSensitiveHeaderValue(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestBuildRequestEventRecordsActorAndSanitizedTarget(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://example.com/path?token=secret")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	ev := buildRequestEvent("req_123", Identity{APIKeyID: "key_123", TenantID: "ten_123"}, &ValidatedRequest{
		Method:     http.MethodPost,
		URL:        u,
		BodyData:   []byte("hello"),
		Headers:    []HeaderPair{{Name: "Authorization", Value: "Bearer secret"}},
		Replayable: true,
	})

	if ev.RequestID != "req_123" {
		t.Fatalf("RequestID = %q, want req_123", ev.RequestID)
	}
	if ev.APIKeyID != "key_123" {
		t.Fatalf("APIKeyID = %q, want key_123", ev.APIKeyID)
	}
	if ev.TenantID != "ten_123" {
		t.Fatalf("TenantID = %q, want ten_123", ev.TenantID)
	}
	if ev.TargetHost != "example.com" {
		t.Fatalf("TargetHost = %q, want example.com", ev.TargetHost)
	}
	if ev.TargetURL != "https://example.com/path" {
		t.Fatalf("TargetURL = %q, want https://example.com/path", ev.TargetURL)
	}
	if ev.Method != http.MethodPost {
		t.Fatalf("Method = %q, want POST", ev.Method)
	}
	if ev.RequestSizeBytes != 5 {
		t.Fatalf("RequestSizeBytes = %d, want 5", ev.RequestSizeBytes)
	}
	if ev.IngressType != IngressTypeREST {
		t.Fatalf("IngressType = %q, want %q", ev.IngressType, IngressTypeREST)
	}
	if ev.CaptureDecision != requestMetadataNone {
		t.Fatalf("CaptureDecision = %q, want %q", ev.CaptureDecision, requestMetadataNone)
	}
	if ev.Timestamp.IsZero() {
		t.Fatal("Timestamp is zero")
	}
}

func TestRequestMetadataWriterFlushSuccess(t *testing.T) {
	t.Parallel()

	sink := &recordingRequestEventSink{}
	writer := NewRequestMetadataWriter(sink, 10, 10, time.Hour)
	t.Cleanup(writer.Close)

	writer.Enqueue(RequestEvent{RequestID: requestMetadataTestRequestID})

	err := writer.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	events := sink.events()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].RequestID != requestMetadataTestRequestID {
		t.Fatalf("event request_id = %q, want %q", events[0].RequestID, requestMetadataTestRequestID)
	}
}

func TestRequestMetadataWriterOutageKeepsQueuedEvents(t *testing.T) {
	t.Parallel()

	sink := &recordingRequestEventSink{err: errors.New("clickhouse down")}
	writer := NewRequestMetadataWriter(sink, 10, 10, time.Hour)
	t.Cleanup(writer.Close)

	writer.Enqueue(RequestEvent{RequestID: requestMetadataTestRequestID})

	err := writer.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want outage error")
	}
	if len(sink.events()) != 0 {
		t.Fatalf("events len = %d, want 0 after failed flush", len(sink.events()))
	}

	sink.err = nil
	err = writer.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() retry error = %v", err)
	}

	events := sink.events()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1 after retry", len(events))
	}
	if events[0].RequestID != requestMetadataTestRequestID {
		t.Fatalf("event request_id = %q, want %q", events[0].RequestID, requestMetadataTestRequestID)
	}
}

func TestRequestMetadataWriterDropsOldestWhenFull(t *testing.T) {
	t.Parallel()

	sink := &recordingRequestEventSink{}
	writer := NewRequestMetadataWriter(sink, 1, 1, time.Hour)
	t.Cleanup(writer.Close)

	writer.Enqueue(RequestEvent{RequestID: requestMetadataTestRequestID})
	writer.Enqueue(RequestEvent{RequestID: requestMetadataTestNextRequestID})

	err := writer.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	events := sink.events()
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].RequestID != requestMetadataTestNextRequestID {
		t.Fatalf("event request_id = %q, want %q", events[0].RequestID, requestMetadataTestNextRequestID)
	}
}

func TestRequestHandlerQueuesSanitizedMetadata(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	mustCreate(t, store, APIKeyRecord{
		ID:            requestMetadataTestKeyActor,
		ScopeType:     ScopeTenant,
		TenantID:      requestMetadataTestTenantActor,
		Role:          RoleRequester,
		Prefix:        gen.Prefix,
		SecretHash:    HashAPIKeySecret(gen.Secret, pepper),
		Status:        APIKeyStatusActive,
		CreatedAt:     time.Now().UTC(),
		ConfigVersion: 1,
	})

	recorder := &captureRequestMetadataRecorder{}
	handler := NewRequestHandler(1_048_576, 1_048_576, 120_000, NewAuthenticator(store, pepper), recorder)
	handler.SetDispatcher(fakeRequestDispatcher{})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(requestMetadataTestRequestEnvelope))
	req.Header.Set("Authorization", "Bearer "+gen.Secret)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	event, ok := recorder.last()
	if !ok {
		t.Fatal("metadata recorder did not receive an event")
	}
	if event.APIKeyID != requestMetadataTestKeyActor {
		t.Fatalf("APIKeyID = %q, want key_actor", event.APIKeyID)
	}
	if event.TenantID != requestMetadataTestTenantActor {
		t.Fatalf("TenantID = %q, want ten_actor", event.TenantID)
	}
	if event.TargetURL != requestMetadataTestTargetURL {
		t.Fatalf("TargetURL = %q, want %q", event.TargetURL, requestMetadataTestTargetURL)
	}
	if event.TargetHost != "example.com" {
		t.Fatalf("TargetHost = %q, want example.com", event.TargetHost)
	}
	if event.UpstreamStatus != http.StatusOK {
		t.Fatalf("UpstreamStatus = %d, want 200 (docs/tasks/p0/32: real dispatch outcome)", event.UpstreamStatus)
	}
	if event.ClientStatus != http.StatusOK {
		t.Fatalf("ClientStatus = %d, want 200", event.ClientStatus)
	}
}

// fakeFailingRequestDispatcher always returns a canonical PipelineError, so
// request_events finalization (docs/tasks/p0/32) can be tested against a
// dispatch failure instead of only the success path.
type fakeFailingRequestDispatcher struct{}

func (fakeFailingRequestDispatcher) Dispatch(context.Context, DispatchInput) (SuccessResponse, *PipelineError) {
	return SuccessResponse{}, &PipelineError{Code: RouteNoMatch, RoutingMs: 5, TotalMs: 7}
}

// TestRequestHandlerRecordsFailureOutcome verifies a failed dispatch produces
// a request_events row with the canonical error code/category instead of the
// pre-dispatch synthetic 200 the writer used to emit (docs/tasks/p0/32).
func TestRequestHandlerRecordsFailureOutcome(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	mustCreate(t, store, APIKeyRecord{
		ID:            requestMetadataTestKeyActor,
		ScopeType:     ScopeTenant,
		TenantID:      requestMetadataTestTenantActor,
		Role:          RoleRequester,
		Prefix:        gen.Prefix,
		SecretHash:    HashAPIKeySecret(gen.Secret, pepper),
		Status:        APIKeyStatusActive,
		CreatedAt:     time.Now().UTC(),
		ConfigVersion: 1,
	})

	recorder := &captureRequestMetadataRecorder{}
	handler := NewRequestHandler(1_048_576, 1_048_576, 120_000, NewAuthenticator(store, pepper), recorder)
	handler.SetDispatcher(fakeFailingRequestDispatcher{})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(requestMetadataTestRequestEnvelope))
	req.Header.Set("Authorization", "Bearer "+gen.Secret)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("status = %d, want a failure status", w.Code)
	}

	event, ok := recorder.last()
	if !ok {
		t.Fatal("metadata recorder did not receive an event")
	}
	if event.UpstreamStatus != 0 {
		t.Fatalf("UpstreamStatus = %d, want 0 (no synthetic 200 on failure)", event.UpstreamStatus)
	}
	if event.ClientStatus == http.StatusOK {
		t.Fatal("ClientStatus = 200, want the canonical failure status")
	}
	if event.ErrorCode != ErrorRegistry[RouteNoMatch].Code {
		t.Fatalf("ErrorCode = %q, want %q", event.ErrorCode, ErrorRegistry[RouteNoMatch].Code)
	}
	if event.ErrorCategory != ErrorRegistry[RouteNoMatch].Category {
		t.Fatalf("ErrorCategory = %q, want %q", event.ErrorCategory, ErrorRegistry[RouteNoMatch].Category)
	}
	if event.RoutingMS != 5 {
		t.Fatalf("RoutingMS = %d, want 5 (partial timing measured before failure)", event.RoutingMS)
	}
	if event.TotalMS != 7 {
		t.Fatalf("TotalMS = %d, want 7", event.TotalMS)
	}
}

func TestApplyRequestOutcomeSuccessFillsRealFields(t *testing.T) {
	t.Parallel()

	base := RequestEvent{RequestID: "req_ok"}
	resp := SuccessResponse{
		Status:            http.StatusCreated,
		Timing:            RequestTiming{RoutingMs: 1, AssignmentMs: 2, EgressMs: 3, TotalMs: 6},
		ResponseSizeBytes: 42,
	}

	got := applyRequestOutcome(base, resp, nil)

	if got.UpstreamStatus != http.StatusCreated {
		t.Fatalf("UpstreamStatus = %d, want 201", got.UpstreamStatus)
	}
	if got.ClientStatus != http.StatusOK {
		t.Fatalf("ClientStatus = %d, want 200", got.ClientStatus)
	}
	if got.ResponseSizeBytes != 42 {
		t.Fatalf("ResponseSizeBytes = %d, want 42", got.ResponseSizeBytes)
	}
	if got.RoutingMS != 1 || got.AssignmentMS != 2 || got.EgressMS != 3 || got.TotalMS != 6 {
		t.Fatalf("timings = %+v, want routing=1 assignment=2 egress=3 total=6", got)
	}
	if got.ErrorCode != "" {
		t.Fatalf("ErrorCode = %q, want empty on success", got.ErrorCode)
	}
}

func TestApplyRequestOutcomeFailureFillsCanonicalError(t *testing.T) {
	t.Parallel()

	base := RequestEvent{RequestID: "req_fail"}
	perr := &PipelineError{Code: TimeoutExceeded, TimeoutType: "total_deadline_timeout", RoutingMs: 1, AssignmentMs: 2, TotalMs: 10}

	got := applyRequestOutcome(base, SuccessResponse{}, perr)

	if got.UpstreamStatus != 0 {
		t.Fatalf("UpstreamStatus = %d, want 0", got.UpstreamStatus)
	}
	if got.ClientStatus != uint16OrMax(ErrorRegistry[TimeoutExceeded].HTTPStatus) {
		t.Fatalf("ClientStatus = %d, want %d", got.ClientStatus, ErrorRegistry[TimeoutExceeded].HTTPStatus)
	}
	if got.ErrorCode != ErrorRegistry[TimeoutExceeded].Code {
		t.Fatalf("ErrorCode = %q, want %q", got.ErrorCode, ErrorRegistry[TimeoutExceeded].Code)
	}
	if got.TimeoutType != "total_deadline_timeout" {
		t.Fatalf("TimeoutType = %q, want total_deadline_timeout", got.TimeoutType)
	}
	if got.TotalMS != 10 {
		t.Fatalf("TotalMS = %d, want 10", got.TotalMS)
	}
}

// TestHTTPClickHouseSinkRequestFormat pins the exact HTTP request the live
// sink sends. It regression-guards the DateTime64 parse failure that silently
// dropped all telemetry: RequestEvent.Timestamp serializes as RFC3339, which
// ClickHouse rejects unless date_time_input_format=best_effort is set. The
// prior tests all used an in-memory sink, so this real-transport path was
// never exercised.
func TestHTTPClickHouseSinkRequestFormat(t *testing.T) {
	var gotQuery url.Values
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := NewHTTPClickHouseSink(srv.URL, "straw", "", "", srv.Client())
	ts := time.Date(2026, 7, 4, 16, 44, 11, int(812*time.Millisecond), time.UTC)
	err := sink.WriteRequestEvents(context.Background(), []RequestEvent{{
		Timestamp: ts, RequestID: "req_ch", TenantID: "t1", ClientStatus: 404, ErrorCode: errorCodeRouteNoMatch,
	}})
	if err != nil {
		t.Fatalf("WriteRequestEvents() error = %v", err)
	}

	if got := gotQuery.Get("date_time_input_format"); got != "best_effort" {
		t.Fatalf("date_time_input_format = %q, want best_effort (RFC3339 timestamps 400 without it)", got)
	}
	if got := gotQuery.Get("query"); got != "INSERT INTO request_events FORMAT JSONEachRow" {
		t.Fatalf("query = %q", got)
	}
	if !strings.Contains(gotBody, `"timestamp":"2026-07-04T16:44:11`) {
		t.Fatalf("body timestamp not RFC3339 as expected: %s", gotBody)
	}
}
