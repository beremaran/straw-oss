package protocol

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ulikunitz/xz/lzma"
)

// Compress compresses data using the LZMA algorithm.
// Returns the compressed data or an error if compression fails.
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	var buf bytes.Buffer
	writer, err := lzma.NewWriter(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create LZMA writer: %w", err)
	}

	_, err = writer.Write(data)
	if err != nil {
		return nil, fmt.Errorf("failed to write data: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close LZMA writer: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress decompresses LZMA data back to the original bytes.
// Returns the decompressed data or an error if decompression fails.
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	reader, err := lzma.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create LZMA reader: %w", err)
	}

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}

	return decompressed, nil
}

// CompressionRatio calculates the compression ratio (compressed/original).
// A lower ratio means better compression.
func CompressionRatio(original, compressed []byte) float64 {
	if len(original) == 0 {
		return 0
	}
	return float64(len(compressed)) / float64(len(original))
}
