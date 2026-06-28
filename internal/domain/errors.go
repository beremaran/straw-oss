//nolint:funcorder
package domain

import (
	"fmt"
	"net/http"

	"github.com/beremaran/straw/pkg/protocol"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code string `json:"code"`

	Message string `json:"message"`

	Retryable bool `json:"retryable"`

	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`

	RequestID string `json:"request_id,omitempty"`

	TraceID string `json:"trace_id,omitempty"`
}

type StrawError struct {
	Code      string
	Message   string
	Retryable bool
	HTTPCode  int
}

func (e *StrawError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

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

var ErrSessionMigrationLimit = &StrawError{
	Code:      "SESSION_MIGRATION_LIMIT",
	Message:   "Session has exceeded maximum migration attempts",
	Retryable: false,
	HTTPCode:  http.StatusGone,
}

var ErrNoMatchingRule = &StrawError{
	Code:      "NO_MATCHING_RULE",
	Message:   "No routing rule matches the provided tags",
	Retryable: false,
	HTTPCode:  http.StatusNotFound,
}

func NewRateLimitError(quotaKey string, retryAfterSeconds int) *StrawError {
	return &StrawError{
		Code:      protocol.ErrCodeRateLimitExceeded,
		Message:   fmt.Sprintf("Rate limit exceeded for quota key '%s'", quotaKey),
		Retryable: true,
		HTTPCode:  http.StatusTooManyRequests,
	}
}

func NewUpstreamError(targetStatus int, message string) *StrawError {
	return &StrawError{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   message,
		Retryable: targetStatus >= 500,
		HTTPCode:  http.StatusBadGateway,
	}
}
