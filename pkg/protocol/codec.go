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
	// Use default compression level for a good balance of speed and ratio
	encoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		panic(fmt.Sprintf("failed to create zstd encoder: %v", err))
	}

	decoder, err = zstd.NewReader(nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create zstd decoder: %v", err))
	}
}

// Compress compresses data using the Zstd algorithm.
// Returns the compressed data or an error if compression fails.
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	return encoder.EncodeAll(data, nil), nil
}

// Decompress decompresses Zstd data back to the original bytes.
// Returns the decompressed data or an error if decompression fails.
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	return decoder.DecodeAll(data, nil)
}

// CompressionRatio calculates the compression ratio (compressed/original).
// A lower ratio means better compression.
func CompressionRatio(original, compressed []byte) float64 {
	if len(original) == 0 {
		return 0
	}
	return float64(len(compressed)) / float64(len(original))
}
