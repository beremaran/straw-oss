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
