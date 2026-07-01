package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/protocol"
)

const exampleURL = "https://example.com"

const testID = "test-123"

const getMethod = "GET"

func TestNewClient(t *testing.T) {
	client := NewClient("")
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

func TestNewClient_WithEgressID(t *testing.T) {
	client := NewClient("test-egress")

	if client.egressID != "test-egress" {
		t.Errorf("expected egress ID 'test-egress', got %v", client.egressID)
	}
}

func TestClient_Close(t *testing.T) {
	client := NewClient("")

	err := client.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestClient_Do_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := NewClient("test-ep")

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

	if resp.EgressID != "test-ep" {
		t.Errorf("expected egress ID test-ep, got %s", resp.EgressID)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if string(resp.Body) != `{"status": "ok"}` {
		t.Errorf("expected body %s, got %s", `{"status": "ok"}`, string(resp.Body))
	}
}

func TestClient_Do_Error(t *testing.T) {
	client := NewClient("")

	req := &protocol.Request{
		ID:      "test-error",
		Method:  getMethod,
		URL:     "http://localhost:1",
		Headers: protocol.HeaderMap{},
		Timeout: 100 * time.Millisecond,
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

func TestClient_Do_DefaultChromeProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewClient("")

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
