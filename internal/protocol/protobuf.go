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

// MarshalTunnelOpen encodes a tunnel open request.
func MarshalTunnelOpen(msg *wirepb.TunnelOpen) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal tunnel open protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalTunnelOpen decodes a tunnel open request.
func UnmarshalTunnelOpen(data []byte) (*wirepb.TunnelOpen, error) {
	var msg wirepb.TunnelOpen

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal tunnel open protobuf: %w", err)
	}

	return &msg, nil
}

// MarshalTunnelOpenResult encodes a tunnel open result.
func MarshalTunnelOpenResult(msg *wirepb.TunnelOpenResult) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal tunnel open result protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalTunnelOpenResult decodes a tunnel open result.
func UnmarshalTunnelOpenResult(data []byte) (*wirepb.TunnelOpenResult, error) {
	var msg wirepb.TunnelOpenResult

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal tunnel open result protobuf: %w", err)
	}

	return &msg, nil
}

// MarshalTunnelChunk encodes a tunnel data chunk.
func MarshalTunnelChunk(msg *wirepb.TunnelChunk) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal tunnel chunk protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalTunnelChunk decodes a tunnel data chunk.
func UnmarshalTunnelChunk(data []byte) (*wirepb.TunnelChunk, error) {
	var msg wirepb.TunnelChunk

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal tunnel chunk protobuf: %w", err)
	}

	return &msg, nil
}

// MarshalTunnelClose encodes a tunnel close message.
func MarshalTunnelClose(msg *wirepb.TunnelClose) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal tunnel close protobuf: %w", err)
	}

	return data, nil
}

// UnmarshalTunnelClose decodes a tunnel close message.
func UnmarshalTunnelClose(data []byte) (*wirepb.TunnelClose, error) {
	var msg wirepb.TunnelClose

	err := proto.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal tunnel close protobuf: %w", err)
	}

	return &msg, nil
}
