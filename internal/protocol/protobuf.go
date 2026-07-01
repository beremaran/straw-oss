package protocol

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/beremaran/straw/internal/protocol/wirepb"
)

// MarshalRequest encodes a request for the Control -> Egress wire protocol.
func MarshalRequest(req *wirepb.Request) ([]byte, error) {
	data, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalRequest decodes a Control -> Egress wire protocol request.
func UnmarshalRequest(data []byte) (*wirepb.Request, error) {
	var msg wirepb.Request

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal request protobuf: %w", err)
	}

	return &msg, nil
}

// MarshalResponse encodes a response for the Egress -> Control wire protocol.
func MarshalResponse(resp *wirepb.Response) ([]byte, error) {
	data, err := proto.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal response protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalResponse decodes an Egress -> Control wire protocol response.
func UnmarshalResponse(data []byte) (*wirepb.Response, error) {
	var msg wirepb.Response

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response protobuf: %w", err)
	}

	return &msg, nil
}
