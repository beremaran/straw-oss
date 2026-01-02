package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// consumeOnceMockBroker implements broker.MessageBroker for testing Consumer.
type consumeOnceMockBroker struct {
	response    []byte
	responseErr error

	// For tracking calls
	calledQueue   string
	calledTimeout time.Duration
}

func (m *consumeOnceMockBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	return nil
}

func (m *consumeOnceMockBroker) Subscribe(ctx context.Context, queue string, handler broker.Handler) error {
	return nil
}

func (m *consumeOnceMockBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
	return nil
}

func (m *consumeOnceMockBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	m.calledQueue = queue
	m.calledTimeout = timeout

	if m.responseErr != nil {
		return nil, m.responseErr
	}
	return m.response, nil
}

func (m *consumeOnceMockBroker) Close() error {
	return nil
}

func (m *consumeOnceMockBroker) DeclareExchange(ctx context.Context, name, kind string) error {
	return nil
}

func (m *consumeOnceMockBroker) DeclareQueue(ctx context.Context, name string) error {
	return nil
}

func (m *consumeOnceMockBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	return nil
}

func (m *consumeOnceMockBroker) IsConnected() bool {
	return true
}

func (m *consumeOnceMockBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	return 0, nil
}

func TestConsumer_New(t *testing.T) {
	mb := &consumeOnceMockBroker{}
	c := NewConsumer(mb)

	if c.timeout != DefaultResultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultResultTimeout, c.timeout)
	}

	if c.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestConsumer_NewWithOptions(t *testing.T) {
	mb := &consumeOnceMockBroker{}
	customTimeout := 60 * time.Second

	c := NewConsumer(mb, WithTimeout(customTimeout))

	if c.timeout != customTimeout {
		t.Errorf("expected timeout %v, got %v", customTimeout, c.timeout)
	}
}

func TestConsumer_WithConsumerLogger(t *testing.T) {
	mb := &consumeOnceMockBroker{}
	logger := slog.Default()

	c := NewConsumer(mb, WithConsumerLogger(logger))

	if c.logger != logger {
		t.Error("expected logger to be set")
	}
}

func TestConsumer_WaitForResult_Success(t *testing.T) {
	// Create a result message
	result := ResultMessage{
		RequestID:      "test-req-123",
		EndpointID:     "endpoint-001",
		StatusCode:     200,
		Headers:        protocol.HeaderMap{{Key: "Content-Type", Value: "application/json"}},
		CompressedBody: []byte(`{"success": true}`), // uncompressed for this test
		BodyCompressed: false,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	mb := &consumeOnceMockBroker{
		response: resultBytes,
	}
	c := NewConsumer(mb, WithTimeout(5*time.Second))

	got, err := c.WaitForResult(context.Background(), "results.test-req-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.RequestID != "test-req-123" {
		t.Errorf("expected request ID 'test-req-123', got %q", got.RequestID)
	}

	if got.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", got.StatusCode)
	}

	if mb.calledQueue != "results.test-req-123" {
		t.Errorf("expected queue 'results.test-req-123', got %q", mb.calledQueue)
	}

	if mb.calledTimeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", mb.calledTimeout)
	}
}

func TestConsumer_WaitForResult_Timeout(t *testing.T) {
	mb := &consumeOnceMockBroker{
		responseErr: broker.ErrTimeout,
	}
	c := NewConsumer(mb, WithTimeout(100*time.Millisecond))

	_, err := c.WaitForResult(context.Background(), "results.test-req-456")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrResultTimeout) {
		t.Errorf("expected ErrResultTimeout, got %v", err)
	}
}

func TestConsumer_WaitForResult_Decompression(t *testing.T) {
	// Create original body and compress it
	originalBody := []byte(`{"message": "hello world", "status": "ok"}`)
	compressedBody, err := protocol.Compress(originalBody)
	if err != nil {
		t.Fatalf("failed to compress body: %v", err)
	}

	// Create result with compressed body
	result := ResultMessage{
		RequestID:      "test-req-compressed",
		StatusCode:     200,
		CompressedBody: compressedBody,
		BodyCompressed: true,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	mb := &consumeOnceMockBroker{
		response: resultBytes,
	}
	c := NewConsumer(mb)

	got, err := c.WaitForResult(context.Background(), "results.test-req-compressed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Body should be decompressed
	if got.BodyCompressed {
		t.Error("expected body to be decompressed (BodyCompressed = false)")
	}

	if string(got.CompressedBody) != string(originalBody) {
		t.Errorf("expected body %q, got %q", string(originalBody), string(got.CompressedBody))
	}
}

func TestConsumer_WaitForResult_ErrorResponse(t *testing.T) {
	// Create result with error
	result := ResultMessage{
		RequestID:  "test-req-error",
		EndpointID: "endpoint-001",
		Error: &protocol.ErrorInfo{
			Code:      protocol.ErrCodeUpstreamError,
			Message:   "connection refused",
			Retryable: true,
		},
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	mb := &consumeOnceMockBroker{
		response: resultBytes,
	}
	c := NewConsumer(mb)

	got, err := c.WaitForResult(context.Background(), "results.test-req-error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Error == nil {
		t.Fatal("expected error to be set")
	}

	if got.Error.Code != protocol.ErrCodeUpstreamError {
		t.Errorf("expected error code %q, got %q", protocol.ErrCodeUpstreamError, got.Error.Code)
	}
}

func TestConsumer_WaitForResult_InvalidJSON(t *testing.T) {
	mb := &consumeOnceMockBroker{
		response: []byte("not valid json"),
	}
	c := NewConsumer(mb)

	_, err := c.WaitForResult(context.Background(), "results.test-req-invalid")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestResultMessage_ToResponse(t *testing.T) {
	result := &ResultMessage{
		RequestID:      "test-req-123",
		EndpointID:     "endpoint-001",
		SessionID:      "session-567",
		StatusCode:     201,
		Headers:        protocol.HeaderMap{{Key: "X-Custom", Value: "value"}},
		CompressedBody: []byte("test body"),
		Error: &protocol.ErrorInfo{
			Code:    "TEST_ERROR",
			Message: "test",
		},
		Timing: &protocol.TimingInfo{
			Total: 100 * time.Millisecond,
		},
	}

	resp := result.ToResponse()

	if resp.RequestID != "test-req-123" {
		t.Errorf("expected request ID 'test-req-123', got %q", resp.RequestID)
	}

	if resp.EndpointID != "endpoint-001" {
		t.Errorf("expected endpoint ID 'endpoint-001', got %q", resp.EndpointID)
	}

	if resp.SessionID != "session-567" {
		t.Errorf("expected session ID 'session-567', got %q", resp.SessionID)
	}

	if resp.StatusCode != 201 {
		t.Errorf("expected status code 201, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "test body" {
		t.Errorf("expected body 'test body', got %q", string(resp.Body))
	}

	if resp.Error == nil || resp.Error.Code != "TEST_ERROR" {
		t.Error("expected error to be copied")
	}

	if resp.Timing == nil || resp.Timing.Total != 100*time.Millisecond {
		t.Error("expected timing to be copied")
	}
}

func TestConsumer_WaitForResult_DecompressionError(t *testing.T) {
	// Create invalid compressed data
	result := ResultMessage{
		RequestID:      "test-req-decomp-error",
		StatusCode:     200,
		CompressedBody: []byte("invalid-compressed-data"),
		BodyCompressed: true,
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	mb := &consumeOnceMockBroker{
		response: resultBytes,
	}
	c := NewConsumer(mb)

	got, err := c.WaitForResult(context.Background(), "results.test-req-decomp-error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to compressed data but marked as not compressed (as per implementation logic)
	// This confirms the current behavior, though arguably it might be better to return error?
	// The implementation suppresses error and returns raw bytes.
	if got.BodyCompressed {
		t.Error("expected BodyCompressed to be false after fallback")
	}

	if string(got.CompressedBody) != "invalid-compressed-data" {
		t.Errorf("expected original body, got %q", string(got.CompressedBody))
	}
}
