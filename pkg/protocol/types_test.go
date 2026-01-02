package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRequest_JSONRoundTrip(t *testing.T) {
	req := &Request{
		ID:          "req-12345",
		Method:      "POST",
		URL:         "https://example.com/api/data",
		Headers:     HeaderMap{{Key: "Content-Type", Value: "application/json"}},
		Body:        []byte(`{"foo":"bar"}`),
		Timeout:     30 * time.Second,
		Fingerprint: "chrome-130",
		SessionID:   "sess-abc",
		TraceID:     "trace-xyz",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ID != req.ID {
		t.Errorf("ID mismatch: %s != %s", decoded.ID, req.ID)
	}
	if decoded.Method != req.Method {
		t.Errorf("Method mismatch: %s != %s", decoded.Method, req.Method)
	}
	if decoded.URL != req.URL {
		t.Errorf("URL mismatch: %s != %s", decoded.URL, req.URL)
	}
	if decoded.Fingerprint != req.Fingerprint {
		t.Errorf("Fingerprint mismatch: %s != %s", decoded.Fingerprint, req.Fingerprint)
	}
	if decoded.Timeout != req.Timeout {
		t.Errorf("Timeout mismatch: %v != %v", decoded.Timeout, req.Timeout)
	}
}

func TestResponse_JSONRoundTrip(t *testing.T) {
	resp := &Response{
		RequestID:  "req-12345",
		StatusCode: 200,
		Headers: HeaderMap{
			{Key: "Content-Type", Value: "application/json"},
			{Key: "X-Request-Id", Value: "abc123"},
		},
		Body:       []byte(`{"result":"success"}`),
		EndpointID: "endpoint-us-1",
		SessionID:  "sess-abc",
		Timing: &TimingInfo{
			DNSLookup:    10 * time.Millisecond,
			TCPConnect:   20 * time.Millisecond,
			TLSHandshake: 50 * time.Millisecond,
			FirstByte:    100 * time.Millisecond,
			Total:        500 * time.Millisecond,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.RequestID != resp.RequestID {
		t.Errorf("RequestID mismatch")
	}
	if decoded.StatusCode != resp.StatusCode {
		t.Errorf("StatusCode mismatch")
	}
	if decoded.Timing.Total != resp.Timing.Total {
		t.Errorf("Timing.Total mismatch")
	}
}

func TestResponse_WithError(t *testing.T) {
	resp := &Response{
		RequestID:  "req-12345",
		StatusCode: 403,
		Error: &ErrorInfo{
			Code:       ErrCodeAuthForbidden,
			Message:    "API key lacks permission for requested tags",
			Retryable:  false,
			RetryAfter: 0,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Error == nil {
		t.Fatal("expected error to be present")
	}
	if decoded.Error.Code != ErrCodeAuthForbidden {
		t.Errorf("Error.Code mismatch: %s", decoded.Error.Code)
	}
}

func TestHeaderMap_Get(t *testing.T) {
	headers := HeaderMap{
		{Key: "Content-Type", Value: "application/json"},
		{Key: "X-Custom-Header", Value: "custom-value"},
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"Content-Type", "application/json"},
		{"content-type", "application/json"}, // case-insensitive
		{"CONTENT-TYPE", "application/json"}, // case-insensitive
		{"X-Custom-Header", "custom-value"},
		{"x-custom-header", "custom-value"},
		{"NonExistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := headers.Get(tt.key)
			if result != tt.expected {
				t.Errorf("Get(%s) = %s, expected %s", tt.key, result, tt.expected)
			}
		})
	}
}

func TestHeaderMap_Set(t *testing.T) {
	headers := HeaderMap{
		{Key: "Content-Type", Value: "text/html"},
	}

	// Update existing
	headers.Set("Content-Type", "application/json")
	if headers.Get("Content-Type") != "application/json" {
		t.Error("Set did not update existing header")
	}

	// Add new
	headers.Set("X-New-Header", "new-value")
	if headers.Get("X-New-Header") != "new-value" {
		t.Error("Set did not add new header")
	}

	if len(headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(headers))
	}
}

func TestHeaderMap_Del(t *testing.T) {
	headers := HeaderMap{
		{Key: "Content-Type", Value: "application/json"},
		{Key: "X-To-Delete", Value: "delete-me"},
		{Key: "X-Keep", Value: "keep-me"},
	}

	headers.Del("X-To-Delete")

	if headers.Get("X-To-Delete") != "" {
		t.Error("Del did not remove header")
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Error("Del removed wrong header")
	}
	if headers.Get("X-Keep") != "keep-me" {
		t.Error("Del removed wrong header")
	}
	if len(headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(headers))
	}
}

func TestHeaderMap_Clone(t *testing.T) {
	original := HeaderMap{
		{Key: "Content-Type", Value: "application/json"},
	}

	clone := original.Clone()

	// Modify clone
	clone.Set("Content-Type", "text/html")
	clone.Set("X-New", "new")

	// Original should be unchanged
	if original.Get("Content-Type") != "application/json" {
		t.Error("Clone modified original")
	}
	if len(original) != 1 {
		t.Error("Clone modified original length")
	}
}

func TestHeaderMap_Clone_Nil(t *testing.T) {
	var headers HeaderMap
	clone := headers.Clone()
	if clone != nil {
		t.Error("Clone of nil should be nil")
	}
}

func TestHeaderMap_OrderPreservation(t *testing.T) {
	headers := HeaderMap{
		{Key: "First", Value: "1"},
		{Key: "Second", Value: "2"},
		{Key: "Third", Value: "3"},
	}

	data, err := json.Marshal(headers)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded HeaderMap
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify order is preserved
	for i, h := range headers {
		if decoded[i].Key != h.Key || decoded[i].Value != h.Value {
			t.Errorf("order not preserved at index %d: expected %v, got %v",
				i, h, decoded[i])
		}
	}
}

func TestHeaderMap_UnmarshalJSON_ObjectFormat(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected HeaderMap
	}{
		{
			name: "simple object with string values",
			json: `{"User-Agent": "k6-load-test", "Accept": "application/json"}`,
			expected: HeaderMap{
				{Key: "User-Agent", Value: "k6-load-test"},
				{Key: "Accept", Value: "application/json"},
			},
		},
		{
			name: "object with array values (joined)",
			json: `{"User-Agent": ["k6-load-test"], "Accept": ["application/json", "text/html"]}`,
			expected: HeaderMap{
				{Key: "User-Agent", Value: "k6-load-test"},
				{Key: "Accept", Value: "application/json, text/html"},
			},
		},
		{
			name:     "empty object",
			json:     `{}`,
			expected: HeaderMap{},
		},
		{
			name: "mixed string and array values",
			json: `{"User-Agent": "test", "Accept": ["json", "xml"]}`,
			expected: HeaderMap{
				{Key: "User-Agent", Value: "test"},
				{Key: "Accept", Value: "json, xml"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var headers HeaderMap
			if err := json.Unmarshal([]byte(tt.json), &headers); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if len(headers) != len(tt.expected) {
				t.Errorf("expected %d headers, got %d", len(tt.expected), len(headers))
			}

			for i, h := range tt.expected {
				if i >= len(headers) {
					t.Errorf("missing header at index %d", i)
					continue
				}
				if headers[i].Key != h.Key {
					t.Errorf("header %d: key mismatch, expected %s, got %s", i, h.Key, headers[i].Key)
				}
				if headers[i].Value != h.Value {
					t.Errorf("header %d: value mismatch, expected %s, got %s", i, h.Value, headers[i].Value)
				}
			}
		})
	}
}

func TestHeaderMap_UnmarshalJSON_ArrayFormat(t *testing.T) {
	jsonData := `[{"key": "User-Agent", "value": "k6-load-test"}, {"key": "Accept", "value": "application/json"}]`
	expected := HeaderMap{
		{Key: "User-Agent", Value: "k6-load-test"},
		{Key: "Accept", Value: "application/json"},
	}

	var headers HeaderMap
	if err := json.Unmarshal([]byte(jsonData), &headers); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(headers) != len(expected) {
		t.Errorf("expected %d headers, got %d", len(expected), len(headers))
	}

	for i, h := range expected {
		if headers[i].Key != h.Key || headers[i].Value != h.Value {
			t.Errorf("header %d mismatch: expected %v, got %v", i, h, headers[i])
		}
	}
}

func TestHeaderMap_UnmarshalJSON_PreservesOrder(t *testing.T) {
	// Object format should preserve order
	jsonData := `{"First": "1", "Second": "2", "Third": "3", "Fourth": "4"}`

	var headers HeaderMap
	if err := json.Unmarshal([]byte(jsonData), &headers); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	expectedOrder := []string{"First", "Second", "Third", "Fourth"}
	for i, expectedKey := range expectedOrder {
		if headers[i].Key != expectedKey {
			t.Errorf("order not preserved at index %d: expected %s, got %s", i, expectedKey, headers[i].Key)
		}
	}
}

func TestRequest_UnmarshalJSON_LoadTestFormat(t *testing.T) {
	// This test verifies that the server can accept headers in the format
	// sent by the load test (object format instead of array format)
	jsonData := `{
		"url": "http://example.com",
		"method": "GET",
		"headers": {
			"User-Agent": "k6-load-test",
			"Accept": "application/json"
		}
	}`

	var req Request
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify headers were parsed correctly
	if len(req.Headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(req.Headers))
	}

	if req.Headers[0].Key != "User-Agent" || req.Headers[0].Value != "k6-load-test" {
		t.Errorf("first header mismatch: got %v", req.Headers[0])
	}

	if req.Headers[1].Key != "Accept" || req.Headers[1].Value != "application/json" {
		t.Errorf("second header mismatch: got %v", req.Headers[1])
	}
}

func TestSignedTask_JSONRoundTrip(t *testing.T) {
	task := &SignedTask{
		Payload:   []byte("compressed-payload-data"),
		Signature: "abc123signature",
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded SignedTask
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Signature != task.Signature {
		t.Errorf("Signature mismatch")
	}
	if decoded.Timestamp != task.Timestamp {
		t.Errorf("Timestamp mismatch")
	}
}

func TestErrorInfo_JSONRoundTrip(t *testing.T) {
	errInfo := &ErrorInfo{
		Code:       ErrCodeRateLimitExceeded,
		Message:    "Rate limit exceeded for quota key 'target:amazon'",
		Retryable:  true,
		RetryAfter: 30 * time.Second,
	}

	data, err := json.Marshal(errInfo)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ErrorInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Code != errInfo.Code {
		t.Errorf("Code mismatch")
	}
	if decoded.Retryable != errInfo.Retryable {
		t.Errorf("Retryable mismatch")
	}
	if decoded.RetryAfter != errInfo.RetryAfter {
		t.Errorf("RetryAfter mismatch")
	}
}

func TestTimingInfo_JSONRoundTrip(t *testing.T) {
	timing := &TimingInfo{
		DNSLookup:    10 * time.Millisecond,
		TCPConnect:   25 * time.Millisecond,
		TLSHandshake: 50 * time.Millisecond,
		FirstByte:    100 * time.Millisecond,
		Total:        500 * time.Millisecond,
	}

	data, err := json.Marshal(timing)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded TimingInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.DNSLookup != timing.DNSLookup {
		t.Errorf("DNSLookup mismatch: %v != %v", decoded.DNSLookup, timing.DNSLookup)
	}
	if decoded.Total != timing.Total {
		t.Errorf("Total mismatch: %v != %v", decoded.Total, timing.Total)
	}
}
