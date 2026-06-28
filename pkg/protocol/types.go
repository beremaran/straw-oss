package protocol

import (
	"encoding/json"
	"strings"
	"time"
)

type Request struct {
	ID string `json:"id"`

	Method string `json:"method"`

	URL string `json:"url"`

	Headers HeaderMap `json:"headers"`

	Body []byte `json:"body,omitempty"`

	Timeout time.Duration `json:"timeout,omitempty"`

	Fingerprint string `json:"fingerprint,omitempty"`

	SessionID string `json:"session_id,omitempty"`

	TraceID string `json:"trace_id,omitempty"`

	ReplyTo string `json:"reply_to,omitempty"`

	StreamResponse bool `json:"stream_response,omitempty"`

	MaxResponseSize int64 `json:"max_response_size,omitempty"`
}

func (r *Request) EstimateWireSize() uint64 {

	size := uint64(len(r.Method) + len(r.URL) + 12)

	for _, h := range r.Headers {

		size += uint64(len(h.Key) + len(h.Value) + 4)
	}

	size += 2

	size += uint64(len(r.Body))

	return size
}

type Response struct {
	RequestID string `json:"request_id"`

	StatusCode int `json:"status_code"`

	Headers HeaderMap `json:"headers"`

	Body []byte `json:"body,omitempty"`

	Error *ErrorInfo `json:"error,omitempty"`

	Timing *TimingInfo `json:"timing,omitempty"`

	EndpointID string `json:"endpoint_id,omitempty"`

	SessionID string `json:"session_id,omitempty"`

	IsStreaming bool `json:"is_streaming,omitempty"`
}

func (r *Response) EstimateWireSize() uint64 {

	size := uint64(15)

	for _, h := range r.Headers {

		size += uint64(len(h.Key) + len(h.Value) + 4)
	}

	size += 2

	size += uint64(len(r.Body))

	return size
}

type HeaderMap []Header

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (h HeaderMap) Get(key string) string {
	for _, header := range h {
		if equalFold(header.Key, key) {
			return header.Value
		}
	}
	return ""
}

func (h *HeaderMap) Set(key, value string) {
	for i, header := range *h {
		if equalFold(header.Key, key) {
			(*h)[i].Value = value
			return
		}
	}
	*h = append(*h, Header{Key: key, Value: value})
}

func (h *HeaderMap) Del(key string) {
	result := (*h)[:0]
	for _, header := range *h {
		if !equalFold(header.Key, key) {
			result = append(result, header)
		}
	}
	*h = result
}

func (h HeaderMap) Clone() HeaderMap {
	if h == nil {
		return nil
	}
	clone := make(HeaderMap, len(h))
	copy(clone, h)
	return clone
}

func (h *HeaderMap) UnmarshalJSON(data []byte) error {

	var arrayFormat []Header
	if err := json.Unmarshal(data, &arrayFormat); err == nil {
		*h = HeaderMap(arrayFormat)
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(string(data)))
	if t, err := dec.Token(); err != nil {
		return err
	} else if t != json.Delim('{') {
		return &json.UnmarshalTypeError{Value: "object", Offset: 0}
	}

	var result HeaderMap
	for dec.More() {

		keyToken, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return &json.UnmarshalTypeError{Value: "string", Offset: dec.InputOffset()}
		}

		var value string
		var rawValue json.RawMessage
		if err := dec.Decode(&rawValue); err != nil {
			return err
		}

		if len(rawValue) > 0 && rawValue[0] == '[' {
			var values []string
			if err := json.Unmarshal(rawValue, &values); err != nil {
				return err
			}

			value = strings.Join(values, ", ")
		} else {

			if err := json.Unmarshal(rawValue, &value); err != nil {
				return err
			}
		}

		result = append(result, Header{Key: key, Value: value})
	}

	if _, err := dec.Token(); err != nil {
		return err
	}

	*h = result
	return nil
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

type SignedTask struct {
	Payload []byte `json:"payload"`

	Signature string `json:"signature"`

	Timestamp int64 `json:"ts"`
}

type ErrorInfo struct {
	Code string `json:"code"`

	Message string `json:"message"`

	Retryable bool `json:"retryable"`

	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

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

type TimingInfo struct {
	DNSLookup time.Duration `json:"dns_lookup,omitempty"`

	TCPConnect time.Duration `json:"tcp_connect,omitempty"`

	TLSHandshake time.Duration `json:"tls_handshake,omitempty"`

	FirstByte time.Duration `json:"first_byte,omitempty"`

	Total time.Duration `json:"total"`
}
