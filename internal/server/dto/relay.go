package dto

type RelayRequest struct {
	ID              string            `json:"id,omitempty"`
	Method          string            `json:"method,omitempty"`
	URL             string            `json:"url"                         validate:"required,url"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            []byte            `json:"body,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	SessionID       string            `json:"session_id,omitempty"`
	TraceID         string            `json:"trace_id,omitempty"`
	StreamResponse  bool              `json:"stream_response,omitempty"`
	MaxResponseSize int64             `json:"max_response_size,omitempty"`
}

type RelayResponse struct {
	RequestID   string            `json:"request_id"`
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        []byte            `json:"body,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Timing      *TimingDTO        `json:"timing,omitempty"`
	Meta        *RelayMetaDTO     `json:"meta,omitempty"`
	IsStreaming bool              `json:"is_streaming,omitempty"`
}

type TimingDTO struct {
	DNSLookup    string `json:"dns_lookup,omitempty"`
	TCPConnect   string `json:"tcp_connect,omitempty"`
	TLSHandshake string `json:"tls_handshake,omitempty"`
	FirstByte    string `json:"first_byte,omitempty"`
	Total        string `json:"total"`
}

type RelayMetaDTO struct {
	Retries    int      `json:"retries,omitempty"`
	Pool       string   `json:"pool,omitempty"`
	EndpointID string   `json:"endpoint_id,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}
