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

func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	return encoder.EncodeAll(data, nil), nil
}

func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	return decoder.DecodeAll(data, nil)
}

func CompressionRatio(original, compressed []byte) float64 {
	if len(original) == 0 {
		return 0
	}
	return float64(len(compressed)) / float64(len(original))
}
