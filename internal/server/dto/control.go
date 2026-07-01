package dto

// ControlRequest is the request body for proxying a request through the control.
type ControlRequest struct {
	ID              string            `json:"id,omitempty"`
	Method          string            `json:"method,omitempty"`
	URL             string            `json:"url"                         validate:"required,url"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            []byte            `json:"body,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	MaxResponseSize int64             `json:"max_response_size,omitempty"`
}

// ControlResponse is the response body from controlling a request.
type ControlResponse struct {
	RequestID  string            `json:"request_id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
	Timing     *TimingDTO        `json:"timing,omitempty"`
	Meta       *ControlMetaDTO   `json:"meta,omitempty"`
}

// TimingDTO contains timing information for a controlled request.
type TimingDTO struct {
	DNSLookup    string `json:"dns_lookup,omitempty"`
	TCPConnect   string `json:"tcp_connect,omitempty"`
	TLSHandshake string `json:"tls_handshake,omitempty"`
	FirstByte    string `json:"first_byte,omitempty"`
	Total        string `json:"total"`
}

// ControlMetaDTO contains metadata about a controlled request.
type ControlMetaDTO struct {
	Retries  int      `json:"retries,omitempty"`
	Pool     string   `json:"pool,omitempty"`
	EgressID string   `json:"egress_id,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}
