package dto

// RelayRequest is the request body for proxying a request through the relay.
type RelayRequest struct {
	ID              string            `json:"id,omitempty"`
	Method          string            `json:"method,omitempty"`
	URL             string            `json:"url"                         validate:"required,url"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            []byte            `json:"body,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	MaxResponseSize int64             `json:"max_response_size,omitempty"`
}

// RelayResponse is the response body from relaying a request.
type RelayResponse struct {
	RequestID  string            `json:"request_id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
	Timing     *TimingDTO        `json:"timing,omitempty"`
	Meta       *RelayMetaDTO     `json:"meta,omitempty"`
}

// TimingDTO contains timing information for a relayed request.
type TimingDTO struct {
	DNSLookup    string `json:"dns_lookup,omitempty"`
	TCPConnect   string `json:"tcp_connect,omitempty"`
	TLSHandshake string `json:"tls_handshake,omitempty"`
	FirstByte    string `json:"first_byte,omitempty"`
	Total        string `json:"total"`
}

// RelayMetaDTO contains metadata about a relayed request.
type RelayMetaDTO struct {
	Retries    int      `json:"retries,omitempty"`
	Pool       string   `json:"pool,omitempty"`
	EndpointID string   `json:"endpoint_id,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}
