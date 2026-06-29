package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

const testEndpointID = "endpoint-001"

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

func (m *mockPublisherBroker) Subscribe(_ context.Context, _ string, _ broker.Handler, _ ...broker.SubscribeOption) error {
	return nil
}

func (m *mockPublisherBroker) ConsumeOnce(_ context.Context, _ string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockPublisherBroker) Close() error {
	return nil
}

func (m *mockPublisherBroker) DeclareStream(_ context.Context, _ string, _ ...string) error {
	return nil
}

func (m *mockPublisherBroker) IsConnected() bool {
	return true
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

	if !p.useConfirm {
		t.Error("expected useConfirm to be true by default")
	}
}

func TestPublisher_Publish(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	resp := &protocol.Response{
		RequestID:  "test-request-123",
		EndpointID: testEndpointID,
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

	var result ResultMessage
	err = json.Unmarshal(msg.Body, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.RequestID != "test-request-123" {
		t.Errorf("expected request ID 'test-request-123', got %q", result.RequestID)
	}

	if result.EndpointID != testEndpointID {
		t.Errorf("expected endpoint ID %q, got %q", testEndpointID, result.EndpointID)
	}

	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}

	if !result.BodyCompressed {
		t.Error("expected body to be compressed")
	}

	decompressed, err := protocol.Decompress(result.CompressedBody)
	if err != nil {
		t.Fatalf("failed to decompress body: %v", err)
	}

	if string(decompressed) != `{"message": "hello world"}` {
		t.Errorf("unexpected body: %s", string(decompressed))
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

	var result ResultMessage
	err = json.Unmarshal(msgs[0].Body, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(result.CompressedBody) != 0 {
		t.Error("expected empty compressed body")
	}

	if result.BodyCompressed {
		t.Error("expected body_compressed to be false for empty body")
	}
}

func TestPublisher_Publish_ErrorResponse(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	resp := &protocol.Response{
		RequestID:  "test-request-789",
		EndpointID: testEndpointID,
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

	var result ResultMessage
	err = json.Unmarshal(msgs[0].Body, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

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

func TestPublisher_PublishError(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	errInfo := &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "network error",
		Retryable: true,
	}

	err := p.PublishError(context.Background(), "test-req", testEndpointID, errInfo, "results.test-req")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := mb.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Subject != "results.test-req" {
		t.Errorf("expected subject 'results.test-req', got %q", msgs[0].Subject)
	}
}

func TestPublisher_Handler(t *testing.T) {
	mb := &mockPublisherBroker{}
	p := NewPublisher(mb)

	handler := p.Handler()
	if handler == nil {
		t.Fatal("expected handler to be non-nil")
	}

	resp := &protocol.Response{
		RequestID:  "test-request",
		StatusCode: 200,
	}

	err := handler(context.Background(), resp, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := mb.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestNewNetworkError(t *testing.T) {
	err := NewNetworkError("connection refused", true)

	if err.Code != protocol.ErrCodeUpstreamError {
		t.Errorf("expected code %q, got %q", protocol.ErrCodeUpstreamError, err.Code)
	}

	if err.Message != "network error: connection refused" {
		t.Errorf("unexpected message: %s", err.Message)
	}

	if !err.Retryable {
		t.Error("expected error to be retryable")
	}
}

func TestNewTLSError(t *testing.T) {
	err := NewTLSError("certificate verify failed")

	if err.Code != protocol.ErrCodeUpstreamError {
		t.Errorf("expected code %q, got %q", protocol.ErrCodeUpstreamError, err.Code)
	}

	if err.Message != "tls error: certificate verify failed" {
		t.Errorf("unexpected message: %s", err.Message)
	}

	if err.Retryable {
		t.Error("expected TLS errors to not be retryable")
	}
}

func TestNewHTTPError(t *testing.T) {
	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.statusCode), func(t *testing.T) {
			err := NewHTTPError(tt.statusCode, "test error")

			if err.Retryable != tt.retryable {
				t.Errorf("status %d: expected retryable=%v, got %v", tt.statusCode, tt.retryable, err.Retryable)
			}
		})
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("request timed out after 30s")

	if err.Code != protocol.ErrCodeEndpointTimeout {
		t.Errorf("expected code %q, got %q", protocol.ErrCodeEndpointTimeout, err.Code)
	}

	if err.Message != "timeout: request timed out after 30s" {
		t.Errorf("unexpected message: %s", err.Message)
	}

	if !err.Retryable {
		t.Error("expected timeout errors to be retryable")
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

	var result ResultMessage
	err = json.Unmarshal(msgs[0].Body, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if !result.BodyCompressed {
		t.Error("expected body to be compressed")
	}

	if len(result.CompressedBody) >= len(largeBody) {
		t.Errorf("expected compressed body to be smaller than original (%d >= %d)",
			len(result.CompressedBody), len(largeBody))
	}

	decompressed, err := protocol.Decompress(result.CompressedBody)
	if err != nil {
		t.Fatalf("failed to decompress body: %v", err)
	}

	if len(decompressed) != len(largeBody) {
		t.Errorf("decompressed size mismatch: %d != %d", len(decompressed), len(largeBody))
	}
}
