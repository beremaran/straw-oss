package domain

import (
	"testing"

	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

func TestDomainError_Error(t *testing.T) {
	err := &DomainError{
		Code:    "TEST_ERROR",
		Message: "This is a test error",
	}

	want := "TEST_ERROR: This is a test error"
	if got := err.Error(); got != want {
		t.Errorf("DomainError.Error() = %q, want %q", got, want)
	}
}

func TestDomainError_ToResponse(t *testing.T) {
	err := ErrAuthInvalid

	resp := err.ToResponse("req-123", "trace-456")

	if resp.Error.Code != protocol.ErrCodeAuthInvalid {
		t.Errorf("ToResponse() Code = %s, want %s", resp.Error.Code, protocol.ErrCodeAuthInvalid)
	}
	if resp.Error.RequestID != "req-123" {
		t.Errorf("ToResponse() RequestID = %s, want req-123", resp.Error.RequestID)
	}
	if resp.Error.TraceID != "trace-456" {
		t.Errorf("ToResponse() TraceID = %s, want trace-456", resp.Error.TraceID)
	}
	if resp.Error.Retryable != false {
		t.Errorf("ToResponse() Retryable = %v, want false", resp.Error.Retryable)
	}
}

func TestIsDomainError(t *testing.T) {
	domainErr := ErrAuthInvalid
	regularErr := &struct{ error }{}

	if !IsDomainError(domainErr) {
		t.Error("IsDomainError() returned false for DomainError")
	}

	if IsDomainError(regularErr) {
		t.Error("IsDomainError() returned true for non-DomainError")
	}
}

func TestAsDomainError(t *testing.T) {
	domainErr := ErrRateLimitExceeded

	de, ok := AsDomainError(domainErr)
	if !ok {
		t.Error("AsDomainError() returned false for DomainError")
	}
	if de.Code != protocol.ErrCodeRateLimitExceeded {
		t.Errorf("AsDomainError() Code = %s, want %s", de.Code, protocol.ErrCodeRateLimitExceeded)
	}
}

func TestNewRateLimitError(t *testing.T) {
	err := NewRateLimitError("target:amazon", 30)

	if err.Code != protocol.ErrCodeRateLimitExceeded {
		t.Errorf("NewRateLimitError() Code = %s, want %s", err.Code, protocol.ErrCodeRateLimitExceeded)
	}
	if !err.Retryable {
		t.Error("NewRateLimitError() Retryable = false, want true")
	}
}

func TestNewUpstreamError(t *testing.T) {
	tests := []struct {
		name         string
		targetStatus int
		wantRetry    bool
	}{
		{name: "5xx retryable", targetStatus: 500, wantRetry: true},
		{name: "4xx not retryable", targetStatus: 403, wantRetry: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewUpstreamError(tt.targetStatus, "test")
			if err.Retryable != tt.wantRetry {
				t.Errorf("NewUpstreamError() Retryable = %v, want %v", err.Retryable, tt.wantRetry)
			}
		})
	}
}

func TestPredefinedErrors(t *testing.T) {
	errors := []*DomainError{
		ErrAuthInvalid,
		ErrAuthForbidden,
		ErrRateLimitExceeded,
		ErrNoEndpointsAvailable,
		ErrEndpointTimeout,
		ErrUpstreamError,
		ErrSessionExpired,
		ErrInternalError,
		ErrSessionMigrationLimit,
		ErrNoMatchingRule,
	}

	for _, err := range errors {
		if err.Code == "" {
			t.Errorf("DomainError has empty Code: %+v", err)
		}
		if err.Message == "" {
			t.Errorf("DomainError has empty Message: %+v", err)
		}
		if err.HTTPCode == 0 {
			t.Errorf("DomainError has zero HTTPCode: %+v", err)
		}
	}
}
