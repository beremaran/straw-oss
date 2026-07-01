package egress

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/protocol"
)

const testEgressID = "egress-001"

type mockPublisherBroker struct {
	publishedMsgs []publishedMsg
	mu            sync.Mutex
	publishErr    error
}

func (m *mockPublisherBroker) Publish(_ context.Context, subject string, body []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, publishedMsg{
		Subject: subject,
		Body:    body,
	})

	return nil
}

func (m *mockPublisherBroker) getMessages() []publishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.publishedMsgs
}

func TestPublisher_New(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	if p.broker != mb {
		t.Error("expected broker to be set")
	}

	if p.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestPublisher_Publish(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	resp := &protocol.Response{
		RequestID:  "test-request-123",
		EgressID:   testEgressID,
		StatusCode: 200,
		Headers:    protocol.HeaderMap{{Key: "Content-Type", Value: "application/json"}},
		Body:       []byte(`{"message": "hello world"}`),
		Timing: &protocol.TimingInfo{
			Total: 100 * time.Millisecond,
		},
	}

	err := p.Publish(context.Background(), resp, "results.test-request-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := mb.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	msg := msgs[0]

	expectedSubject := "results.test-request-123"
	if msg.Subject != expectedSubject {
		t.Errorf("expected subject %q, got %q", expectedSubject, msg.Subject)
	}

	result := decodePublishedResponse(t, msg.Body)

	if result.RequestID != "test-request-123" {
		t.Errorf("expected request ID 'test-request-123', got %q", result.RequestID)
	}

	if result.EgressID != testEgressID {
		t.Errorf("expected egress ID %q, got %q", testEgressID, result.EgressID)
	}

	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}

	if string(result.Body) != `{"message": "hello world"}` {
		t.Errorf("unexpected body: %s", string(result.Body))
	}

	if result.Timing == nil {
		t.Error("expected timing to be set")
	}
}

func TestPublisher_Publish_EmptyBody(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	resp := &protocol.Response{
		RequestID:  "test-request-456",
		StatusCode: 204,
	}

	err := p.Publish(context.Background(), resp, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := mb.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	result := decodePublishedResponse(t, msgs[0].Body)

	if len(result.Body) != 0 {
		t.Error("expected empty body")
	}
}

func TestPublisher_Publish_ErrorResponse(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	resp := &protocol.Response{
		RequestID: "test-request-789",
		EgressID:  testEgressID,
		Error: &protocol.ErrorInfo{
			Code:      protocol.ErrCodeUpstreamError,
			Message:   "connection refused",
			Retryable: true,
		},
	}

	err := p.Publish(context.Background(), resp, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := mb.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	result := decodePublishedResponse(t, msgs[0].Body)

	if result.Error == nil {
		t.Fatal("expected error to be set")
	}

	if result.Error.Code != protocol.ErrCodeUpstreamError {
		t.Errorf("expected error code %q, got %q", protocol.ErrCodeUpstreamError, result.Error.Code)
	}

	if result.Error.Message != "connection refused" {
		t.Errorf("expected error message 'connection refused', got %q", result.Error.Message)
	}

	if !result.Error.Retryable {
		t.Error("expected error to be retryable")
	}
}

func TestPublisher_Publish_NilResponse(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	err := p.Publish(context.Background(), nil, "")
	if err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestPublisher_Publish_MissingRequestID(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	resp := &protocol.Response{
		StatusCode: 200,
	}

	err := p.Publish(context.Background(), resp, "")
	if err == nil {
		t.Fatal("expected error for missing request ID")
	}
}

func TestPublisher_Publish_LargeBody(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	largeBody := make([]byte, 100*1024)
	for i := range largeBody {
		largeBody[i] = byte('A' + (i % 26))
	}

	resp := &protocol.Response{
		RequestID:  "test-large-body",
		StatusCode: 200,
		Body:       largeBody,
	}

	err := p.Publish(context.Background(), resp, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := mb.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	result := decodePublishedResponse(t, msgs[0].Body)
	if len(result.Body) != len(largeBody) {
		t.Errorf("body size mismatch: %d != %d", len(result.Body), len(largeBody))
	}

	if string(result.Body) != string(largeBody) {
		t.Error("body bytes changed")
	}
}

func decodePublishedResponse(t *testing.T, data []byte) *protocol.Response {
	t.Helper()

	result, err := protocol.UnmarshalResponse(data)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	return result
}
