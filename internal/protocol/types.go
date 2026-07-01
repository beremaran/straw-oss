// Package protocol defines the request/response types, serialization, and
// signature verification used between control and egress nodes.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Request represents an outbound HTTP request to be proxied by an egress.
type Request struct {
	ID              string        `json:"id"`
	Method          string        `json:"method"`
	URL             string        `json:"url"`
	Headers         HeaderMap     `json:"headers"`
	Body            []byte        `json:"body,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
	ReplyTo         string        `json:"reply_to,omitempty"`
	MaxResponseSize int64         `json:"max_response_size,omitempty"`
}

// Response represents the result of a proxied HTTP request.
type Response struct {
	RequestID  string      `json:"request_id"`
	StatusCode int         `json:"status_code"`
	Headers    HeaderMap   `json:"headers"`
	Body       []byte      `json:"body,omitempty"`
	Error      *ErrorInfo  `json:"error,omitempty"`
	Timing     *TimingInfo `json:"timing,omitempty"`
	EgressID   string      `json:"egress_id,omitempty"`
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

// SignedTask is a compressed, signed request payload sent to an egress.
type SignedTask struct {
	Payload   []byte `json:"payload"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"ts"`
}

// ErrorInfo describes an error returned by an egress.
type ErrorInfo struct {
	Code       string        `json:"code"`
	Message    string        `json:"message"`
	Retryable  bool          `json:"retryable"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

// Error codes returned in ErrorInfo.
const (
	ErrCodeEgressTimeout    = "EGRESS_TIMEOUT"
	ErrCodeUpstreamError    = "UPSTREAM_ERROR"
	ErrCodeInternalError    = "INTERNAL_ERROR"
	ErrCodeSignatureInvalid = "SIGNATURE_INVALID"
	ErrCodeReplayAttack     = "REPLAY_ATTACK"
)

// TimingInfo captures per-stage latency measurements for a proxied request.
type TimingInfo struct {
	DNSLookup    time.Duration `json:"dns_lookup,omitempty"`
	TCPConnect   time.Duration `json:"tcp_connect,omitempty"`
	TLSHandshake time.Duration `json:"tls_handshake,omitempty"`
	FirstByte    time.Duration `json:"first_byte,omitempty"`
	Total        time.Duration `json:"total"`
}

// Test constants shared across protocol test files.
const (
	testReqID           = "req-123"
	testReqIDLong       = "req-12345"
	testMethodGet       = "GET"
	testMethodPost      = "POST"
	testURL             = "https://example.com"
	testContentType     = "Content-Type"
	testUserAgent       = "User-Agent"
	testAccept          = "Accept"
	testCustomValue     = "custom-value"
	testJSONContentType = "application/json"
	testK6UserAgent     = "k6-load-test"
)
