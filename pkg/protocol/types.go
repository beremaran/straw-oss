// Package protocol defines the request/response types, serialization, and
// signature verification used between relay and endpoint nodes.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	// wireSizeRequestOverhead is the approximate JSON overhead for a Request
	// body (field name characters, braces, and commas for the fixed fields).
	wireSizeRequestOverhead = 12

	// wireSizeResponseOverhead is the approximate JSON overhead for a Response
	// body (field name characters, braces, and commas for the fixed fields).
	wireSizeResponseOverhead = 15

	// wireSizeHeaderOverhead is the approximate per-header JSON overhead
	// (colon, comma, and quoting characters).
	wireSizeHeaderOverhead = 4
)

// Request represents an outbound HTTP request to be proxied by an endpoint.
type Request struct {
	ID              string        `json:"id"`
	Method          string        `json:"method"`
	URL             string        `json:"url"`
	Headers         HeaderMap     `json:"headers"`
	Body            []byte        `json:"body,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
	Fingerprint     string        `json:"fingerprint,omitempty"`
	SessionID       string        `json:"session_id,omitempty"`
	TraceID         string        `json:"trace_id,omitempty"`
	ReplyTo         string        `json:"reply_to,omitempty"`
	StreamResponse  bool          `json:"stream_response,omitempty"`
	MaxResponseSize int64         `json:"max_response_size,omitempty"`
}

// EstimateWireSize returns an approximate byte count for the JSON wire
// representation of the request. It is an estimate, not an exact size.
func (r *Request) EstimateWireSize() uint64 {
	size := uint64(len(r.Method) + len(r.URL) + wireSizeRequestOverhead)

	for _, h := range r.Headers {
		size += uint64(len(h.Key) + len(h.Value) + wireSizeHeaderOverhead)
	}

	size += 2

	size += uint64(len(r.Body))

	return size
}

// Response represents the result of a proxied HTTP request.
type Response struct {
	RequestID   string      `json:"request_id"`
	StatusCode  int         `json:"status_code"`
	Headers     HeaderMap   `json:"headers"`
	Body        []byte      `json:"body,omitempty"`
	Error       *ErrorInfo  `json:"error,omitempty"`
	Timing      *TimingInfo `json:"timing,omitempty"`
	EndpointID  string      `json:"endpoint_id,omitempty"`
	SessionID   string      `json:"session_id,omitempty"`
	IsStreaming bool        `json:"is_streaming,omitempty"`
}

// EstimateWireSize returns an approximate byte count for the JSON wire
// representation of the response. It is an estimate, not an exact size.
func (r *Response) EstimateWireSize() uint64 {
	size := uint64(wireSizeResponseOverhead)

	for _, h := range r.Headers {
		size += uint64(len(h.Key) + len(h.Value) + wireSizeHeaderOverhead)
	}

	size += 2

	size += uint64(len(r.Body))

	return size
}

// HeaderMap is an ordered list of HTTP headers that preserves insertion order
// when serialized to JSON.
type HeaderMap []Header

// Header represents a single HTTP header key-value pair.
type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Get returns the value of the first header matching the given key, using
// case-insensitive comparison. It returns an empty string if no match exists.
func (h HeaderMap) Get(key string) string {
	for _, header := range h {
		if equalFold(header.Key, key) {
			return header.Value
		}
	}

	return ""
}

// Set updates the value of the first header matching the given key, or appends
// a new header if no match exists.
func (h *HeaderMap) Set(key, value string) {
	for i, header := range *h {
		if equalFold(header.Key, key) {
			(*h)[i].Value = value

			return
		}
	}

	*h = append(*h, Header{Key: key, Value: value})
}

// Del removes the first header matching the given key, using case-insensitive
// comparison.
func (h *HeaderMap) Del(key string) {
	result := (*h)[:0]
	for _, header := range *h {
		if !equalFold(header.Key, key) {
			result = append(result, header)
		}
	}

	*h = result
}

// Clone returns a shallow copy of the header map. If the receiver is nil, it
// returns nil.
func (h HeaderMap) Clone() HeaderMap {
	if h == nil {
		return nil
	}

	clone := make(HeaderMap, len(h))
	copy(clone, h)

	return clone
}

// UnmarshalJSON deserializes headers from either an array of objects or a JSON
// object mapping header names to string or string-array values.
func (h *HeaderMap) UnmarshalJSON(data []byte) error {
	var arrayFormat []Header

	err := json.Unmarshal(data, &arrayFormat)
	if err == nil {
		*h = HeaderMap(arrayFormat)

		return nil
	}

	return h.unmarshalJSONObject(data)
}

func (h *HeaderMap) unmarshalJSONObject(data []byte) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))

	var t json.Token

	t, err := dec.Token()
	if err != nil {
		return fmt.Errorf("decoding object start: %w", err)
	} else if t != json.Delim('{') {
		return &json.UnmarshalTypeError{Value: "object", Offset: 0}
	}

	var result HeaderMap

	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return fmt.Errorf("decoding header key: %w", err)
		}

		key, ok := keyToken.(string)
		if !ok {
			return &json.UnmarshalTypeError{Value: "string", Offset: dec.InputOffset()}
		}

		var rawValue json.RawMessage

		err = dec.Decode(&rawValue)
		if err != nil {
			return fmt.Errorf("decoding header value: %w", err)
		}

		value, err := decodeHeaderJSONValue(rawValue)
		if err != nil {
			return err
		}

		result = append(result, Header{Key: key, Value: value})
	}

	_, err = dec.Token()
	if err != nil {
		return fmt.Errorf("decoding object end: %w", err)
	}

	*h = result

	return nil
}

func decodeHeaderJSONValue(rawValue json.RawMessage) (string, error) {
	if len(rawValue) > 0 && rawValue[0] == '[' {
		var values []string

		err := json.Unmarshal(rawValue, &values)
		if err != nil {
			return "", fmt.Errorf("decoding header array value: %w", err)
		}

		return strings.Join(values, ", "), nil
	}

	var value string

	err := json.Unmarshal(rawValue, &value)
	if err != nil {
		return "", fmt.Errorf("decoding header string value: %w", err)
	}

	return value, nil
}

func equalFold(a, b string) bool {
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

// SignedTask is a compressed, signed request payload sent to an endpoint.
type SignedTask struct {
	Payload   []byte `json:"payload"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"ts"`
}

// ErrorInfo describes an error returned by an endpoint.
type ErrorInfo struct {
	Code       string        `json:"code"`
	Message    string        `json:"message"`
	Retryable  bool          `json:"retryable"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

// Error codes returned in ErrorInfo.
const (
	ErrCodeAuthInvalid          = "AUTH_INVALID"
	ErrCodeAuthForbidden        = "AUTH_FORBIDDEN"
	ErrCodeRateLimitExceeded    = "RATE_LIMIT_EXCEEDED"
	ErrCodeNoEndpointsAvailable = "NO_ENDPOINTS_AVAILABLE"
	ErrCodeEndpointTimeout      = "ENDPOINT_TIMEOUT"
	ErrCodeUpstreamError        = "UPSTREAM_ERROR"
	ErrCodeSessionExpired       = "SESSION_EXPIRED"
	ErrCodeInternalError        = "INTERNAL_ERROR"
	ErrCodeSignatureInvalid     = "SIGNATURE_INVALID"
	ErrCodeReplayAttack         = "REPLAY_ATTACK"
)

// TimingInfo captures per-stage latency measurements for a proxied request.
type TimingInfo struct {
	DNSLookup    time.Duration `json:"dns_lookup,omitempty"`
	TCPConnect   time.Duration `json:"tcp_connect,omitempty"`
	TLSHandshake time.Duration `json:"tls_handshake,omitempty"`
	FirstByte    time.Duration `json:"first_byte,omitempty"`
	Total        time.Duration `json:"total"`
}

// ControlCommand is sent from relay to an endpoint to issue administrative
// commands.
type ControlCommand struct {
	CommandID  string         `json:"command_id"`
	EndpointID string         `json:"endpoint_id"`
	Command    string         `json:"command"`
	IssuedAt   time.Time      `json:"issued_at"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// CommandAck acknowledges receipt of a ControlCommand.
type CommandAck struct {
	CommandID  string    `json:"command_id"`
	EndpointID string    `json:"endpoint_id"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	Timestamp  time.Time `json:"ts"`
}

// LogEntry records a structured log emitted by an endpoint.
type LogEntry struct {
	EndpointID string         `json:"endpoint_id"`
	ObservedAt time.Time      `json:"observed_at"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Attrs      map[string]any `json:"attrs,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
}

// Test constants shared across protocol test files.
const (
	testReqID           = "req-123"
	testReqIDLong       = "req-12345"
	testMethodGet       = "GET"
	testMethodPost      = "POST"
	testURL             = "https://example.com"
	testFingerprint     = "chrome-130"
	testContentType     = "Content-Type"
	testUserAgent       = "User-Agent"
	testAccept          = "Accept"
	testCustomValue     = "custom-value"
	testJSONContentType = "application/json"
	testK6UserAgent     = "k6-load-test"
)
