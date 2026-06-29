package filter

// Request holds the attributes of an HTTP request for filtering.
type Request struct {
	URL         string
	Host        string
	ContentType string
	Method      string
}

// NewFilterRequest creates a new Request with the given attributes.
func NewFilterRequest(url, host, contentType, method string) *Request {
	return &Request{
		URL:         url,
		Host:        host,
		ContentType: contentType,
		Method:      method,
	}
}
