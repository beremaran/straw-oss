package protocol

import (
	"testing"
	"time"

	"github.com/beremaran/straw/internal/protocol/wirepb"
)

func TestMarshalRequestRoundTrip(t *testing.T) {
	req := &wirepb.Request{
		Id:              "req-1",
		Method:          "POST",
		Url:             "https://example.com/upload",
		Headers:         []*wirepb.Header{{Key: "Content-Type", Value: "application/octet-stream"}},
		Body:            []byte{0, 1, 2, 3},
		TimeoutNanos:    int64(1500 * time.Millisecond),
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

	if decoded.GetId() != req.GetId() || decoded.GetMethod() != req.GetMethod() || decoded.GetUrl() != req.GetUrl() {
		t.Fatalf("decoded request = %#v, want %#v", decoded, req)
	}
	if decoded.GetHeaders()[0].GetValue() != "application/octet-stream" {
		t.Fatalf("decoded header = %q", decoded.GetHeaders()[0].GetValue())
	}
	if string(decoded.GetBody()) != string(req.GetBody()) {
		t.Fatalf("decoded body = %v, want %v", decoded.GetBody(), req.GetBody())
	}
	if decoded.GetTimeoutNanos() != req.GetTimeoutNanos() {
		t.Fatalf("decoded timeout = %v, want %v", decoded.GetTimeoutNanos(), req.GetTimeoutNanos())
	}
	if decoded.GetReplyTo() != req.GetReplyTo() {
		t.Fatalf("decoded reply_to = %q, want %q", decoded.GetReplyTo(), req.GetReplyTo())
	}
	if decoded.GetMaxResponseSize() != req.GetMaxResponseSize() {
		t.Fatalf("decoded max_response_size = %d, want %d", decoded.GetMaxResponseSize(), req.GetMaxResponseSize())
	}
}

func TestMarshalResponseRoundTrip(t *testing.T) {
	resp := &wirepb.Response{
		RequestId:  "req-1",
		StatusCode: 201,
		Headers:    []*wirepb.Header{{Key: "Content-Encoding", Value: "gzip"}},
		Body:       []byte{0x1f, 0x8b, 0x08},
		Error: &wirepb.ErrorInfo{
			Code:            ErrCodeUpstreamError,
			Message:         "upstream failed",
			Retryable:       true,
			RetryAfterNanos: int64(2 * time.Second),
		},
		Timing: &wirepb.TimingInfo{
			DnsLookupNanos:    int64(time.Millisecond),
			TcpConnectNanos:   int64(2 * time.Millisecond),
			TlsHandshakeNanos: int64(3 * time.Millisecond),
			FirstByteNanos:    int64(4 * time.Millisecond),
			TotalNanos:        int64(10 * time.Millisecond),
		},
		EgressId: "egress-1",
	}

	data, err := MarshalResponse(resp)
	if err != nil {
		t.Fatalf("MarshalResponse() error = %v", err)
	}

	decoded, err := UnmarshalResponse(data)
	if err != nil {
		t.Fatalf("UnmarshalResponse() error = %v", err)
	}

	if decoded.GetRequestId() != resp.GetRequestId() || decoded.GetStatusCode() != resp.GetStatusCode() || decoded.GetEgressId() != resp.GetEgressId() {
		t.Fatalf("decoded response = %#v, want %#v", decoded, resp)
	}
	if decoded.GetHeaders()[0].GetValue() != "gzip" {
		t.Fatalf("decoded content encoding = %q", decoded.GetHeaders()[0].GetValue())
	}
	if string(decoded.GetBody()) != string(resp.GetBody()) {
		t.Fatalf("decoded body = %v, want %v", decoded.GetBody(), resp.GetBody())
	}
	if decoded.GetError() == nil || decoded.GetError().GetCode() != ErrCodeUpstreamError || decoded.GetError().GetRetryAfterNanos() != int64(2*time.Second) {
		t.Fatalf("decoded error = %#v", decoded.GetError())
	}
	if decoded.GetTiming() == nil || decoded.GetTiming().GetTotalNanos() != int64(10*time.Millisecond) {
		t.Fatalf("decoded timing = %#v", decoded.GetTiming())
	}
}

func TestUnmarshalRequestRejectsInvalidProtobuf(t *testing.T) {
	_, err := UnmarshalRequest([]byte{0xff, 0xff, 0xff})
	if err == nil {
		t.Fatal("UnmarshalRequest() expected error")
	}
}
