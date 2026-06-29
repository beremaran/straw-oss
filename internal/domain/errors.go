package domain

import (
	"fmt"
	"net/http"

	"github.com/beremaran/straw/pkg/protocol"
)

// ErrorResponse is the top-level error response structure.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail provides structured error information for API responses.
type ErrorDetail struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	TraceID           string `json:"trace_id,omitempty"`
}

// StrawError represents a domain error with an HTTP status code.
type StrawError struct {
	Code      string
	Message   string
	Retryable bool
	HTTPCode  int
}

// NewRateLimitError creates a rate limit exceeded error for the given quota key.
func NewRateLimitError(quotaKey string, _ int) *StrawError {
	return &StrawError{
		Code:      protocol.ErrCodeRateLimitExceeded,
		Message:   fmt.Sprintf("Rate limit exceeded for quota key '%s'", quotaKey),
		Retryable: true,
		HTTPCode:  http.StatusTooManyRequests,
	}
}

// NewUpstreamError creates an error for upstream failures, marking it retryable for server errors.
func NewUpstreamError(targetStatus int, message string) *StrawError {
	return &StrawError{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   message,
		Retryable: targetStatus >= http.StatusInternalServerError,
		HTTPCode:  http.StatusBadGateway,
	}
}

func (e *StrawError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ToResponse converts the StrawError into an ErrorResponse for the given request and trace IDs.
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

var (
	// ErrAuthInvalid indicates the provided API key is invalid or missing.
	ErrAuthInvalid = &StrawError{
		Code:      protocol.ErrCodeAuthInvalid,
		Message:   "Invalid or missing API key",
		Retryable: false,
		HTTPCode:  http.StatusUnauthorized,
	}

	// ErrAuthForbidden indicates the API key lacks permission for the requested tags.
	ErrAuthForbidden = &StrawError{
		Code:      protocol.ErrCodeAuthForbidden,
		Message:   "API key lacks permission for requested tags",
		Retryable: false,
		HTTPCode:  http.StatusForbidden,
	}

	// ErrRateLimitExceeded indicates the rate limit has been exceeded.
	ErrRateLimitExceeded = &StrawError{
		Code:      protocol.ErrCodeRateLimitExceeded,
		Message:   "Rate limit exceeded",
		Retryable: true,
		HTTPCode:  http.StatusTooManyRequests,
	}

	// ErrNoEndpointsAvailable indicates no healthy endpoints match the requested tags.
	ErrNoEndpointsAvailable = &StrawError{
		Code:      protocol.ErrCodeNoEndpointsAvailable,
		Message:   "No healthy endpoints available for the requested tags",
		Retryable: true,
		HTTPCode:  http.StatusServiceUnavailable,
	}

	// ErrEndpointTimeout indicates an endpoint did not respond in time.
	ErrEndpointTimeout = &StrawError{
		Code:      protocol.ErrCodeEndpointTimeout,
		Message:   "Endpoint did not respond in time",
		Retryable: true,
		HTTPCode:  http.StatusGatewayTimeout,
	}

	// ErrUpstreamError indicates the target website returned an error.
	ErrUpstreamError = &StrawError{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "Target website returned an error",
		Retryable: false,
		HTTPCode:  http.StatusBadGateway,
	}

	// ErrSessionExpired indicates the session has expired or does not exist.
	ErrSessionExpired = &StrawError{
		Code:      protocol.ErrCodeSessionExpired,
		Message:   "Session has expired or does not exist",
		Retryable: false,
		HTTPCode:  http.StatusGone,
	}

	// ErrInternalError indicates an internal server error.
	ErrInternalError = &StrawError{
		Code:      protocol.ErrCodeInternalError,
		Message:   "Internal server error",
		Retryable: true,
		HTTPCode:  http.StatusInternalServerError,
	}
)

// ErrSessionMigrationLimit indicates the session has exceeded maximum migration attempts.
var ErrSessionMigrationLimit = &StrawError{
	Code:      "SESSION_MIGRATION_LIMIT",
	Message:   "Session has exceeded maximum migration attempts",
	Retryable: false,
	HTTPCode:  http.StatusGone,
}

// ErrNoMatchingRule indicates no routing rule matches the provided tags.
var ErrNoMatchingRule = &StrawError{
	Code:      "NO_MATCHING_RULE",
	Message:   "No routing rule matches the provided tags",
	Retryable: false,
	HTTPCode:  http.StatusNotFound,
}
