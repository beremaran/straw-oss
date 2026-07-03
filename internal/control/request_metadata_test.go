package control

import (
	"context"
	"errors"
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
	if ev.IngressType != requestMetadataIngressType {
		t.Fatalf("IngressType = %q, want %q", ev.IngressType, requestMetadataIngressType)
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
		ID:            "key_actor",
		ScopeType:     ScopeTenant,
		TenantID:      "ten_actor",
		Role:          RoleRequester,
		Prefix:        gen.Prefix,
		SecretHash:    HashAPIKeySecret(gen.Secret, pepper),
		Status:        APIKeyStatusActive,
		CreatedAt:     time.Now().UTC(),
		ConfigVersion: 1,
	})

	recorder := &captureRequestMetadataRecorder{}
	handler := NewRequestHandler(1_048_576, 1_048_576, 120_000, NewAuthenticator(store, pepper), recorder)

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
	if event.APIKeyID != "key_actor" {
		t.Fatalf("APIKeyID = %q, want key_actor", event.APIKeyID)
	}
	if event.TenantID != "ten_actor" {
		t.Fatalf("TenantID = %q, want ten_actor", event.TenantID)
	}
	if event.TargetURL != requestMetadataTestTargetURL {
		t.Fatalf("TargetURL = %q, want %q", event.TargetURL, requestMetadataTestTargetURL)
	}
	if event.TargetHost != "example.com" {
		t.Fatalf("TargetHost = %q, want example.com", event.TargetHost)
	}
}
