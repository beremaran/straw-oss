package filter

type Request struct {
	URL string

	Host string

	ContentType string

	Method string
}

func NewFilterRequest(url, host, contentType, method string) *Request {
	return &Request{
		URL:         url,
		Host:        host,
		ContentType: contentType,
		Method:      method,
	}
}
