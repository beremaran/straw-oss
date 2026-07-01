package protocol

import (
	"testing"
	"time"
)

func TestMarshalRequestRoundTrip(t *testing.T) {
	req := &Request{
		ID:              "req-1",
		Method:          "POST",
		URL:             "https://example.com/upload",
		Headers:         HeaderMap{{Key: "Content-Type", Value: "application/octet-stream"}},
		Body:            []byte{0, 1, 2, 3},
		Timeout:         1500 * time.Millisecond,
		ReplyTo:         "results.req-1",
		MaxResponseSize: 4096,
	}

	data, err := MarshalRequest(req)
	if err != nil {
		t.Fatalf("MarshalRequest() error = %v", err)
	}

	decoded, err := UnmarshalRequest(data)
	if err != nil {
		t.Fatalf("UnmarshalRequest() error = %v", err)
	}

	if decoded.ID != req.ID || decoded.Method != req.Method || decoded.URL != req.URL {
		t.Fatalf("decoded request = %#v, want %#v", decoded, req)
	}
	if decoded.Headers.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("decoded header = %q", decoded.Headers.Get("Content-Type"))
	}
	if string(decoded.Body) != string(req.Body) {
		t.Fatalf("decoded body = %v, want %v", decoded.Body, req.Body)
	}
	if decoded.Timeout != req.Timeout {
		t.Fatalf("decoded timeout = %v, want %v", decoded.Timeout, req.Timeout)
	}
	if decoded.ReplyTo != req.ReplyTo {
		t.Fatalf("decoded reply_to = %q, want %q", decoded.ReplyTo, req.ReplyTo)
	}
	if decoded.MaxResponseSize != req.MaxResponseSize {
		t.Fatalf("decoded max_response_size = %d, want %d", decoded.MaxResponseSize, req.MaxResponseSize)
	}
}

func TestMarshalResponseRoundTrip(t *testing.T) {
	resp := &Response{
		RequestID:  "req-1",
		StatusCode: 201,
		Headers:    HeaderMap{{Key: "Content-Encoding", Value: "gzip"}},
		Body:       []byte{0x1f, 0x8b, 0x08},
		Error: &ErrorInfo{
			Code:       ErrCodeUpstreamError,
			Message:    "upstream failed",
			Retryable:  true,
			RetryAfter: 2 * time.Second,
		},
		Timing: &TimingInfo{
			DNSLookup:    time.Millisecond,
			TCPConnect:   2 * time.Millisecond,
			TLSHandshake: 3 * time.Millisecond,
			FirstByte:    4 * time.Millisecond,
			Total:        10 * time.Millisecond,
		},
		EgressID: "egress-1",
	}

	data, err := MarshalResponse(resp)
	if err != nil {
		t.Fatalf("MarshalResponse() error = %v", err)
	}

	decoded, err := UnmarshalResponse(data)
	if err != nil {
		t.Fatalf("UnmarshalResponse() error = %v", err)
	}

	if decoded.RequestID != resp.RequestID || decoded.StatusCode != resp.StatusCode || decoded.EgressID != resp.EgressID {
		t.Fatalf("decoded response = %#v, want %#v", decoded, resp)
	}
	if decoded.Headers.Get("Content-Encoding") != "gzip" {
		t.Fatalf("decoded content encoding = %q", decoded.Headers.Get("Content-Encoding"))
	}
	if string(decoded.Body) != string(resp.Body) {
		t.Fatalf("decoded body = %v, want %v", decoded.Body, resp.Body)
	}
	if decoded.Error == nil || decoded.Error.Code != ErrCodeUpstreamError || decoded.Error.RetryAfter != 2*time.Second {
		t.Fatalf("decoded error = %#v", decoded.Error)
	}
	if decoded.Timing == nil || decoded.Timing.Total != 10*time.Millisecond {
		t.Fatalf("decoded timing = %#v", decoded.Timing)
	}
}

func TestUnmarshalRequestRejectsInvalidProtobuf(t *testing.T) {
	_, err := UnmarshalRequest([]byte{0xff, 0xff, 0xff})
	if err == nil {
		t.Fatal("UnmarshalRequest() expected error")
	}
}
