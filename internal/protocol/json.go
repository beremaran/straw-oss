package protocol

import (
	"encoding/json"
	"fmt"
)

// MarshalCompressed serializes v to JSON and compresses the result with zstd.
func MarshalCompressed(v any) ([]byte, error) {
	jsonData, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return Compress(jsonData)
}

// UnmarshalCompressed decompresses data and deserializes it into v.
func UnmarshalCompressed(data []byte, v any) error {
	jsonData, err := Decompress(data)
	if err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}

	err = json.Unmarshal(jsonData, v)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}
