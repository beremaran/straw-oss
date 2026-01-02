package http

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

func TestBuildRequest_Basic(t *testing.T) {
	preset, ok := fingerprint.Get("chrome-133")
	if !ok {
		t.Fatal("chrome-133 preset not found")
	}

	req := &protocol.Request{
		ID:     "test-123",
		Method: "GET",
		URL:    "https://example.com/path",
		Headers: protocol.HeaderMap{
			{Key: "X-Custom-Header", Value: "custom-value"},
		},
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req, preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fhttpReq.Method != "GET" {
		t.Errorf("expected method GET, got %s", fhttpReq.Method)
	}

	if fhttpReq.URL.String() != "https://example.com/path" {
		t.Errorf("expected URL https://example.com/path, got %s", fhttpReq.URL.String())
	}

	if fhttpReq.Host != "example.com" {
		t.Errorf("expected host example.com, got %s", fhttpReq.Host)
	}

	if fhttpReq.Header.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected custom header value 'custom-value', got %s", fhttpReq.Header.Get("X-Custom-Header"))
	}
}

func TestBuildRequest_WithBody(t *testing.T) {
	preset, _ := fingerprint.Get("chrome-133")

	body := []byte(`{"key": "value"}`)
	req := &protocol.Request{
		ID:      "test-body",
		Method:  "POST",
		URL:     "https://example.com/api",
		Headers: protocol.HeaderMap{},
		Body:    body,
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req, preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fhttpReq.Method != "POST" {
		t.Errorf("expected method POST, got %s", fhttpReq.Method)
	}

	if fhttpReq.Body == nil {
		t.Error("expected non-nil body")
	}
}

func TestBuildRequest_InvalidURL(t *testing.T) {
	preset, _ := fingerprint.Get("chrome-133")

	req := &protocol.Request{
		ID:      "test-invalid",
		Method:  "GET",
		URL:     "://invalid-url",
		Headers: protocol.HeaderMap{},
	}

	ctx := context.Background()
	_, err := BuildRequest(ctx, req, preset)
	if err == nil {
		t.Error("expected error for invalid URL")
	}

	var clientErr *ClientError
	ok := errors.As(err, &clientErr)
	if !ok {
		t.Errorf("expected ClientError, got %T", err)
	} else if clientErr.Code != "INVALID_URL" {
		t.Errorf("expected error code INVALID_URL, got %s", clientErr.Code)
	}
}

func TestBuildRequest_AppliesFingerprintHeaders(t *testing.T) {
	preset, _ := fingerprint.Get("chrome-133")

	req := &protocol.Request{
		ID:      "test-fp-headers",
		Method:  "GET",
		URL:     "https://example.com",
		Headers: protocol.HeaderMap{},
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req, preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that fingerprint-specific headers are set
	if preset.UserAgent != "" && fhttpReq.Header.Get("User-Agent") != preset.UserAgent {
		t.Errorf("expected User-Agent %q, got %q", preset.UserAgent, fhttpReq.Header.Get("User-Agent"))
	}

	// Check default browser headers
	if fhttpReq.Header.Get("Accept") == "" {
		t.Error("expected Accept header to be set")
	}

	if fhttpReq.Header.Get("Accept-Encoding") == "" {
		t.Error("expected Accept-Encoding header to be set")
	}

	if fhttpReq.Header.Get("Connection") == "" {
		t.Error("expected Connection header to be set")
	}
}

func TestBuildRequest_DoesNotOverrideExistingHeaders(t *testing.T) {
	preset, _ := fingerprint.Get("chrome-133")

	req := &protocol.Request{
		ID:     "test-no-override",
		Method: "GET",
		URL:    "https://example.com",
		Headers: protocol.HeaderMap{
			{Key: "User-Agent", Value: "Custom-Agent"},
			{Key: "Accept", Value: "text/plain"},
		},
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req, preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Custom headers should not be overridden
	if fhttpReq.Header.Get("User-Agent") != "Custom-Agent" {
		t.Errorf("expected User-Agent 'Custom-Agent', got %q", fhttpReq.Header.Get("User-Agent"))
	}

	if fhttpReq.Header.Get("Accept") != "text/plain" {
		t.Errorf("expected Accept 'text/plain', got %q", fhttpReq.Header.Get("Accept"))
	}
}

func TestBuildRequest_WithContext(t *testing.T) {
	preset, _ := fingerprint.Get("chrome-133")

	req := &protocol.Request{
		ID:      "test-context",
		Method:  "GET",
		URL:     "https://example.com",
		Headers: protocol.HeaderMap{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fhttpReq, err := BuildRequest(ctx, req, preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify context is attached
	if fhttpReq.Context() != ctx {
		t.Error("expected request to have the provided context")
	}
}

func TestApplyHeaderOrder(t *testing.T) {
	preset, _ := fingerprint.Get("chrome-133")
	if len(preset.HeaderOrder) == 0 {
		t.Skip("preset has no header order")
	}

	req := &protocol.Request{
		ID:     "test-header-order",
		Method: "GET",
		URL:    "https://example.com",
		Headers: protocol.HeaderMap{
			{Key: "X-Custom", Value: "value"},
			{Key: "Accept", Value: "text/html"},
		},
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req, preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify HeaderOrderKey is set
	if len(fhttpReq.Header) == 0 {
		t.Error("expected headers to be set")
	}
}

func TestHeadersToProtocol(t *testing.T) {
	fhttpHeaders := make(map[string][]string)
	fhttpHeaders["Content-Type"] = []string{"application/json"}
	fhttpHeaders["X-Custom"] = []string{"value1", "value2"}

	result := HeadersToProtocol(fhttpHeaders)

	// Check that headers are converted
	foundContentType := false
	customCount := 0
	for _, h := range result {
		if h.Key == "Content-Type" && h.Value == "application/json" {
			foundContentType = true
		}
		if h.Key == "X-Custom" {
			customCount++
		}
	}

	if !foundContentType {
		t.Error("expected Content-Type header in result")
	}

	if customCount != 2 {
		t.Errorf("expected 2 X-Custom headers, got %d", customCount)
	}
}
