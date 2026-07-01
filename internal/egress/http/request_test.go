package http

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/protocol/wirepb"
)

func TestBuildRequest_Basic(t *testing.T) {
	req := &wirepb.Request{
		Id:     "test-123",
		Method: getMethod,
		Url:    "https://example.com/path",
		Headers: []*wirepb.Header{
			{Key: "X-Custom-Header", Value: "custom-value"},
		},
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fhttpReq.Method != http.MethodGet {
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
	body := []byte(`{"key": "value"}`)
	req := &wirepb.Request{
		Id:     "test-body",
		Method: "POST",
		Url:    "https://example.com/api",
		Body:   body,
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fhttpReq.Method != http.MethodPost {
		t.Errorf("expected method POST, got %s", fhttpReq.Method)
	}

	if fhttpReq.Body == nil {
		t.Error("expected non-nil body")
	}
}

func TestBuildRequest_InvalidURL(t *testing.T) {
	req := &wirepb.Request{
		Id:     "test-invalid",
		Method: getMethod,
		Url:    "://invalid-url",
	}

	ctx := context.Background()
	_, err := BuildRequest(ctx, req)
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

func TestBuildRequest_AppliesDefaultChromeHeaders(t *testing.T) {
	req := &wirepb.Request{
		Id:     "test-fp-headers",
		Method: getMethod,
		Url:    exampleURL,
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fhttpReq.Header.Get("User-Agent") != chromeUserAgent {
		t.Errorf("expected User-Agent %q, got %q", chromeUserAgent, fhttpReq.Header.Get("User-Agent"))
	}

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
	req := &wirepb.Request{
		Id:     "test-no-override",
		Method: getMethod,
		Url:    exampleURL,
		Headers: []*wirepb.Header{
			{Key: userAgentHeader, Value: "Custom-Agent"},
			{Key: acceptHeader, Value: "text/plain"},
		},
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fhttpReq.Header.Get("User-Agent") != "Custom-Agent" {
		t.Errorf("expected User-Agent 'Custom-Agent', got %q", fhttpReq.Header.Get("User-Agent"))
	}

	if fhttpReq.Header.Get("Accept") != "text/plain" {
		t.Errorf("expected Accept 'text/plain', got %q", fhttpReq.Header.Get("Accept"))
	}
}

func TestBuildRequest_WithContext(t *testing.T) {
	req := &wirepb.Request{
		Id:     "test-context",
		Method: "GET",
		Url:    "https://example.com",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fhttpReq, err := BuildRequest(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fhttpReq.Context() != ctx {
		t.Error("expected request to have the provided context")
	}
}

func TestApplyHeaderOrder(t *testing.T) {
	req := &wirepb.Request{
		Id:     "test-header-order",
		Method: "GET",
		Url:    "https://example.com",
		Headers: []*wirepb.Header{
			{Key: "X-Custom", Value: "value"},
			{Key: "Accept", Value: "text/html"},
		},
	}

	ctx := context.Background()
	fhttpReq, err := BuildRequest(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fhttpReq.Header) == 0 {
		t.Error("expected headers to be set")
	}
}

func TestHeadersToProtocol(t *testing.T) {
	fhttpHeaders := make(map[string][]string)
	fhttpHeaders["Content-Type"] = []string{"application/json"}
	fhttpHeaders["X-Custom"] = []string{"value1", "value2"}

	result := HeadersToProtocol(fhttpHeaders)

	foundContentType := false
	customCount := 0
	for _, h := range result {
		if h.GetKey() == "Content-Type" && h.GetValue() == "application/json" {
			foundContentType = true
		}
		if h.GetKey() == "X-Custom" {
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
