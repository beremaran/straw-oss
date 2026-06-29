package orchestrator

import (
	"fmt"
	"testing"

	"github.com/beremaran/straw/pkg/protocol"
)

func TestFailureType_String(t *testing.T) {
	tests := []struct {
		failure  FailureType
		expected string
	}{
		{FailureNone, "none"},
		{FailureTimeout, testFailureTimeoutStr},
		{FailureConnection, testFailureConnectionStr},
		{FailureRateLimited, "rate_limited"},
		{FailureBlocked, "blocked"},
		{FailureUpstream, "upstream"},
		{FailureInternal, "internal"},
		{FailureType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.failure.String(); got != tt.expected {
				t.Errorf("FailureType.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFailureType_ShouldRetry(t *testing.T) {
	tests := []struct {
		failure  FailureType
		expected bool
	}{
		{FailureNone, false},
		{FailureTimeout, true},
		{FailureConnection, true},
		{FailureRateLimited, true},
		{FailureBlocked, false},
		{FailureUpstream, true},
		{FailureInternal, true},
	}

	for _, tt := range tests {
		t.Run(tt.failure.String(), func(t *testing.T) {
			if got := tt.failure.ShouldRetry(); got != tt.expected {
				t.Errorf("FailureType.ShouldRetry() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFailureType_ShouldEscalate(t *testing.T) {
	tests := []struct {
		failure  FailureType
		expected bool
	}{
		{FailureNone, false},
		{FailureTimeout, false},
		{FailureConnection, false},
		{FailureRateLimited, false},
		{FailureBlocked, true},
		{FailureUpstream, false},
		{FailureInternal, false},
	}

	for _, tt := range tests {
		t.Run(tt.failure.String(), func(t *testing.T) {
			if got := tt.failure.ShouldEscalate(); got != tt.expected {
				t.Errorf("FailureType.ShouldEscalate() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFailureType_RequiresBackoff(t *testing.T) {
	tests := []struct {
		failure  FailureType
		expected bool
	}{
		{FailureNone, false},
		{FailureTimeout, false},
		{FailureConnection, false},
		{FailureRateLimited, true},
		{FailureBlocked, false},
		{FailureUpstream, false},
		{FailureInternal, false},
	}

	for _, tt := range tests {
		t.Run(tt.failure.String(), func(t *testing.T) {
			if got := tt.failure.RequiresBackoff(); got != tt.expected {
				t.Errorf("FailureType.RequiresBackoff() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifyFailure(t *testing.T) {
	tests := []struct {
		name     string
		result   *ResultMessage
		expected FailureType
	}{
		{
			name:     "nil result",
			result:   nil,
			expected: FailureInternal,
		},
		{
			name: "success 200",
			result: &ResultMessage{
				StatusCode: 200,
			},
			expected: FailureNone,
		},
		{
			name: "success 302 redirect",
			result: &ResultMessage{
				StatusCode: 302,
			},
			expected: FailureNone,
		},
		{
			name: "forbidden 403",
			result: &ResultMessage{
				StatusCode: 403,
			},
			expected: FailureBlocked,
		},
		{
			name: "rate limited 429",
			result: &ResultMessage{
				StatusCode: 429,
			},
			expected: FailureRateLimited,
		},
		{
			name: "server error 500",
			result: &ResultMessage{
				StatusCode: 500,
			},
			expected: FailureUpstream,
		},
		{
			name: "server error 503",
			result: &ResultMessage{
				StatusCode: 503,
			},
			expected: FailureUpstream,
		},
		{
			name: "client error 400",
			result: &ResultMessage{
				StatusCode: 400,
			},
			expected: FailureNone,
		},
		{
			name: "no status code",
			result: &ResultMessage{
				StatusCode: 0,
			},
			expected: FailureConnection,
		},
		{
			name: "error info with timeout",
			result: &ResultMessage{
				Error: &protocol.ErrorInfo{
					Code:    protocol.ErrCodeEndpointTimeout,
					Message: testRequestTimedOut,
				},
			},
			expected: FailureTimeout,
		},
		{
			name: "error info with internal error",
			result: &ResultMessage{
				Error: &protocol.ErrorInfo{
					Code:    protocol.ErrCodeInternalError,
					Message: "internal server error",
				},
			},
			expected: FailureInternal,
		},
		{
			name: "upstream error with timeout message",
			result: &ResultMessage{
				Error: &protocol.ErrorInfo{
					Code:    protocol.ErrCodeUpstreamError,
					Message: "context deadline exceeded",
				},
			},
			expected: FailureTimeout,
		},
		{
			name: "upstream error with connection refused",
			result: &ResultMessage{
				Error: &protocol.ErrorInfo{
					Code:    protocol.ErrCodeUpstreamError,
					Message: "connection refused by target",
				},
			},
			expected: FailureConnection,
		},
		{
			name: "upstream error generic",
			result: &ResultMessage{
				Error: &protocol.ErrorInfo{
					Code:    protocol.ErrCodeUpstreamError,
					Message: "HTTP 500 from target",
				},
			},
			expected: FailureUpstream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyFailure(tt.result); got != tt.expected {
				t.Errorf("ClassifyFailure() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsBlockedResponse(t *testing.T) {
	tests := []struct {
		name     string
		result   *ResultMessage
		expected bool
	}{
		{
			name:     "nil result",
			result:   nil,
			expected: false,
		},
		{
			name: "403 status",
			result: &ResultMessage{
				StatusCode: 403,
			},
			expected: true,
		},
		{
			name: "200 status",
			result: &ResultMessage{
				StatusCode: 200,
			},
			expected: false,
		},
		{
			name: "captcha content type",
			result: &ResultMessage{
				StatusCode: 200,
				Headers: protocol.HeaderMap{
					{Key: contentTypeValue, Value: "text/html; captcha=true"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBlockedResponse(tt.result); got != tt.expected {
				t.Errorf("IsBlockedResponse() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, false},
		{301, false},
		{400, false},
		{403, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status-%d", tt.statusCode), func(t *testing.T) {
			if got := IsRetryableStatusCode(tt.statusCode); got != tt.expected {
				t.Errorf("IsRetryableStatusCode(%d) = %v, want %v", tt.statusCode, got, tt.expected)
			}
		})
	}
}

func TestClassifyErrorInfo_EdgeCases(t *testing.T) {
	t.Run("nil error info", func(t *testing.T) {
		failure := classifyErrorInfo(nil)
		if failure != FailureNone {
			t.Errorf("expected FailureNone for nil error info, got %v", failure)
		}
	})

	t.Run("unknown error code", func(t *testing.T) {
		errInfo := &protocol.ErrorInfo{
			Code:    "UNKNOWN_ERROR",
			Message: "unknown error",
		}
		failure := classifyErrorInfo(errInfo)
		if failure != FailureInternal {
			t.Errorf("expected FailureInternal for unknown error code, got %v", failure)
		}
	})

	t.Run("upstream error with deadline exceeded", func(t *testing.T) {
		errInfo := &protocol.ErrorInfo{
			Code:    protocol.ErrCodeUpstreamError,
			Message: "context deadline exceeded",
		}
		failure := classifyErrorInfo(errInfo)
		if failure != FailureTimeout {
			t.Errorf("expected FailureTimeout for deadline exceeded, got %v", failure)
		}
	})

	t.Run("upstream error with connection reset", func(t *testing.T) {
		errInfo := &protocol.ErrorInfo{
			Code:    protocol.ErrCodeUpstreamError,
			Message: "connection reset by peer",
		}
		failure := classifyErrorInfo(errInfo)
		if failure != FailureConnection {
			t.Errorf("expected FailureConnection for connection reset, got %v", failure)
		}
	})

	t.Run("upstream error with connection refused", func(t *testing.T) {
		errInfo := &protocol.ErrorInfo{
			Code:    protocol.ErrCodeUpstreamError,
			Message: errConnectionRefused,
		}
		failure := classifyErrorInfo(errInfo)
		if failure != FailureConnection {
			t.Errorf("expected FailureConnection for connection refused, got %v", failure)
		}
	})
}

func TestShouldRetry_EdgeCases(t *testing.T) {
	t.Run("unknown failure type", func(t *testing.T) {
		failure := FailureType(99)
		result := failure.ShouldRetry()
		if result {
			t.Error("expected false for unknown failure type")
		}
	})
}
