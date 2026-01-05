// Package protocol defines shared data types and codecs used by both
// the Relay Server and Endpoint components of the Straw Proxy system.
package protocol

import (
	"encoding/json"
	"strings"
	"time"
)

// Request represents an HTTP request to be proxied by an Endpoint.
type Request struct {
	// ID is a unique identifier for this request (used for correlation).
	ID string `json:"id"`

	// Method is the HTTP method (GET, POST, etc.).
	Method string `json:"method"`

	// URL is the target URL to fetch.
	URL string `json:"url"`

	// Headers contains the HTTP headers to send.
	Headers HeaderMap `json:"headers"`

	// Body contains the request body (for POST, PUT, etc.).
	Body []byte `json:"body,omitempty"`

	// Timeout specifies how long the Endpoint should wait for a response.
	Timeout time.Duration `json:"timeout,omitempty"`

	// Fingerprint specifies which browser TLS fingerprint preset to use.
	Fingerprint string `json:"fingerprint,omitempty"`

	// SessionID links this request to a sticky session (optional).
	SessionID string `json:"session_id,omitempty"`

	// TraceID for distributed tracing (OpenTelemetry).
	TraceID string `json:"trace_id,omitempty"`

	// ReplyTo specifies the queue name to send the response to.
	// If empty, the endpoint generates a default result queue name.
	ReplyTo string `json:"reply_to,omitempty"`

	// StreamResponse indicates the caller wants the response streamed
	// rather than fully buffered. Useful for large file downloads.
	StreamResponse bool `json:"stream_response,omitempty"`

	// MaxResponseSize limits response body size in bytes (0 = use default).
	// When set, overrides the endpoint's default max body size.
	MaxResponseSize int64 `json:"max_response_size,omitempty"`
}

// EstimateWireSize returns the approximate size of the HTTP request in bytes.
// It includes the method, URL, headers, and body.
func (r *Request) EstimateWireSize() uint64 {
	// Request line: Method + " " + URL + " HTTP/1.1\r\n"
	size := uint64(len(r.Method) + len(r.URL) + 12)

	for _, h := range r.Headers {
		// Header: Key + ": " + Value + "\r\n"
		size += uint64(len(h.Key) + len(h.Value) + 4)
	}

	// End of headers: "\r\n"
	size += 2

	// Body
	size += uint64(len(r.Body))

	return size
}

// Response represents the result of a proxied request from an Endpoint.
type Response struct {
	// RequestID correlates this response to the original request.
	RequestID string `json:"request_id"`

	// StatusCode is the HTTP status code received from the target.
	StatusCode int `json:"status_code"`

	// Headers contains the HTTP response headers.
	Headers HeaderMap `json:"headers"`

	// Body contains the response body.
	Body []byte `json:"body,omitempty"`

	// Error contains error details if the request failed.
	Error *ErrorInfo `json:"error,omitempty"`

	// Timing contains request timing details for observability.
	Timing *TimingInfo `json:"timing,omitempty"`

	// EndpointID identifies which endpoint handled this request.
	EndpointID string `json:"endpoint_id,omitempty"`

	// SessionID if a session was created or used.
	SessionID string `json:"session_id,omitempty"`

	// IsStreaming indicates this is a streaming response where
	// the body will be delivered separately (not in this payload).
	IsStreaming bool `json:"is_streaming,omitempty"`
}

// EstimateWireSize returns the approximate size of the HTTP response in bytes.
// It includes the status line, headers, and body.
func (r *Response) EstimateWireSize() uint64 {
	// Status line: "HTTP/1.1 " + 3 chars status + " " + StatusText + "\r\n"
	// We estimate status line as ~15 bytes (e.g. "HTTP/1.1 200 OK\r\n")
	size := uint64(15)

	for _, h := range r.Headers {
		// Header: Key + ": " + Value + "\r\n"
		size += uint64(len(h.Key) + len(h.Value) + 4)
	}

	// End of headers: "\r\n"
	size += 2

	// Body
	size += uint64(len(r.Body))

	return size
}

// HeaderMap is an ordered list of HTTP headers.
// We use a slice instead of a map to preserve header order,
// which is important for TLS fingerprinting.
type HeaderMap []Header

// Header represents a single HTTP header.
type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Get returns the first value for the given header key (case-insensitive).
func (h HeaderMap) Get(key string) string {
	for _, header := range h {
		if equalFold(header.Key, key) {
			return header.Value
		}
	}
	return ""
}

// Set adds or replaces a header value.
func (h *HeaderMap) Set(key, value string) {
	for i, header := range *h {
		if equalFold(header.Key, key) {
			(*h)[i].Value = value
			return
		}
	}
	*h = append(*h, Header{Key: key, Value: value})
}

// Del removes a header by key.
func (h *HeaderMap) Del(key string) {
	result := (*h)[:0]
	for _, header := range *h {
		if !equalFold(header.Key, key) {
			result = append(result, header)
		}
	}
	*h = result
}

// Clone returns a deep copy of the HeaderMap.
func (h HeaderMap) Clone() HeaderMap {
	if h == nil {
		return nil
	}
	clone := make(HeaderMap, len(h))
	copy(clone, h)
	return clone
}

// UnmarshalJSON implements custom JSON unmarshaling for HeaderMap.
// It supports two formats:
// 1. Array format (existing): [{"key": "name", "value": "val"}]
// 2. Object format (new): {"name": "val"} or {"name": ["val"]}
// The object format preserves header order for TLS fingerprinting.
func (h *HeaderMap) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as array of Header structs (existing format)
	var arrayFormat []Header
	if err := json.Unmarshal(data, &arrayFormat); err == nil {
		*h = HeaderMap(arrayFormat)
		return nil
	}

	// If array format fails, try object format
	// Use a decoder to iterate over object keys in order
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if t, err := dec.Token(); err != nil {
		return err
	} else if t != json.Delim('{') {
		return &json.UnmarshalTypeError{Value: "object", Offset: 0}
	}

	var result HeaderMap
	for dec.More() {
		// Read key (header name)
		keyToken, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return &json.UnmarshalTypeError{Value: "string", Offset: dec.InputOffset()}
		}

		// Read value (could be string or array of strings)
		var value string
		var rawValue json.RawMessage
		if err := dec.Decode(&rawValue); err != nil {
			return err
		}

		// Check if value is an array
		if len(rawValue) > 0 && rawValue[0] == '[' {
			var values []string
			if err := json.Unmarshal(rawValue, &values); err != nil {
				return err
			}
			// Join array values with comma (standard HTTP header behavior)
			value = strings.Join(values, ", ")
		} else {
			// Value is a string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return err
			}
		}

		result = append(result, Header{Key: key, Value: value})
	}

	// Consume closing brace
	if _, err := dec.Token(); err != nil {
		return err
	}

	*h = result
	return nil
}

// equalFold is a simple case-insensitive string comparison for ASCII.
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

// SignedTask wraps a compressed request payload with an HMAC signature
// for secure transport between Server and Endpoint via RabbitMQ.
type SignedTask struct {
	// Payload is the LZMA compressed request data.
	Payload []byte `json:"payload"`

	// Signature is the HMAC-SHA256 signature of the payload.
	Signature string `json:"signature"`

	// Timestamp is a Unix timestamp for replay protection.
	// Endpoints should reject tasks older than 60 seconds.
	Timestamp int64 `json:"ts"`
}

// ErrorInfo contains error details when a request fails.
type ErrorInfo struct {
	// Code is a machine-readable error code (e.g., "UPSTREAM_ERROR").
	Code string `json:"code"`

	// Message is a human-readable error description.
	Message string `json:"message"`

	// Retryable indicates whether the client should retry the request.
	Retryable bool `json:"retryable"`

	// RetryAfter suggests how long to wait before retrying (optional).
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

// Error codes as defined in Section 5.2 of the design document.
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

// TimingInfo contains request timing details for observability.
type TimingInfo struct {
	// DNSLookup is the time spent resolving DNS.
	DNSLookup time.Duration `json:"dns_lookup,omitempty"`

	// TCPConnect is the time spent establishing TCP connection.
	TCPConnect time.Duration `json:"tcp_connect,omitempty"`

	// TLSHandshake is the time spent on TLS negotiation.
	TLSHandshake time.Duration `json:"tls_handshake,omitempty"`

	// FirstByte is the time until the first response byte was received.
	FirstByte time.Duration `json:"first_byte,omitempty"`

	// Total is the total request duration.
	Total time.Duration `json:"total"`
}
