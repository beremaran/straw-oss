package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fhttp "github.com/useflyent/fhttp"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

type mockTransportProvider struct {
	transport *fhttp.Transport
}

func (m *mockTransportProvider) GetTransport(host string, preset fingerprint.Preset) *fhttp.Transport {
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
	req := NewRequest("GET", "https://example.com", nil)

	if req.Method != "GET" {
		t.Errorf("expected method GET, got %s", req.Method)
	}

	if req.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", req.URL)
	}

	if req.ID == "" {
		t.Error("expected non-empty request ID")
	}
}

func TestNewRequest_WithBody(t *testing.T) {
	body := []byte(`{"key": "value"}`)
	req := NewRequest("POST", "https://example.com/api", body)

	if req.Method != "POST" {
		t.Errorf("expected method POST, got %s", req.Method)
	}

	if string(req.Body) != `{"key": "value"}` {
		t.Errorf("expected body %s, got %s", `{"key": "value"}`, string(req.Body))
	}
}

func TestClient_Do_MockServer(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	registry := fingerprint.DefaultRegistry()
	// Mock provider that returns standard http transport adapter?
	// The client uses fhttp.Client. Standard httptest server supports http1.1.
	// fhttp should be compatible.
	provider := &mockTransportProvider{}
	client := NewClient(registry, provider, WithEndpointID("test-ep"))

	req := &protocol.Request{
		ID:          "test-123",
		Method:      "GET",
		URL:         server.URL,
		Headers:     protocol.HeaderMap{},
		Fingerprint: "chrome-133",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Do(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.RequestID != "test-123" {
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
		ID:          "test-error",
		Method:      "GET",
		URL:         "http://localhost:1", // Non-existent port
		Headers:     protocol.HeaderMap{},
		Fingerprint: "chrome-133",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	resp, err := client.Do(ctx, req)
	// We should get a response with an error, not an error from Do()
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

func TestClient_Do_FingerprintFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	registry := fingerprint.DefaultRegistry()
	provider := &mockTransportProvider{}
	client := NewClient(registry, provider)

	// Use a non-existent fingerprint
	req := &protocol.Request{
		ID:          "test-fallback",
		Method:      "GET",
		URL:         server.URL,
		Headers:     protocol.HeaderMap{},
		Fingerprint: "non-existent-fingerprint",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Do(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should succeed with fallback fingerprint
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
