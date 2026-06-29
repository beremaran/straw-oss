package orchestrator

import (
	"net/http"
	"strings"

	"github.com/beremaran/straw/pkg/protocol"
)

// FailureType classifies the kind of failure encountered during request processing.
type FailureType int

const (
	// FailureNone indicates no failure occurred.
	FailureNone FailureType = iota

	// FailureTimeout indicates the request timed out.
	FailureTimeout

	// FailureConnection indicates a connection failure.
	FailureConnection

	// FailureRateLimited indicates the request was rate limited.
	FailureRateLimited

	// FailureBlocked indicates the request was blocked (e.g. captcha).
	FailureBlocked

	// FailureUpstream indicates an upstream server error.
	FailureUpstream

	// FailureInternal indicates an internal error.
	FailureInternal
)

const (
	strNone        = "none"
	strTimeout     = "timeout"
	strConnection  = "connection"
	strRateLimited = "rate_limited"
	strBlocked     = "blocked"
	strUpstream    = "upstream"
	strInternal    = "internal"
	strUnknown     = "unknown"
)

func (f FailureType) String() string {
	switch f {
	case FailureNone:
		return strNone
	case FailureTimeout:
		return strTimeout
	case FailureConnection:
		return strConnection
	case FailureRateLimited:
		return strRateLimited
	case FailureBlocked:
		return strBlocked
	case FailureUpstream:
		return strUpstream
	case FailureInternal:
		return strInternal
	default:
		return strUnknown
	}
}

// ShouldRetry returns whether the failure is retryable.
func (f FailureType) ShouldRetry() bool {
	switch f {
	case FailureNone:
		return false
	case FailureBlocked:
		return false
	case FailureTimeout, FailureConnection, FailureUpstream, FailureInternal:
		return true
	case FailureRateLimited:
		return true
	default:
		return false
	}
}

// ShouldEscalate returns whether the failure requires immediate escalation.
func (f FailureType) ShouldEscalate() bool {
	return f == FailureBlocked
}

// RequiresBackoff returns whether the failure requires a backoff delay before retry.
func (f FailureType) RequiresBackoff() bool {
	return f == FailureRateLimited
}

// ClassifyFailure determines the FailureType from a ResultMessage.
func ClassifyFailure(result *ResultMessage) FailureType {
	if result == nil {
		return FailureInternal
	}

	if result.Error != nil {
		return classifyErrorInfo(result.Error)
	}

	return classifyStatusCode(result.StatusCode)
}

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

func classifyStatusCode(statusCode int) FailureType {
	switch {
	case statusCode == 0:
		return FailureConnection
	case statusCode >= 200 && statusCode < 400:
		return FailureNone
	case statusCode == http.StatusForbidden:
		return FailureBlocked
	case statusCode == http.StatusTooManyRequests:
		return FailureRateLimited
	case statusCode >= http.StatusInternalServerError:
		return FailureUpstream
	default:
		return FailureNone
	}
}

// IsRetryableStatusCode returns whether an HTTP status code is retryable.
func IsRetryableStatusCode(statusCode int) bool {
	failure := classifyStatusCode(statusCode)

	return failure.ShouldRetry()
}

// IsBlockedResponse returns whether the result indicates a blocked response (e.g. captcha).
func IsBlockedResponse(result *ResultMessage) bool {
	if result == nil {
		return false
	}

	if result.StatusCode == http.StatusForbidden {
		return true
	}

	if result.Headers != nil {
		contentType := result.Headers.Get("Content-Type")
		if strings.Contains(strings.ToLower(contentType), "captcha") {
			return true
		}
	}

	return false
}
