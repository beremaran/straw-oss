// Package orchestrator provides task orchestration for the Relay Server.
// This file implements failure classification for retry decisions.
package orchestrator

import (
	"net/http"
	"strings"

	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// FailureType categorizes endpoint response failures for retry decisions.
// This follows Section 8.3 of the design document.
type FailureType int

const (
	// FailureNone indicates no failure occurred.
	FailureNone FailureType = iota

	// FailureTimeout indicates a network timeout.
	// Action: Retry, then escalate after max retries.
	FailureTimeout

	// FailureConnection indicates a TCP connection failure (refused, reset, etc.).
	// Action: Retry, then escalate after max retries.
	FailureConnection

	// FailureRateLimited indicates HTTP 429 from the target.
	// Action: Retry with backoff, do NOT escalate (rate limit applies across pools).
	FailureRateLimited

	// FailureBlocked indicates HTTP 403/Captcha from the target.
	// Action: Immediate escalation to next pool (same endpoint type won't help).
	FailureBlocked

	// FailureUpstream indicates HTTP 5xx from the target.
	// Action: Retry, then escalate after max retries.
	FailureUpstream

	// FailureInternal indicates an internal endpoint error.
	// Action: Retry, then escalate after max retries.
	FailureInternal
)

// String returns a human-readable name for the failure type.
func (f FailureType) String() string {
	switch f {
	case FailureNone:
		return "none"
	case FailureTimeout:
		return "timeout"
	case FailureConnection:
		return "connection"
	case FailureRateLimited:
		return "rate_limited"
	case FailureBlocked:
		return "blocked"
	case FailureUpstream:
		return "upstream"
	case FailureInternal:
		return "internal"
	default:
		return "unknown"
	}
}

// ShouldRetry returns whether this failure type should be retried within the same pool.
// Some failures like FailureBlocked should immediately escalate instead.
func (f FailureType) ShouldRetry() bool {
	switch f {
	case FailureNone:
		return false // No failure, no retry needed
	case FailureBlocked:
		return false // 403/Captcha should immediately escalate
	case FailureTimeout, FailureConnection, FailureUpstream, FailureInternal:
		return true // Retry within pool
	case FailureRateLimited:
		return true // Retry with backoff
	default:
		return false
	}
}

// ShouldEscalate returns whether this failure should immediately escalate to the next pool.
// This is true for failures where retrying with the same pool type won't help.
func (f FailureType) ShouldEscalate() bool {
	return f == FailureBlocked
}

// RequiresBackoff returns whether this failure type requires exponential backoff before retry.
func (f FailureType) RequiresBackoff() bool {
	return f == FailureRateLimited
}

// ClassifyFailure determines the failure type from a result message.
// It examines the error info and status code to classify the failure.
func ClassifyFailure(result *ResultMessage) FailureType {
	if result == nil {
		return FailureInternal
	}

	// Check for error info first
	if result.Error != nil {
		return classifyErrorInfo(result.Error)
	}

	// Classify by status code
	return classifyStatusCode(result.StatusCode)
}

// classifyErrorInfo classifies failure based on error information.
func classifyErrorInfo(errInfo *protocol.ErrorInfo) FailureType {
	if errInfo == nil {
		return FailureNone
	}

	switch errInfo.Code {
	case protocol.ErrCodeEndpointTimeout:
		return FailureTimeout
	case protocol.ErrCodeInternalError:
		return FailureInternal
	case protocol.ErrCodeUpstreamError:
		// Check message for more specific classification
		msg := strings.ToLower(errInfo.Message)
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
			return FailureTimeout
		}
		if strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") {
			return FailureConnection
		}
		return FailureUpstream
	default:
		return FailureInternal
	}
}

// classifyStatusCode classifies failure based on HTTP status code.
func classifyStatusCode(statusCode int) FailureType {
	switch {
	case statusCode == 0:
		// No response received - likely connection/timeout issue
		return FailureConnection
	case statusCode >= 200 && statusCode < 400:
		// Success or redirect - no failure
		return FailureNone
	case statusCode == http.StatusForbidden:
		// 403 - likely blocked/captcha
		return FailureBlocked
	case statusCode == http.StatusTooManyRequests:
		// 429 - rate limited by target
		return FailureRateLimited
	case statusCode >= 500:
		// 5xx - upstream server error
		return FailureUpstream
	default:
		// Other 4xx errors - client errors, don't retry
		return FailureNone
	}
}

// IsRetryableStatusCode returns whether the status code indicates a retryable failure.
func IsRetryableStatusCode(statusCode int) bool {
	failure := classifyStatusCode(statusCode)
	return failure.ShouldRetry()
}

// IsBlockedResponse checks if the response indicates the request was blocked.
// This includes 403 status and common captcha indicators.
func IsBlockedResponse(result *ResultMessage) bool {
	if result == nil {
		return false
	}

	// Check status code
	if result.StatusCode == http.StatusForbidden {
		return true
	}

	// Check for captcha indicators in headers
	if result.Headers != nil {
		contentType := result.Headers.Get("Content-Type")
		if strings.Contains(strings.ToLower(contentType), "captcha") {
			return true
		}
	}

	return false
}
