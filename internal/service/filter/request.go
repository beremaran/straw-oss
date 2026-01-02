package filter

// FilterRequest represents the request data needed for filtering decisions.
type FilterRequest struct {
	// URL is the full request URL.
	URL string

	// Host is the target hostname.
	Host string

	// ContentType is the expected response content-type (from Accept header).
	ContentType string

	// Method is the HTTP method (GET, POST, etc.).
	Method string
}

// NewFilterRequest creates a new FilterRequest.
func NewFilterRequest(url, host, contentType, method string) *FilterRequest {
	return &FilterRequest{
		URL:         url,
		Host:        host,
		ContentType: contentType,
		Method:      method,
	}
}
