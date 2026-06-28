package orchestrator

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/beremaran/straw/pkg/protocol"
)

type ResponseBuilder struct {
	FilterHeaders []string
}

func NewResponseBuilder() *ResponseBuilder {
	return &ResponseBuilder{
		FilterHeaders: defaultFilteredHeaders,
	}
}

var defaultFilteredHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
	"Content-Length",
	"Content-Encoding",
}

type RelayMetadata struct {
	Retries       int
	Pool          string
	Timing        *protocol.TimingInfo
	EndpointID    string
	SessionID     string
	Migrated      bool
	MigrateCount  int
	AttemptErrors []AttemptError
}

func (b *ResponseBuilder) WriteResponse(w http.ResponseWriter, result *ResultMessage, meta *RelayMetadata) error {

	statusCode := result.StatusCode
	if statusCode == 0 {

		if result.Error != nil {
			statusCode = http.StatusBadGateway
		} else {
			statusCode = http.StatusOK
		}
	}

	b.copyHeaders(w.Header(), result.Headers)

	b.addRelayHeaders(w.Header(), meta)

	if result.Error != nil {
		return b.writeErrorResponse(w, statusCode, result.Error)
	}

	w.Header().Set("Content-Type", result.Headers.Get("Content-Type"))
	w.WriteHeader(statusCode)
	_, err := w.Write(result.CompressedBody)
	return err
}

func (b *ResponseBuilder) copyHeaders(resp http.Header, headers protocol.HeaderMap) {
	for _, h := range headers {
		if b.isFiltered(h.Key) {
			continue
		}
		resp.Add(h.Key, h.Value)
	}
}

func (b *ResponseBuilder) isFiltered(key string) bool {
	for _, filtered := range b.FilterHeaders {
		if equalFoldASCII(key, filtered) {
			return true
		}
	}
	return false
}

func (b *ResponseBuilder) addRelayHeaders(resp http.Header, meta *RelayMetadata) {
	if meta == nil {
		return
	}

	if meta.Retries > 0 {
		resp.Set("X-Relay-Retries", strconv.Itoa(meta.Retries))
	}

	if meta.Pool != "" {
		resp.Set("X-Relay-Pool", meta.Pool)
	}

	if meta.Timing != nil {
		resp.Set("X-Relay-Timing", formatTiming(meta.Timing))
	}

	if meta.EndpointID != "" {
		resp.Set("X-Relay-Endpoint", meta.EndpointID)
	}

	if meta.SessionID != "" {
		resp.Set("X-Session-ID", meta.SessionID)
	}

	if meta.Migrated {
		resp.Set("X-Session-Migrated", "true")
		resp.Set("X-Session-Migration-Count", strconv.Itoa(meta.MigrateCount))
	}

	if len(meta.AttemptErrors) > 0 {
		if errorsJSON, err := json.Marshal(formatAttemptErrors(meta.AttemptErrors)); err == nil {
			resp.Set("X-Relay-Attempt-Errors", string(errorsJSON))
		}
	}
}

func (b *ResponseBuilder) writeErrorResponse(w http.ResponseWriter, statusCode int, errInfo *protocol.ErrorInfo) error {
	requestID := w.Header().Get("X-Request-ID")
	response := map[string]interface{}{
		"error": map[string]interface{}{
			"code":       errInfo.Code,
			"message":    errInfo.Message,
			"retryable":  errInfo.Retryable,
			"request_id": requestID,
		},
	}

	if errInfo.RetryAfter > 0 {
		response["error"].(map[string]interface{})["retry_after_seconds"] = int(errInfo.RetryAfter.Seconds())
		w.Header().Set("Retry-After", strconv.Itoa(int(errInfo.RetryAfter.Seconds())))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(response)
}

func formatTiming(t *protocol.TimingInfo) string {
	if t == nil {
		return ""
	}
	return t.Total.Round(time.Millisecond).String()
}

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

func WriteTimeoutResponse(w http.ResponseWriter, requestID string) error {
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGatewayTimeout)

	return json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":       protocol.ErrCodeEndpointTimeout,
			"message":    "Endpoint did not respond in time",
			"retryable":  true,
			"request_id": requestID,
		},
	})
}

type attemptErrorSummary struct {
	Pool     int    `json:"p"`
	Attempt  int    `json:"a"`
	Endpoint string `json:"e,omitempty"`
	Failure  string `json:"f"`
	Message  string `json:"m,omitempty"`
}

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

func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}
