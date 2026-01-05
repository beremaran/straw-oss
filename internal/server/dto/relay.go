package dto

// RelayRequest is the API request for proxying HTTP requests.
//
//	@Description HTTP request to be proxied through the relay
type RelayRequest struct {
	// ID is an optional client-provided request ID for correlation
	ID string `json:"id,omitempty"`

	// Method is the HTTP method (defaults to GET)
	Method string `json:"method,omitempty"`

	// URL is the target URL to proxy (required)
	URL string `json:"url" validate:"required,url"`

	// Headers are the HTTP headers to send
	Headers map[string]string `json:"headers,omitempty"`

	// Body is the request body
	Body []byte `json:"body,omitempty"`

	// Timeout is the request timeout (e.g., "30s")
	Timeout string `json:"timeout,omitempty"`

	// SessionID for sticky session affinity
	SessionID string `json:"session_id,omitempty"`

	// TraceID for distributed tracing
	TraceID string `json:"trace_id,omitempty"`

	// StreamResponse requests streaming response instead of buffered.
	// Useful for large file downloads to avoid memory issues.
	StreamResponse bool `json:"stream_response,omitempty"`

	// MaxResponseSize limits response body size in bytes (0 = use default)
	MaxResponseSize int64 `json:"max_response_size,omitempty"`
}

// RelayResponse is the API response from a proxied request.
//
//	@Description Response from a proxied HTTP request
type RelayResponse struct {
	// RequestID correlates to the original request
	RequestID string `json:"request_id"`

	// StatusCode is the HTTP status from the target
	StatusCode int `json:"status_code"`

	// Headers from the target response
	Headers map[string]string `json:"headers,omitempty"`

	// Body is the response body
	Body []byte `json:"body,omitempty"`

	// SessionID if a session was created or used
	SessionID string `json:"session_id,omitempty"`

	// Timing contains request timing breakdown
	Timing *TimingDTO `json:"timing,omitempty"`

	// Meta contains relay metadata
	Meta *RelayMetaDTO `json:"meta,omitempty"`

	// IsStreaming indicates body will be delivered via streaming endpoint
	IsStreaming bool `json:"is_streaming,omitempty"`
}

// TimingDTO contains request timing details
type TimingDTO struct {
	DNSLookup    string `json:"dns_lookup,omitempty"`
	TCPConnect   string `json:"tcp_connect,omitempty"`
	TLSHandshake string `json:"tls_handshake,omitempty"`
	FirstByte    string `json:"first_byte,omitempty"`
	Total        string `json:"total"`
}

// RelayMetaDTO contains relay-specific metadata
type RelayMetaDTO struct {
	Retries    int      `json:"retries,omitempty"`
	Pool       string   `json:"pool,omitempty"`
	EndpointID string   `json:"endpoint_id,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}
