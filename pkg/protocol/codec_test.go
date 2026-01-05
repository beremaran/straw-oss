package protocol

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestCompress_EmptyData(t *testing.T) {
	result, err := Compress([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d bytes", len(result))
	}
}

func TestDecompress_EmptyData(t *testing.T) {
	result, err := Decompress([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d bytes", len(result))
	}
}

func TestCompressDecompress_RoundTrip_SmallData(t *testing.T) {
	original := []byte("Hello, Straw Proxy!")

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Errorf("round-trip failed\noriginal:     %s\ndecompressed: %s", original, decompressed)
	}
}

func TestCompressDecompress_RoundTrip_LargeData(t *testing.T) {
	// Create a large, repetitive dataset (compresses well)
	original := []byte(strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10000))

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Errorf("round-trip failed: lengths differ (original=%d, decompressed=%d)",
			len(original), len(decompressed))
	}

	// Verify compression actually reduced size
	ratio := CompressionRatio(original, compressed)
	t.Logf("Compression ratio: %.2f (original=%d, compressed=%d)",
		ratio, len(original), len(compressed))

	if ratio >= 1.0 {
		t.Errorf("expected compression to reduce size, but ratio=%.2f", ratio)
	}
}

func TestCompressDecompress_RoundTrip_RandomData(t *testing.T) {
	// Random data doesn't compress well but should still round-trip correctly
	original := make([]byte, 1024)
	_, err := rand.Read(original)
	if err != nil {
		t.Fatalf("failed to generate random data: %v", err)
	}

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Errorf("round-trip failed for random data")
	}
}

func TestCompressDecompress_RoundTrip_JSONLike(t *testing.T) {
	// Simulate JSON payload like what we'd actually compress
	original := []byte(`{
		"id": "req-12345",
		"method": "GET",
		"url": "https://example.com/api/data",
		"headers": [
			{"key": "User-Agent", "value": "Mozilla/5.0"},
			{"key": "Accept", "value": "application/json"},
			{"key": "Accept-Language", "value": "en-US,en;q=0.9"}
		],
		"fingerprint": "chrome-130"
	}`)

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Errorf("round-trip failed for JSON data")
	}

	t.Logf("JSON compression: %d -> %d bytes (%.1f%% reduction)",
		len(original), len(compressed), (1-CompressionRatio(original, compressed))*100)
}

func TestDecompress_CorruptedData(t *testing.T) {
	_, err := Decompress([]byte("not valid zstd data"))
	if err == nil {
		t.Error("expected error for corrupted data, got nil")
	}
}

func TestCompressionRatio(t *testing.T) {
	tests := []struct {
		name       string
		original   []byte
		compressed []byte
		expected   float64
	}{
		{"empty", []byte{}, []byte{}, 0},
		{"equal", []byte("abc"), []byte("abc"), 1.0},
		{"half", []byte("abcd"), []byte("ab"), 0.5},
		{"double", []byte("ab"), []byte("abcd"), 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := CompressionRatio(tt.original, tt.compressed)
			if ratio != tt.expected {
				t.Errorf("expected %.2f, got %.2f", tt.expected, ratio)
			}
		})
	}
}

func BenchmarkCompress(b *testing.B) {
	data := []byte(strings.Repeat("benchmark test data ", 1000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Compress(data)
	}
}

func BenchmarkDecompress(b *testing.B) {
	data := []byte(strings.Repeat("benchmark test data ", 1000))
	compressed, _ := Compress(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decompress(compressed)
	}
}
