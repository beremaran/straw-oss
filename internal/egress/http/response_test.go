package http

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/protocol"
)

const (
	contentEncodingHeader = "Content-Encoding"
	contentTypeHeader     = "Content-Type"
	contentTypeJSON       = "application/json"
	contentTypeTextPlain  = "text/plain"
	xCustomHeader         = "X-Custom"
	xCustomValue          = "value1"
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
			contentTypeHeader: []string{contentTypeTextPlain},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{
		Total: 100,
	}

	protoResp, err := BuildResponse("req-123", resp, timing, DefaultMaxBodySize, "egress-1")
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

	if protoResp.EgressID != "egress-1" {
		t.Errorf("expected egress ID egress-1, got %s", protoResp.EgressID)
	}

	if protoResp.Timing == nil || protoResp.Timing.Total != 100 {
		t.Error("expected timing info to be set")
	}
}

func TestBuildResponse_GzipBodyPreserved(t *testing.T) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, _ = gzWriter.Write([]byte("Gzipped content"))
	_ = gzWriter.Close()
	rawBody := append([]byte(nil), buf.Bytes()...)

	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			contentEncodingHeader: []string{"gzip"},
			contentTypeHeader:     []string{contentTypeTextPlain},
		},
		Body: nopCloser{bytes.NewReader(buf.Bytes())},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-gzip", resp, timing, DefaultMaxBodySize, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(protoResp.Body, rawBody) {
		t.Errorf("expected raw gzip body %v, got %v", rawBody, protoResp.Body)
	}
	if protoResp.Headers.Get(contentEncodingHeader) != "gzip" {
		t.Errorf("expected Content-Encoding gzip, got %q", protoResp.Headers.Get(contentEncodingHeader))
	}
}

func TestBuildResponse_BrotliBodyPreserved(t *testing.T) {
	body := []byte{0xce, 0xb2, 0xcf, 0x81}

	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			contentEncodingHeader: []string{"br"},
			contentTypeHeader:     []string{contentTypeTextPlain},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-br", resp, timing, DefaultMaxBodySize, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(protoResp.Body, body) {
		t.Errorf("expected raw brotli body %v, got %v", body, protoResp.Body)
	}
}

func TestBuildResponse_NoCompression(t *testing.T) {
	body := []byte("Plain text content")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			contentTypeHeader: []string{contentTypeTextPlain},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-plain", resp, timing, DefaultMaxBodySize, "")
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
			contentEncodingHeader: []string{"identity"},
			contentTypeHeader:     []string{contentTypeTextPlain},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-identity", resp, timing, DefaultMaxBodySize, "")
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
	protoResp, err := BuildResponse("req-no-body", resp, timing, DefaultMaxBodySize, "")
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
	protoResp, err := BuildResponse("req-large", resp, timing, maxSize, "")
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
			contentTypeHeader: []string{contentTypeJSON},
			xCustomHeader:     []string{xCustomValue},
			"X-Multi-Value":   []string{"a", "b"},
			"Cache-Control":   []string{"no-cache"},
		},
		Body: nopCloser{bytes.NewReader([]byte("{}"))},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-headers", resp, timing, DefaultMaxBodySize, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if protoResp.Headers.Get(contentTypeHeader) != contentTypeJSON {
		t.Errorf("expected Content-Type '%s', got %s", contentTypeJSON, protoResp.Headers.Get(contentTypeHeader))
	}

	if protoResp.Headers.Get(xCustomHeader) != xCustomValue {
		t.Errorf("expected X-Custom '%s', got %s", xCustomValue, protoResp.Headers.Get(xCustomHeader))
	}

	if protoResp.Headers.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control 'no-cache', got %s", protoResp.Headers.Get("Cache-Control"))
	}
}
