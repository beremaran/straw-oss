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

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func TestBuildResponse_Basic(t *testing.T) {
	body := []byte("Hello, World!")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			ContentTypeHeader: []string{HeaderValueTextPlain},
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
			ContentEncoding:   []string{"gzip"},
			ContentTypeHeader: []string{HeaderValueTextPlain},
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
	if protoResp.Headers.Get(ContentEncoding) != "gzip" {
		t.Errorf("expected Content-Encoding gzip, got %q", protoResp.Headers.Get(ContentEncoding))
	}
}

func TestBuildResponse_BrotliBodyPreserved(t *testing.T) {
	body := []byte{0xce, 0xb2, 0xcf, 0x81}

	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			ContentEncoding:   []string{"br"},
			ContentTypeHeader: []string{HeaderValueTextPlain},
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
			ContentTypeHeader: []string{HeaderValueTextPlain},
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
			ContentEncoding:   []string{"identity"},
			ContentTypeHeader: []string{HeaderValueTextPlain},
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
			ContentTypeHeader:  []string{HeaderValueApplicationJSON},
			HeaderValueXCustom: []string{HeaderValueValue1},
			"X-Multi-Value":    []string{"a", "b"},
			"Cache-Control":    []string{"no-cache"},
		},
		Body: nopCloser{bytes.NewReader([]byte("{}"))},
	}

	timing := protocol.TimingInfo{Total: 50}
	protoResp, err := BuildResponse("req-headers", resp, timing, DefaultMaxBodySize, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if protoResp.Headers.Get(ContentTypeHeader) != HeaderValueApplicationJSON {
		t.Errorf("expected Content-Type '%s', got %s", HeaderValueApplicationJSON, protoResp.Headers.Get(ContentTypeHeader))
	}

	if protoResp.Headers.Get(HeaderValueXCustom) != HeaderValueValue1 {
		t.Errorf("expected X-Custom '%s', got %s", HeaderValueValue1, protoResp.Headers.Get(HeaderValueXCustom))
	}

	if protoResp.Headers.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control 'no-cache', got %s", protoResp.Headers.Get("Cache-Control"))
	}
}

func TestBuildResponseWithOptions_BufferedResponse(t *testing.T) {
	body := []byte("Normal content to buffer")
	resp := &fhttp.Response{
		StatusCode: 200,
		Header: fhttp.Header{
			ContentTypeHeader: []string{HeaderValueTextPlain},
		},
		Body: nopCloser{bytes.NewReader(body)},
	}

	timing := protocol.TimingInfo{Total: 50}
	opts := ResponseOptions{
		MaxBodySize: DefaultMaxBodySize,
	}

	protoResp, err := BuildResponseWithOptions("req-buffered", resp, timing, opts, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		MaxBodySize: customMaxSize,
	}

	protoResp, err := BuildResponseWithOptions("req-custom-size", resp, timing, opts, "")
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
		MaxBodySize: 0,
	}

	protoResp, err := BuildResponseWithOptions("req-default-size", resp, timing, opts, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(protoResp.Body) != "Small content" {
		t.Errorf("expected body 'Small content', got %s", string(protoResp.Body))
	}
}
