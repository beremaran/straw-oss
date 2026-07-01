// Package protocol serialization helpers for zstd compression and JSON
// marshaling used between control and egress nodes.
package protocol

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
)

var (
	encoder *zstd.Encoder
	decoder *zstd.Decoder
)

func init() {
	var err error

	encoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		panic(fmt.Sprintf("failed to create zstd encoder: %v", err))
	}

	decoder, err = zstd.NewReader(nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create zstd decoder: %v", err))
	}
}

// Compress compresses data using zstd with the default encoder level.
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	return encoder.EncodeAll(data, nil), nil
}

// Decompress decompresses zstd-encoded data.
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	result, err := decoder.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}

	return result, nil
}

// CompressionRatio returns the ratio of compressed size to original size.
func CompressionRatio(original, compressed []byte) float64 {
	if len(original) == 0 {
		return 0
	}

	return float64(len(compressed)) / float64(len(original))
}
