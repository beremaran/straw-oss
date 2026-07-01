package protocol

import (
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/beremaran/straw/internal/protocol/wirepb"
)

// MarshalRequest encodes a request for the Control -> Egress wire protocol.
func MarshalRequest(req *Request) ([]byte, error) {
	data, err := proto.Marshal(requestToWire(req))
	if err != nil {
		return nil, fmt.Errorf("marshal request protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalRequest decodes a Control -> Egress wire protocol request.
func UnmarshalRequest(data []byte) (*Request, error) {
	var msg wirepb.Request

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal request protobuf: %w", err)
	}

	return requestFromWire(&msg), nil
}

// MarshalResponse encodes a response for the Egress -> Control wire protocol.
func MarshalResponse(resp *Response) ([]byte, error) {
	data, err := proto.Marshal(responseToWire(resp))
	if err != nil {
		return nil, fmt.Errorf("marshal response protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalResponse decodes an Egress -> Control wire protocol response.
func UnmarshalResponse(data []byte) (*Response, error) {
	var msg wirepb.Response

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response protobuf: %w", err)
	}

	return responseFromWire(&msg), nil
}

func requestToWire(req *Request) *wirepb.Request {
	if req == nil {
		return &wirepb.Request{}
	}

	return &wirepb.Request{
		Id:              req.ID,
		Method:          req.Method,
		Url:             req.URL,
		Headers:         headersToWire(req.Headers),
		Body:            req.Body,
		TimeoutNanos:    int64(req.Timeout),
		ReplyTo:         req.ReplyTo,
		MaxResponseSize: req.MaxResponseSize,
	}
}

func requestFromWire(msg *wirepb.Request) *Request {
	return &Request{
		ID:              msg.GetId(),
		Method:          msg.GetMethod(),
		URL:             msg.GetUrl(),
		Headers:         headersFromWire(msg.GetHeaders()),
		Body:            msg.GetBody(),
		Timeout:         time.Duration(msg.GetTimeoutNanos()),
		ReplyTo:         msg.GetReplyTo(),
		MaxResponseSize: msg.GetMaxResponseSize(),
	}
}

func responseToWire(resp *Response) *wirepb.Response {
	if resp == nil {
		return &wirepb.Response{}
	}

	return &wirepb.Response{
		RequestId:  resp.RequestID,
		StatusCode: statusCodeToWire(resp.StatusCode),
		Headers:    headersToWire(resp.Headers),
		Body:       resp.Body,
		Error:      errorToWire(resp.Error),
		Timing:     timingToWire(resp.Timing),
		EgressId:   resp.EgressID,
	}
}

func statusCodeToWire(status int) int32 {
	if status > math.MaxInt32 {
		return math.MaxInt32
	}

	if status < math.MinInt32 {
		return math.MinInt32
	}

	return int32(status)
}

func responseFromWire(msg *wirepb.Response) *Response {
	return &Response{
		RequestID:  msg.GetRequestId(),
		StatusCode: int(msg.GetStatusCode()),
		Headers:    headersFromWire(msg.GetHeaders()),
		Body:       msg.GetBody(),
		Error:      errorFromWire(msg.GetError()),
		Timing:     timingFromWire(msg.GetTiming()),
		EgressID:   msg.GetEgressId(),
	}
}

func headersToWire(headers HeaderMap) []*wirepb.Header {
	result := make([]*wirepb.Header, 0, len(headers))
	for _, header := range headers {
		result = append(result, &wirepb.Header{
			Key:   header.Key,
			Value: header.Value,
		})
	}

	return result
}

func headersFromWire(headers []*wirepb.Header) HeaderMap {
	result := make(HeaderMap, 0, len(headers))
	for _, header := range headers {
		result = append(result, Header{
			Key:   header.GetKey(),
			Value: header.GetValue(),
		})
	}

	return result
}

func errorToWire(errInfo *ErrorInfo) *wirepb.ErrorInfo {
	if errInfo == nil {
		return nil
	}

	return &wirepb.ErrorInfo{
		Code:            errInfo.Code,
		Message:         errInfo.Message,
		Retryable:       errInfo.Retryable,
		RetryAfterNanos: int64(errInfo.RetryAfter),
	}
}

func errorFromWire(msg *wirepb.ErrorInfo) *ErrorInfo {
	if msg == nil {
		return nil
	}

	return &ErrorInfo{
		Code:       msg.GetCode(),
		Message:    msg.GetMessage(),
		Retryable:  msg.GetRetryable(),
		RetryAfter: time.Duration(msg.GetRetryAfterNanos()),
	}
}

func timingToWire(timing *TimingInfo) *wirepb.TimingInfo {
	if timing == nil {
		return nil
	}

	return &wirepb.TimingInfo{
		DnsLookupNanos:    int64(timing.DNSLookup),
		TcpConnectNanos:   int64(timing.TCPConnect),
		TlsHandshakeNanos: int64(timing.TLSHandshake),
		FirstByteNanos:    int64(timing.FirstByte),
		TotalNanos:        int64(timing.Total),
	}
}

func timingFromWire(msg *wirepb.TimingInfo) *TimingInfo {
	if msg == nil {
		return nil
	}

	return &TimingInfo{
		DNSLookup:    time.Duration(msg.GetDnsLookupNanos()),
		TCPConnect:   time.Duration(msg.GetTcpConnectNanos()),
		TLSHandshake: time.Duration(msg.GetTlsHandshakeNanos()),
		FirstByte:    time.Duration(msg.GetFirstByteNanos()),
		Total:        time.Duration(msg.GetTotalNanos()),
	}
}
