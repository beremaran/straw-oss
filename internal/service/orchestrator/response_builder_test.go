package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/pkg/protocol"
)

const (
	contentTypeHeader = "Content-Type"
	primaryPool       = "primary"
	ep001             = "ep-001"
	session456        = "session-456"
	ep1               = "ep-1"
	ep2               = "ep-2"
)

func TestResponseBuilder_New(t *testing.T) {
	rb := NewResponseBuilder()

	if rb == nil {
		t.Fatal("expected response builder to be created")
	}

	if len(rb.FilterHeaders) == 0 {
		t.Error("expected default filtered headers to be set")
	}
}

func TestResponseBuilder_WriteResponse_Success(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:  testID,
		StatusCode: 200,
		Headers: protocol.HeaderMap{
			{Key: contentTypeHeader, Value: jsonContentType},
			{Key: "X-Custom-Header", Value: "custom-value"},
		},
		CompressedBody: []byte(`{"message": "hello"}`),
	}

	meta := &RelayMetadata{
		Retries:    2,
		Pool:       primaryPool,
		EndpointID: ep001,
	}

	rec.Header().Set("X-Request-ID", testID)

	err := rb.WriteResponse(rec, result, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != 200 {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected X-Custom-Header 'custom-value', got %q", rec.Header().Get("X-Custom-Header"))
	}

	if rec.Header().Get("X-Relay-Retries") != "2" {
		t.Errorf("expected X-Relay-Retries '2', got %q", rec.Header().Get("X-Relay-Retries"))
	}

	if rec.Header().Get("X-Relay-Pool") != primaryPool {
		t.Errorf("expected X-Relay-Pool %q, got %q", primaryPool, rec.Header().Get("X-Relay-Pool"))
	}

	if rec.Header().Get("X-Relay-Endpoint") != ep001 {
		t.Errorf("expected X-Relay-Endpoint %q, got %q", ep001, rec.Header().Get("X-Relay-Endpoint"))
	}
}

func TestResponseBuilder_WriteResponse_FilteredHeaders(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:  testID,
		StatusCode: 200,
		Headers: protocol.HeaderMap{
			{Key: contentTypeHeader, Value: jsonContentType},
			{Key: "Connection", Value: "keep-alive"},
			{Key: "Transfer-Encoding", Value: "chunked"},
			{Key: "X-Custom-Header", Value: "should-be-copied"},
		},
		CompressedBody: []byte(`{}`),
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Header().Get("Connection") != "" {
		t.Error("Connection header should be filtered")
	}

	if rec.Header().Get("Transfer-Encoding") != "" {
		t.Error("Transfer-Encoding header should be filtered")
	}

	if rec.Header().Get("X-Custom-Header") != "should-be-copied" {
		t.Error("X-Custom-Header should be copied")
	}
}

func TestResponseBuilder_WriteResponse_SessionHeaders(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:      testID,
		StatusCode:     200,
		CompressedBody: []byte(`{}`),
	}

	meta := &RelayMetadata{
		SessionID:    session456,
		Migrated:     true,
		MigrateCount: 2,
	}

	err := rb.WriteResponse(rec, result, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Header().Get("X-Session-ID") != session456 {
		t.Errorf("expected X-Session-ID 'session-456', got %q", rec.Header().Get("X-Session-ID"))
	}

	if rec.Header().Get("X-Session-Migrated") != "true" {
		t.Error("expected X-Session-Migrated 'true'")
	}

	if rec.Header().Get("X-Session-Migration-Count") != "2" {
		t.Errorf("expected X-Session-Migration-Count '2', got %q", rec.Header().Get("X-Session-Migration-Count"))
	}
}

func TestResponseBuilder_WriteResponse_TimingHeader(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:      testID,
		StatusCode:     200,
		CompressedBody: []byte(`{}`),
	}

	meta := &RelayMetadata{
		Timing: &protocol.TimingInfo{
			Total: 150 * time.Millisecond,
		},
	}

	err := rb.WriteResponse(rec, result, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	timing := rec.Header().Get("X-Relay-Timing")
	if timing != "150ms" {
		t.Errorf("expected X-Relay-Timing '150ms', got %q", timing)
	}
}

func TestResponseBuilder_WriteResponse_DefaultStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:      testID,
		StatusCode:     0,
		CompressedBody: []byte(`{}`),
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != 200 {
		t.Errorf("expected status code 200 (default), got %d", rec.Code)
	}
}

func TestResponseBuilder_WriteResponse_ErrorResponse(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:  testID,
		StatusCode: 502,
		Error: &protocol.ErrorInfo{
			Code:      protocol.ErrCodeUpstreamError,
			Message:   "upstream connection failed",
			Retryable: true,
		},
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != 502 {
		t.Errorf("expected status code 502, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body == "" {
		t.Error("expected error response body")
	}
}

func TestWriteTimeoutResponse(t *testing.T) {
	rec := httptest.NewRecorder()

	err := WriteTimeoutResponse(rec, "req-timeout-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("expected status code 504, got %d", rec.Code)
	}

	if rec.Header().Get("X-Request-ID") != "req-timeout-123" {
		t.Errorf("expected X-Request-ID 'req-timeout-123', got %q", rec.Header().Get("X-Request-ID"))
	}
}

func TestEqualFoldASCII(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{contentTypeHeader, "content-type", true},
		{"CONTENT-TYPE", "content-type", true},
		{contentTypeHeader, contentTypeHeader, true},
		{contentTypeHeader, "Content-Length", false},
		{"abc", "abcd", false},
		{"", "", true},
	}

	for _, tt := range tests {
		result := equalFoldASCII(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("equalFoldASCII(%q, %q) = %v, expected %v", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		maxLen   int
		expected string
	}{
		{
			name:     "short message",
			msg:      "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length",
			msg:      "hello world",
			maxLen:   11,
			expected: "hello world",
		},
		{
			name:     "needs truncation",
			msg:      "this is a very long message that needs truncation",
			maxLen:   10,
			expected: "this is...",
		},
		{
			name:     "empty message",
			msg:      "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateMessage(tt.msg, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateMessage(%q, %d) = %q, expected %q", tt.msg, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestFormatAttemptErrors(t *testing.T) {
	errors := []AttemptError{
		{
			Pool:          1,
			Attempt:       1,
			EndpointID:    ep1,
			Failure:       FailureTimeout,
			FailureString: testFailureTimeoutStr,
			Message:       testRequestTimedOut,
			Duration:      100 * time.Millisecond,
		},
		{
			Pool:          2,
			Attempt:       1,
			EndpointID:    ep2,
			Failure:       FailureConnection,
			FailureString: testFailureConnectionStr,
			Message:       errConnectionRefused,
			Duration:      50 * time.Millisecond,
		},
	}

	summaries := formatAttemptErrors(errors)

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	if summaries[0].Pool != 1 {
		t.Errorf("expected pool 1, got %d", summaries[0].Pool)
	}
	if summaries[0].Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", summaries[0].Attempt)
	}
	if summaries[0].Endpoint != ep1 {
		t.Errorf("expected endpoint 'ep-1', got %q", summaries[0].Endpoint)
	}
	if summaries[0].Failure != testFailureTimeoutStr {
		t.Errorf("expected failure 'timeout', got %q", summaries[0].Failure)
	}

	if len(summaries[0].Message) > 50 {
		t.Errorf("message should be truncated to 50 chars, got %d", len(summaries[0].Message))
	}
}

func TestFormatTiming(t *testing.T) {
	tests := []struct {
		name     string
		timing   *protocol.TimingInfo
		expected string
	}{
		{
			name:     "nil timing",
			timing:   nil,
			expected: "",
		},
		{
			name: "simple timing",
			timing: &protocol.TimingInfo{
				Total: 150 * time.Millisecond,
			},
			expected: "150ms",
		},
		{
			name: "microsecond precision",
			timing: &protocol.TimingInfo{
				Total: 123456 * time.Microsecond,
			},
			expected: "123ms",
		},
		{
			name: "nanosecond precision",
			timing: &protocol.TimingInfo{
				Total: 123456789 * time.Nanosecond,
			},
			expected: "123ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTiming(tt.timing)
			if result != tt.expected {
				t.Errorf("formatTiming() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestResponseBuilder_WriteResponse_RetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:  testID,
		StatusCode: 429,
		Error: &protocol.ErrorInfo{
			Code:       protocol.ErrCodeUpstreamError,
			Message:    "rate limited",
			Retryable:  true,
			RetryAfter: 60 * time.Second,
		},
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter != "60" {
		t.Errorf("expected Retry-After '60', got %q", retryAfter)
	}

	body := rec.Body.String()
	if !contains(body, "retry_after_seconds") {
		t.Error("expected response body to contain retry_after_seconds")
	}
}

func TestResponseBuilder_WriteResponse_AttemptErrors(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:      testID,
		StatusCode:     200,
		CompressedBody: []byte(`{}`),
	}

	meta := &RelayMetadata{
		AttemptErrors: []AttemptError{
			{
				Pool:          1,
				Attempt:       1,
				EndpointID:    ep1,
				Failure:       FailureTimeout,
				FailureString: testFailureTimeoutStr,
				Message:       testRequestTimedOut,
			},
			{
				Pool:          1,
				Attempt:       2,
				EndpointID:    ep2,
				Failure:       FailureConnection,
				FailureString: testFailureConnectionStr,
				Message:       errConnectionRefused,
			},
		},
	}

	err := rb.WriteResponse(rec, result, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attemptErrors := rec.Header().Get("X-Relay-Attempt-Errors")
	if attemptErrors == "" {
		t.Error("expected X-Relay-Attempt-Errors header to be set")
	}
}

func TestResponseBuilder_WriteResponse_DefaultStatusCodeWithError(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:  testID,
		StatusCode: 0,
		Error: &protocol.ErrorInfo{
			Code:      protocol.ErrCodeUpstreamError,
			Message:   "upstream error",
			Retryable: true,
		},
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected status code 502 (BadGateway), got %d", rec.Code)
	}
}

func TestResponseBuilder_WriteResponse_NilMetadata(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:      testID,
		StatusCode:     200,
		CompressedBody: []byte(`{"test": true}`),
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != 200 {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestResponseBuilder_WriteResponse_AllRelayHeaders(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:      testID,
		StatusCode:     200,
		CompressedBody: []byte(`{}`),
	}

	meta := &RelayMetadata{
		Retries:      3,
		Pool:         primaryPool,
		EndpointID:   ep001,
		SessionID:    session456,
		Migrated:     true,
		MigrateCount: 2,
		Timing: &protocol.TimingInfo{
			Total: 250 * time.Millisecond,
		},
		AttemptErrors: []AttemptError{
			{
				Pool:          1,
				Attempt:       1,
				EndpointID:    ep001,
				Failure:       FailureTimeout,
				FailureString: testFailureTimeoutStr,
				Message:       testRequestTimedOut,
				Duration:      100 * time.Millisecond,
			},
		},
	}

	err := rb.WriteResponse(rec, result, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Header().Get("X-Relay-Retries") != "3" {
		t.Error("expected X-Relay-Retries header")
	}

	if rec.Header().Get("X-Relay-Pool") != primaryPool {
		t.Error("expected X-Relay-Pool header")
	}

	if rec.Header().Get("X-Relay-Endpoint") != ep001 {
		t.Error("expected X-Relay-Endpoint header")
	}

	if rec.Header().Get("X-Session-ID") != session456 {
		t.Error("expected X-Session-ID header")
	}

	if rec.Header().Get("X-Session-Migrated") != "true" {
		t.Error("expected X-Session-Migrated header")
	}

	if rec.Header().Get("X-Session-Migration-Count") != "2" {
		t.Error("expected X-Session-Migration-Count header")
	}

	if rec.Header().Get("X-Relay-Timing") == "" {
		t.Error("expected X-Relay-Timing header")
	}

	if rec.Header().Get("X-Relay-Attempt-Errors") == "" {
		t.Error("expected X-Relay-Attempt-Errors header")
	}
}

func TestResponseBuilder_WriteResponse_EmptyBody(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:      testID,
		StatusCode:     204,
		CompressedBody: []byte{},
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != 204 {
		t.Errorf("expected status code 204, got %d", rec.Code)
	}

	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body, got %d bytes", rec.Body.Len())
	}
}

func TestResponseBuilder_WriteResponse_ErrorWithoutRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()

	rb := NewResponseBuilder()

	result := &ResultMessage{
		RequestID:  testID,
		StatusCode: 500,
		Error: &protocol.ErrorInfo{
			Code:      protocol.ErrCodeUpstreamError,
			Message:   "server error",
			Retryable: true,
		},
	}

	err := rb.WriteResponse(rec, result, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Header().Get("Retry-After") != "" {
		t.Error("expected no Retry-After header")
	}

	body := rec.Body.String()
	if contains(body, "retry_after_seconds") {
		t.Error("expected no retry_after_seconds in body")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
