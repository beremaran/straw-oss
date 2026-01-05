package protocol

import (
	"encoding/json"
	"fmt"
)

// MarshalJSON marshals a value to JSON bytes.
func MarshalJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return data, nil
}

// UnmarshalJSON unmarshals JSON bytes into a value.
func UnmarshalJSON(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return nil
}

// MarshalCompressed marshals a value to JSON then compresses it with LZMA.
// This is the recommended way to serialize requests for wire transport.
func MarshalCompressed(v any) ([]byte, error) {
	jsonData, err := MarshalJSON(v)
	if err != nil {
		return nil, err
	}
	return Compress(jsonData)
}

// UnmarshalCompressed decompresses LZMA data then unmarshals JSON.
// This is the recommended way to deserialize requests from wire transport.
func UnmarshalCompressed(data []byte, v any) error {
	jsonData, err := Decompress(data)
	if err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}
	return UnmarshalJSON(jsonData, v)
}

// MarshalRequest is a convenience function to marshal a Request.
func MarshalRequest(req *Request) ([]byte, error) {
	return MarshalJSON(req)
}

// UnmarshalRequest is a convenience function to unmarshal a Request.
func UnmarshalRequest(data []byte) (*Request, error) {
	var req Request
	if err := UnmarshalJSON(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// MarshalResponse is a convenience function to marshal a Response.
func MarshalResponse(resp *Response) ([]byte, error) {
	return MarshalJSON(resp)
}

// UnmarshalResponse is a convenience function to unmarshal a Response.
func UnmarshalResponse(data []byte) (*Response, error) {
	var resp Response
	if err := UnmarshalJSON(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// MarshalSignedTask is a convenience function to marshal a SignedTask.
func MarshalSignedTask(task *SignedTask) ([]byte, error) {
	return MarshalJSON(task)
}

// UnmarshalSignedTask is a convenience function to unmarshal a SignedTask.
func UnmarshalSignedTask(data []byte) (*SignedTask, error) {
	var task SignedTask
	if err := UnmarshalJSON(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// UnmarshalRequestTo unmarshals JSON data into the provided Request object.
func UnmarshalRequestTo(data []byte, req *Request) error {
	return UnmarshalJSON(data, req)
}

// UnmarshalResponseTo unmarshals JSON data into the provided Response object.
func UnmarshalResponseTo(data []byte, resp *Response) error {
	return UnmarshalJSON(data, resp)
}

// UnmarshalCompressedTo decompresses and unmarshals into the provided value.
func UnmarshalCompressedTo(data []byte, v any) error {
	jsonData, err := Decompress(data)
	if err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}
	return UnmarshalJSON(jsonData, v)
}
