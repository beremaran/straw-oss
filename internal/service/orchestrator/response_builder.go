package orchestrator

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
	"github.com/labstack/echo/v4"
)

// ResponseBuilder builds HTTP responses from result messages.
type ResponseBuilder struct {
	// FilterHeaders specifies headers to exclude from upstream response.
	FilterHeaders []string
}

// NewResponseBuilder creates a new ResponseBuilder with default settings.
func NewResponseBuilder() *ResponseBuilder {
	return &ResponseBuilder{
		FilterHeaders: defaultFilteredHeaders,
	}
}

// defaultFilteredHeaders are headers that should not be copied from upstream.
// These are hop-by-hop headers or headers that should be set by the proxy.
var defaultFilteredHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
	"Content-Length",   // Will be set by Echo based on actual body
	"Content-Encoding", // Response is already decompressed
}

// RelayMetadata contains metadata about the relay operation.
type RelayMetadata struct {
	Retries       int
	Pool          string
	Timing        *protocol.TimingInfo
	EndpointID    string
	SessionID     string
	Migrated      bool
	MigrateCount  int
	AttemptErrors []AttemptError // Failed attempts for debugging
}

// WriteResponse writes a ResultMessage to an Echo context as an HTTP response.
func (b *ResponseBuilder) WriteResponse(c echo.Context, result *ResultMessage, meta *RelayMetadata) error {
	// Set status code
	statusCode := result.StatusCode
	if statusCode == 0 {
		// Default to 502 if no status code and there's an error
		if result.Error != nil {
			statusCode = http.StatusBadGateway
		} else {
			statusCode = http.StatusOK
		}
	}

	// Copy headers from upstream (filtered)
	b.copyHeaders(c.Response(), result.Headers)

	// Add relay headers
	b.addRelayHeaders(c.Response(), meta)

	// Handle error response
	if result.Error != nil {
		return b.writeErrorResponse(c, statusCode, result.Error)
	}

	// Write body
	return c.Blob(statusCode, result.Headers.Get("Content-Type"), result.CompressedBody)
}

// copyHeaders copies headers from the upstream response to the client response,
// filtering out hop-by-hop headers and other headers that shouldn't be copied.
func (b *ResponseBuilder) copyHeaders(resp *echo.Response, headers protocol.HeaderMap) {
	for _, h := range headers {
		if b.isFiltered(h.Key) {
			continue
		}
		resp.Header().Add(h.Key, h.Value)
	}
}

// isFiltered checks if a header should be filtered out.
func (b *ResponseBuilder) isFiltered(key string) bool {
	for _, filtered := range b.FilterHeaders {
		if equalFoldASCII(key, filtered) {
			return true
		}
	}
	return false
}

// addRelayHeaders adds relay-specific headers to the response.
func (b *ResponseBuilder) addRelayHeaders(resp *echo.Response, meta *RelayMetadata) {
	if meta == nil {
		return
	}

	// Add retry count if any retries occurred
	if meta.Retries > 0 {
		resp.Header().Set("X-Relay-Retries", strconv.Itoa(meta.Retries))
	}

	// Add pool information
	if meta.Pool != "" {
		resp.Header().Set("X-Relay-Pool", meta.Pool)
	}

	// Add timing information
	if meta.Timing != nil {
		resp.Header().Set("X-Relay-Timing", formatTiming(meta.Timing))
	}

	// Add endpoint ID for debugging
	if meta.EndpointID != "" {
		resp.Header().Set("X-Relay-Endpoint", meta.EndpointID)
	}

	// Add session headers
	if meta.SessionID != "" {
		resp.Header().Set("X-Session-ID", meta.SessionID)
	}

	if meta.Migrated {
		resp.Header().Set("X-Session-Migrated", "true")
		resp.Header().Set("X-Session-Migration-Count", strconv.Itoa(meta.MigrateCount))
	}

	// Add attempt errors for debugging (only include if there were failures)
	if len(meta.AttemptErrors) > 0 {
		if errorsJSON, err := json.Marshal(formatAttemptErrors(meta.AttemptErrors)); err == nil {
			resp.Header().Set("X-Relay-Attempt-Errors", string(errorsJSON))
		}
	}
}

// writeErrorResponse writes an error response in the standard format.
func (b *ResponseBuilder) writeErrorResponse(c echo.Context, statusCode int, errInfo *protocol.ErrorInfo) error {
	response := map[string]interface{}{
		"error": map[string]interface{}{
			"code":       errInfo.Code,
			"message":    errInfo.Message,
			"retryable":  errInfo.Retryable,
			"request_id": c.Response().Header().Get(echo.HeaderXRequestID),
		},
	}

	if errInfo.RetryAfter > 0 {
		response["error"].(map[string]interface{})["retry_after_seconds"] = int(errInfo.RetryAfter.Seconds())
		c.Response().Header().Set("Retry-After", strconv.Itoa(int(errInfo.RetryAfter.Seconds())))
	}

	return c.JSON(statusCode, response)
}

// formatTiming formats timing info for the X-Relay-Timing header.
func formatTiming(t *protocol.TimingInfo) string {
	if t == nil {
		return ""
	}
	return t.Total.Round(time.Millisecond).String()
}

// equalFoldASCII is a simple case-insensitive comparison for ASCII strings.
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// WriteTimeoutResponse writes a 504 Gateway Timeout response.
func WriteTimeoutResponse(c echo.Context, requestID string) error {
	c.Response().Header().Set("X-Request-ID", requestID)

	return c.JSON(http.StatusGatewayTimeout, map[string]interface{}{
		"error": map[string]interface{}{
			"code":       protocol.ErrCodeEndpointTimeout,
			"message":    "Endpoint did not respond in time",
			"retryable":  true,
			"request_id": requestID,
		},
	})
}

// attemptErrorSummary is a compact representation of an AttemptError for headers.
type attemptErrorSummary struct {
	Pool     int    `json:"p"`
	Attempt  int    `json:"a"`
	Endpoint string `json:"e,omitempty"`
	Failure  string `json:"f"`
	Message  string `json:"m,omitempty"`
}

// formatAttemptErrors converts AttemptErrors to a compact format for the header.
func formatAttemptErrors(errors []AttemptError) []attemptErrorSummary {
	summaries := make([]attemptErrorSummary, 0, len(errors))
	for _, e := range errors {
		summaries = append(summaries, attemptErrorSummary{
			Pool:     e.Pool,
			Attempt:  e.Attempt,
			Endpoint: e.EndpointID,
			Failure:  e.FailureString,
			Message:  truncateMessage(e.Message, 50),
		})
	}
	return summaries
}

// truncateMessage truncates a message to maxLen characters.
func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}
