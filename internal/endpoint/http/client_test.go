package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/pkg/protocol"
)

const exampleURL = "https://example.com"

const testID = "test-123"

const getMethod = "GET"

type mockTransportProvider struct {
	transport *fhttp.Transport
}

func (m *mockTransportProvider) GetTransport(_ string, _ fingerprint.Preset) *fhttp.Transport {
	if m.transport != nil {
		return m.transport
	}

	return &fhttp.Transport{}
}

func TestNewClient(t *testing.T) {
	registry := fingerprint.DefaultRegistry()
	provider := &mockTransportProvider{}

	client := NewClient(registry, provider)
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.defaultTimeout != DefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultTimeout, client.defaultTimeout)
	}

	if client.maxBodySize != DefaultMaxBodySize {
		t.Errorf("expected max body size %v, got %v", DefaultMaxBodySize, client.maxBodySize)
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	registry := fingerprint.DefaultRegistry()
	provider := &mockTransportProvider{}

	client := NewClient(
		registry,
		provider,
		WithDefaultTimeout(60*time.Second),
		WithMaxBodySize(5*1024*1024),
		WithEndpointID("test-endpoint"),
	)

	if client.defaultTimeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", client.defaultTimeout)
	}

	if client.maxBodySize != 5*1024*1024 {
		t.Errorf("expected max body size 5MB, got %v", client.maxBodySize)
	}

	if client.endpointID != "test-endpoint" {
		t.Errorf("expected endpoint ID 'test-endpoint', got %v", client.endpointID)
	}
}

func TestClient_Close(t *testing.T) {
	registry := fingerprint.DefaultRegistry()
	provider := &mockTransportProvider{}
	client := NewClient(registry, provider)

	err := client.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestNewRequest(t *testing.T) {
	req := NewRequest(getMethod, exampleURL, nil)

	if req.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", req.Method)
	}

	if req.URL != exampleURL {
		t.Errorf("expected URL https://example.com, got %s", req.URL)
	}

	if req.ID == "" {
		t.Error("expected non-empty request ID")
	}
}

func TestNewRequest_WithBody(t *testing.T) {
	body := []byte(`{"key": "value"}`)
	req := NewRequest("POST", "https://example.com/api", body)

	if req.Method != http.MethodPost {
		t.Errorf("expected method POST, got %s", req.Method)
	}

	if string(req.Body) != `{"key": "value"}` {
		t.Errorf("expected body %s, got %s", `{"key": "value"}`, string(req.Body))
	}
}

func TestClient_Do_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	registry := fingerprint.DefaultRegistry()

	provider := &mockTransportProvider{}
	client := NewClient(registry, provider, WithEndpointID("test-ep"))

	req := &protocol.Request{
		ID:      testID,
		Method:  getMethod,
		URL:     server.URL,
		Headers: protocol.HeaderMap{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Do(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.RequestID != testID {
		t.Errorf("expected request ID test-123, got %s", resp.RequestID)
	}

	if resp.EndpointID != "test-ep" {
		t.Errorf("expected endpoint ID test-ep, got %s", resp.EndpointID)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if string(resp.Body) != `{"status": "ok"}` {
		t.Errorf("expected body %s, got %s", `{"status": "ok"}`, string(resp.Body))
	}
}

func TestClient_Do_Error(t *testing.T) {
	registry := fingerprint.DefaultRegistry()
	provider := &mockTransportProvider{}
	client := NewClient(registry, provider, WithDefaultTimeout(100*time.Millisecond))

	req := &protocol.Request{
		ID:      "test-error",
		Method:  getMethod,
		URL:     "http://localhost:1",
		Headers: protocol.HeaderMap{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	resp, err := client.Do(ctx, req)
	if err != nil {
		t.Fatalf("expected response with error field, got error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error in response")
	}

	if resp.Error.Code != protocol.ErrCodeUpstreamError {
		t.Errorf("expected error code %s, got %s", protocol.ErrCodeUpstreamError, resp.Error.Code)
	}
}

func TestClient_Do_DefaultFingerprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	registry := fingerprint.DefaultRegistry()
	provider := &mockTransportProvider{}
	client := NewClient(registry, provider)

	req := &protocol.Request{
		ID:      "test-default",
		Method:  getMethod,
		URL:     server.URL,
		Headers: protocol.HeaderMap{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Do(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestClientError(t *testing.T) {
	err := &ClientError{
		Code:    "TEST_ERROR",
		Message: "test error message",
	}

	expected := "TEST_ERROR: test error message"
	if err.Error() != expected {
		t.Errorf("expected error string %q, got %q", expected, err.Error())
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"generic error", &ClientError{Code: "TEST", Message: "test"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
