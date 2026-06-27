package orchestrator

import (
	"net/http"
	"strings"

	"github.com/beremaran/straw/pkg/protocol"
)

type FailureType int

const (
	FailureNone FailureType = iota

	FailureTimeout

	FailureConnection

	FailureRateLimited

	FailureBlocked

	FailureUpstream

	FailureInternal
)

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

func (f FailureType) ShouldEscalate() bool {
	return f == FailureBlocked
}

func (f FailureType) RequiresBackoff() bool {
	return f == FailureRateLimited
}

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
	case statusCode >= 500:

		return FailureUpstream
	default:

		return FailureNone
	}
}

func IsRetryableStatusCode(statusCode int) bool {
	failure := classifyStatusCode(statusCode)
	return failure.ShouldRetry()
}

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
