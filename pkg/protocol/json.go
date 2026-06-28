package protocol

import (
	"encoding/json"
	"fmt"
)

func MarshalCompressed(v any) ([]byte, error) {
	jsonData, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return Compress(jsonData)
}

func UnmarshalCompressed(data []byte, v any) error {
	jsonData, err := Decompress(data)
	if err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}
	if err := json.Unmarshal(jsonData, v); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}
