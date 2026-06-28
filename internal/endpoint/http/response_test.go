package http

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"testing"

	"github.com/andybalholm/brotli"
	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/pkg/protocol"
)

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func TestBuildResponse_Basic(t *testing.T) {
	body := []byte("Hello, World!")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{
		Total: 100,
	}

	protoResp, err := BuildResponse("req-123", resp, timing, DefaultMaxBodySize, "endpoint-1", "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if protoResp.RequestID != "req-123" {
		t.Errorf("expected request ID req-123, got %s", protoResp.RequestID)
	}

	if protoResp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", protoResp.StatusCode)
	}

	if string(protoResp.Body) != "Hello, World!" {
		t.Errorf("expected body 'Hello, World!', got %s", string(protoResp.Body))
	}

	if protoResp.EndpointID != "endpoint-1" {
		t.Errorf("expected endpoint ID endpoint-1, got %s", protoResp.EndpointID)
	}

	if protoResp.SessionID != "session-1" {
		t.Errorf("expected session ID session-1, got %s", protoResp.SessionID)
	}

	if protoResp.Timing == nil || protoResp.Timing.Total != 100 {
		t.Error("expected timing info to be set")
	}
}

func TestBuildResponse_GzipDecompression(t *testing.T) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, _ = gzWriter.Write([]byte("Gzipped content"))
	_ = gzWriter.Close()

	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Encoding": []string{"gzip"},
			"Content-Type":     []string{"text/plain"},
		},
		Body: nopCloser{bytes.NewReader(buf.Bytes())},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-gzip", resp, timing, DefaultMaxBodySize, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(protoResp.Body) != "Gzipped content" {
		t.Errorf("expected decompressed body 'Gzipped content', got %s", string(protoResp.Body))
	}
}

func TestBuildResponse_BrotliDecompression(t *testing.T) {
	var buf bytes.Buffer
	brWriter := brotli.NewWriter(&buf)
	_, _ = brWriter.Write([]byte("Brotli content"))
	_ = brWriter.Close()

	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Encoding": []string{"br"},
			"Content-Type":     []string{"text/plain"},
		},
		Body: nopCloser{bytes.NewReader(buf.Bytes())},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-br", resp, timing, DefaultMaxBodySize, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(protoResp.Body) != "Brotli content" {
		t.Errorf("expected decompressed body 'Brotli content', got %s", string(protoResp.Body))
	}
}

func TestBuildResponse_NoCompression(t *testing.T) {
	body := []byte("Plain text content")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-plain", resp, timing, DefaultMaxBodySize, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(protoResp.Body) != "Plain text content" {
		t.Errorf("expected body 'Plain text content', got %s", string(protoResp.Body))
	}
}

func TestBuildResponse_IdentityEncoding(t *testing.T) {
	body := []byte("Identity encoded content")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Encoding": []string{"identity"},
			"Content-Type":     []string{"text/plain"},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-identity", resp, timing, DefaultMaxBodySize, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(protoResp.Body) != "Identity encoded content" {
		t.Errorf("expected body 'Identity encoded content', got %s", string(protoResp.Body))
	}
}

func TestBuildResponse_NilBody(t *testing.T) {
	resp := &fhttp.Response{
		StatusCode: 204,
		Header:     fhttp.Header{},
		Body:       nil,
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-no-body", resp, timing, DefaultMaxBodySize, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(protoResp.Body) != 0 {
		t.Errorf("expected empty body, got %d bytes", len(protoResp.Body))
	}
}

func TestBuildResponse_LargeBody(t *testing.T) {
	maxSize := int64(100)
	largeBody := bytes.Repeat([]byte("x"), 200)

	resp := &fhttp.Response{
		StatusCode: 200,
		Header:     fhttp.Header{},
		Body:       nopCloser{bytes.NewReader(largeBody)},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-large", resp, timing, maxSize, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if int64(len(protoResp.Body)) != maxSize {
		t.Errorf("expected body to be truncated to %d bytes, got %d", maxSize, len(protoResp.Body))
	}
}

func TestBuildResponse_HeadersPreserved(t *testing.T) {
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Type":  []string{"application/json"},
			"X-Custom":      []string{"value1"},
			"X-Multi-Value": []string{"a", "b"},
			"Cache-Control": []string{"no-cache"},
		},
		Body: nopCloser{bytes.NewReader([]byte("{}"))},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-headers", resp, timing, DefaultMaxBodySize, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if protoResp.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %s", protoResp.Headers.Get("Content-Type"))
	}

	if protoResp.Headers.Get("X-Custom") != "value1" {
		t.Errorf("expected X-Custom 'value1', got %s", protoResp.Headers.Get("X-Custom"))
	}

	if protoResp.Headers.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control 'no-cache', got %s", protoResp.Headers.Get("Cache-Control"))
	}
}

func TestIsSuccessStatus(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{200, true},
		{201, true},
		{204, true},
		{299, true},
		{300, false},
		{400, false},
		{500, false},
	}

	for _, tt := range tests {
		if result := IsSuccessStatus(tt.code); result != tt.expected {
			t.Errorf("IsSuccessStatus(%d) = %v, expected %v", tt.code, result, tt.expected)
		}
	}
}

func TestIsRedirectStatus(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{301, true},
		{302, true},
		{307, true},
		{308, true},
		{200, false},
		{400, false},
	}

	for _, tt := range tests {
		if result := IsRedirectStatus(tt.code); result != tt.expected {
			t.Errorf("IsRedirectStatus(%d) = %v, expected %v", tt.code, result, tt.expected)
		}
	}
}

func TestIsClientErrorStatus(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{400, true},
		{401, true},
		{403, true},
		{404, true},
		{429, true},
		{200, false},
		{500, false},
	}

	for _, tt := range tests {
		if result := IsClientErrorStatus(tt.code); result != tt.expected {
			t.Errorf("IsClientErrorStatus(%d) = %v, expected %v", tt.code, result, tt.expected)
		}
	}
}

func TestIsServerErrorStatus(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{200, false},
		{404, false},
	}

	for _, tt := range tests {
		if result := IsServerErrorStatus(tt.code); result != tt.expected {
			t.Errorf("IsServerErrorStatus(%d) = %v, expected %v", tt.code, result, tt.expected)
		}
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{200, false},
		{400, false},
		{403, false},
		{404, false},
	}

	for _, tt := range tests {
		if result := IsRetryableStatus(tt.code); result != tt.expected {
			t.Errorf("IsRetryableStatus(%d) = %v, expected %v", tt.code, result, tt.expected)
		}
	}
}

func TestShouldEscalatePool(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{403, true},
		{407, true},
		{200, false},
		{404, false},
		{429, false},
		{500, false},
	}

	for _, tt := range tests {
		if result := ShouldEscalatePool(tt.code); result != tt.expected {
			t.Errorf("ShouldEscalatePool(%d) = %v, expected %v", tt.code, result, tt.expected)
		}
	}
}

func TestBuildResponseWithOptions_StreamingResponse(t *testing.T) {
	body := []byte("Large file content that should not be buffered")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Type":   []string{"application/octet-stream"},
			"Content-Length": []string{"1000000000"},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	opts := ResponseOptions{
		MaxBodySize:    DefaultMaxBodySize,
		StreamResponse: true,
	}

	protoResp, err := BuildResponseWithOptions("req-stream", resp, timing, opts, "endpoint-1", "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !protoResp.IsStreaming {
		t.Error("expected IsStreaming to be true")
	}

	if protoResp.Body != nil {
		t.Error("expected body to be nil for streaming response")
	}

	if protoResp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", protoResp.StatusCode)
	}

	if protoResp.Headers.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("expected Content-Type 'application/octet-stream', got %s", protoResp.Headers.Get("Content-Type"))
	}

	if protoResp.EndpointID != "endpoint-1" {
		t.Errorf("expected endpoint ID endpoint-1, got %s", protoResp.EndpointID)
	}

	if protoResp.SessionID != "session-1" {
		t.Errorf("expected session ID session-1, got %s", protoResp.SessionID)
	}
}

func TestBuildResponseWithOptions_BufferedResponse(t *testing.T) {
	body := []byte("Normal content to buffer")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	opts := ResponseOptions{
		MaxBodySize:    DefaultMaxBodySize,
		StreamResponse: false,
	}

	protoResp, err := BuildResponseWithOptions("req-buffered", resp, timing, opts, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if protoResp.IsStreaming {
		t.Error("expected IsStreaming to be false")
	}

	if string(protoResp.Body) != "Normal content to buffer" {
		t.Errorf("expected body 'Normal content to buffer', got %s", string(protoResp.Body))
	}
}

func TestBuildResponseWithOptions_CustomMaxBodySize(t *testing.T) {
	customMaxSize := int64(10)
	largeBody := bytes.Repeat([]byte("x"), 50)

	resp := &fhttp.Response{
		StatusCode: 200,
		Header:     fhttp.Header{},
		Body:       nopCloser{bytes.NewReader(largeBody)},
	}

	timing := protocol.TimingInfo{Total: 50}
	opts := ResponseOptions{
		MaxBodySize:    customMaxSize,
		StreamResponse: false,
	}

	protoResp, err := BuildResponseWithOptions("req-custom-size", resp, timing, opts, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if int64(len(protoResp.Body)) != customMaxSize {
		t.Errorf("expected body to be truncated to %d bytes, got %d", customMaxSize, len(protoResp.Body))
	}
}

func TestBuildResponseWithOptions_DefaultMaxBodySize(t *testing.T) {
	body := []byte("Small content")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header:     fhttp.Header{},
		Body:       nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	opts := ResponseOptions{
		MaxBodySize:    0,
		StreamResponse: false,
	}

	protoResp, err := BuildResponseWithOptions("req-default-size", resp, timing, opts, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(protoResp.Body) != "Small content" {
		t.Errorf("expected body 'Small content', got %s", string(protoResp.Body))
	}
}
