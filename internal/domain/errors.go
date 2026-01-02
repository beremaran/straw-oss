package domain

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// ErrorResponse represents a standardized error response for clients.
// This matches the schema defined in Section 5.1 of the design document.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the detailed error information.
type ErrorDetail struct {
	// Code is a machine-readable error code.
	Code string `json:"code"`

	// Message is a human-readable error description.
	Message string `json:"message"`

	// Retryable indicates whether the client should retry the request.
	Retryable bool `json:"retryable"`

	// RetryAfterSeconds suggests how long to wait before retrying (optional).
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`

	// RequestID is the unique identifier for this request.
	RequestID string `json:"request_id,omitempty"`

	// TraceID is the distributed tracing ID.
	TraceID string `json:"trace_id,omitempty"`
}

// StrawError is a domain-specific error that implements the error interface.
type StrawError struct {
	Code      string
	Message   string
	Retryable bool
	HTTPCode  int
}

// Error implements the error interface.
func (e *StrawError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ToResponse converts the error to an ErrorResponse for API responses.
func (e *StrawError) ToResponse(requestID, traceID string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorDetail{
			Code:      e.Code,
			Message:   e.Message,
			Retryable: e.Retryable,
			RequestID: requestID,
			TraceID:   traceID,
		},
	}
}

// Common domain errors using codes from pkg/protocol.
var (
	ErrAuthInvalid = &StrawError{
		Code:      protocol.ErrCodeAuthInvalid,
		Message:   "Invalid or missing API key",
		Retryable: false,
		HTTPCode:  http.StatusUnauthorized,
	}

	ErrAuthForbidden = &StrawError{
		Code:      protocol.ErrCodeAuthForbidden,
		Message:   "API key lacks permission for requested tags",
		Retryable: false,
		HTTPCode:  http.StatusForbidden,
	}

	ErrRateLimitExceeded = &StrawError{
		Code:      protocol.ErrCodeRateLimitExceeded,
		Message:   "Rate limit exceeded",
		Retryable: true,
		HTTPCode:  http.StatusTooManyRequests,
	}

	ErrNoEndpointsAvailable = &StrawError{
		Code:      protocol.ErrCodeNoEndpointsAvailable,
		Message:   "No healthy endpoints available for the requested tags",
		Retryable: true,
		HTTPCode:  http.StatusServiceUnavailable,
	}

	ErrEndpointTimeout = &StrawError{
		Code:      protocol.ErrCodeEndpointTimeout,
		Message:   "Endpoint did not respond in time",
		Retryable: true,
		HTTPCode:  http.StatusGatewayTimeout,
	}

	ErrUpstreamError = &StrawError{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "Target website returned an error",
		Retryable: false,
		HTTPCode:  http.StatusBadGateway,
	}

	ErrSessionExpired = &StrawError{
		Code:      protocol.ErrCodeSessionExpired,
		Message:   "Session has expired or does not exist",
		Retryable: false,
		HTTPCode:  http.StatusGone,
	}

	ErrInternalError = &StrawError{
		Code:      protocol.ErrCodeInternalError,
		Message:   "Internal server error",
		Retryable: true,
		HTTPCode:  http.StatusInternalServerError,
	}
)

// ErrSessionMigrationLimit is returned when a session exceeds the max migration count.
var ErrSessionMigrationLimit = &StrawError{
	Code:      "SESSION_MIGRATION_LIMIT",
	Message:   "Session has exceeded maximum migration attempts",
	Retryable: false,
	HTTPCode:  http.StatusGone,
}

// ErrNoMatchingRule is returned when no routing rule matches the request tags.
var ErrNoMatchingRule = &StrawError{
	Code:      "NO_MATCHING_RULE",
	Message:   "No routing rule matches the provided tags",
	Retryable: false,
	HTTPCode:  http.StatusNotFound,
}

// NewRateLimitError creates a rate limit error with retry-after information.
func NewRateLimitError(quotaKey string, retryAfterSeconds int) *StrawError {
	return &StrawError{
		Code:      protocol.ErrCodeRateLimitExceeded,
		Message:   fmt.Sprintf("Rate limit exceeded for quota key '%s'", quotaKey),
		Retryable: true,
		HTTPCode:  http.StatusTooManyRequests,
	}
}

// NewUpstreamError creates an upstream error with the target status code.
func NewUpstreamError(targetStatus int, message string) *StrawError {
	return &StrawError{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   message,
		Retryable: targetStatus >= 500,
		HTTPCode:  http.StatusBadGateway,
	}
}

// IsDomainError checks if an error is a StrawError.
func IsDomainError(err error) bool {
	var strawError *StrawError
	ok := errors.As(err, &strawError)
	return ok
}

// AsDomainError converts an error to a StrawError if possible.
func AsDomainError(err error) (*StrawError, bool) {
	var de *StrawError
	ok := errors.As(err, &de)
	return de, ok
}
