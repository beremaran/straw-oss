package dto

import (
	"net/http"
	"testing"
)

func TestToProtocolRequest(t *testing.T) {
	req := &ControlRequest{
		ID:              "test-req-1",
		Method:          http.MethodPost,
		URL:             "https://example.com/api",
		Headers:         map[string]string{"X-Custom": "value", "Content-Type": "application/json"},
		Body:            []byte(`{"key":"value"}`),
		Timeout:         "30s",
		MaxResponseSize: 1048576,
	}

	protoReq, err := req.ToProtocolRequest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if protoReq.ID != "test-req-1" {
		t.Errorf("ID = %q, want test-req-1", protoReq.ID)
	}
	if protoReq.Method != http.MethodPost {
		t.Errorf("Method = %q, want POST", protoReq.Method)
	}
	if protoReq.URL != "https://example.com/api" {
		t.Errorf("URL = %q, want https://example.com/api", protoReq.URL)
	}
	if len(protoReq.Headers) != 2 {
		t.Errorf("Headers count = %d, want 2", len(protoReq.Headers))
	}
	if string(protoReq.Body) != `{"key":"value"}` {
		t.Errorf("Body = %q, want {\"key\":\"value\"}", protoReq.Body)
	}
}

func TestToProtocolRequestWithInvalidTimeout(t *testing.T) {
	req := &ControlRequest{
		ID:      "test-req-2",
		Method:  "GET",
		URL:     "https://example.com",
		Timeout: "not-a-duration",
	}

	_, err := req.ToProtocolRequest()
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestToProtocolRequestEmpty(t *testing.T) {
	req := &ControlRequest{
		ID:      "test-req-3",
		Method:  "GET",
		URL:     "https://example.com",
		Headers: map[string]string{},
	}

	protoReq, err := req.ToProtocolRequest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(protoReq.Headers) != 0 {
		t.Errorf("Headers count = %d, want 0", len(protoReq.Headers))
	}
}
