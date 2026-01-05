package protocol

import "sync"

var (
	requestPool = sync.Pool{
		New: func() any {
			return &Request{
				Headers: make(HeaderMap, 0, 10), // Pre-allocate some capacity
			}
		},
	}

	responsePool = sync.Pool{
		New: func() any {
			return &Response{
				Headers: make(HeaderMap, 0, 10),
			}
		},
	}
)

// AcquireRequest retrieves a Request from the pool.
func AcquireRequest() *Request {
	req := requestPool.Get().(*Request)
	return req
}

// ReleaseRequest resets and returns a Request to the pool.
func ReleaseRequest(req *Request) {
	if req == nil {
		return
	}
	req.Reset()
	requestPool.Put(req)
}

// AcquireResponse retrieves a Response from the pool.
func AcquireResponse() *Response {
	return responsePool.Get().(*Response)
}

// ReleaseResponse resets and returns a Response to the pool.
func ReleaseResponse(resp *Response) {
	if resp == nil {
		return
	}
	resp.Reset()
	responsePool.Put(resp)
}

// Reset clears the Request for reuse.
func (r *Request) Reset() {
	r.ID = ""
	r.Method = ""
	r.URL = ""
	// Clear headers but keep capacity if possible
	r.Headers = r.Headers[:0]
	r.Body = nil // Allow GC of large bodies
	r.Timeout = 0
	r.Fingerprint = ""
	r.SessionID = ""
	r.TraceID = ""
	r.ReplyTo = ""
	r.StreamResponse = false
	r.MaxResponseSize = 0
}

// Reset clears the Response for reuse.
func (r *Response) Reset() {
	r.RequestID = ""
	r.StatusCode = 0
	r.Headers = r.Headers[:0]
	r.Body = nil
	r.Error = nil
	r.Timing = nil
	r.EndpointID = ""
	r.SessionID = ""
	r.IsStreaming = false
}
