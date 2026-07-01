package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/protocol"
)

const (
	testReqID           = "req-1"
	testEgressID        = "egress-1"
	testContentType     = "text/plain"
	testJSONContentType = "application/json"
)

func TestControlHandlerPublishesAndWritesEgressResult(t *testing.T) {
	resultBody, err := protocol.MarshalResponse(&protocol.Response{
		RequestID:  testReqID,
		EgressID:   testEgressID,
		StatusCode: http.StatusAccepted,
		Headers: protocol.HeaderMap{
			{Key: "Content-Type", Value: testContentType},
			{Key: "Content-Encoding", Value: "gzip"},
		},
		Body: []byte("ok"),
		Timing: &protocol.TimingInfo{
			Total: time.Second,
		},
	})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	mb := &controlMockBroker{
		result: resultBody,
	}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader(`{"id":"req-1","method":"GET","url":"https://example.com"}`),
	)
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want ok", rec.Body.String())
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	if mb.publishSubject != "tasks.egress-1.tasks" {
		t.Fatalf("publish subject = %q", mb.publishSubject)
	}
	if mb.consumeSubject != "results.req-1" {
		t.Fatalf("consume subject = %q", mb.consumeSubject)
	}

	publishedReq, err := protocol.UnmarshalRequest(mb.publishBody)
	if err != nil {
		t.Fatalf("published task is not protobuf: %v", err)
	}
	if publishedReq.ReplyTo != "results.req-1" {
		t.Fatalf("reply_to = %q, want results.req-1", publishedReq.ReplyTo)
	}
}

func TestControlHandlerRejectsInvalidJSON(t *testing.T) {
	mb := &controlMockBroker{}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader("not json"),
	)
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestControlHandlerRejectsMissingURL(t *testing.T) {
	mb := &controlMockBroker{}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader(`{"id":"req-1","method":"GET"}`),
	)
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestControlHandlerDefaultsMethodToGet(t *testing.T) {
	resultBody, _ := protocol.MarshalResponse(&protocol.Response{
		RequestID:  testReqID,
		EgressID:   testEgressID,
		StatusCode: http.StatusOK,
		Body:       []byte("ok"),
	})

	mb := &controlMockBroker{result: resultBody}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader(`{"id":"req-1","url":"https://example.com"}`),
	)
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	publishedReq, err := protocol.UnmarshalRequest(mb.publishBody)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if publishedReq.Method != http.MethodGet {
		t.Fatalf("method = %q, want %q", publishedReq.Method, http.MethodGet)
	}
}

func TestControlHandlerUsesRequestIDFromHeader(t *testing.T) {
	resultBody, _ := protocol.MarshalResponse(&protocol.Response{
		RequestID:  testReqID,
		EgressID:   testEgressID,
		StatusCode: http.StatusOK,
		Body:       []byte("ok"),
	})

	mb := &controlMockBroker{result: resultBody}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader(`{"url":"https://example.com"}`),
	)
	req.Header.Set("X-Request-ID", "custom-id-123")
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	publishedReq, err := protocol.UnmarshalRequest(mb.publishBody)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if publishedReq.ID != "custom-id-123" {
		t.Fatalf("request ID = %q, want custom-id-123", publishedReq.ID)
	}
}

func TestControlHandlerPublishFailure(t *testing.T) {
	mb := &controlMockBroker{
		publishErr: broker.ErrTimeout,
	}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader(`{"id":"req-1","method":"GET","url":"https://example.com"}`),
	)
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestControlHandlerTimeout(t *testing.T) {
	mb := &controlMockBroker{
		consumeErr: broker.ErrTimeout,
	}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader(`{"id":"req-1","method":"GET","url":"https://example.com"}`),
	)
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}

	var resp errorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error["code"] != protocol.ErrCodeEgressTimeout {
		t.Fatalf("error code = %v, want %s", resp.Error["code"], protocol.ErrCodeEgressTimeout)
	}
}

func TestControlHandlerInvalidEgressResponse(t *testing.T) {
	mb := &controlMockBroker{
		result: []byte("not valid protobuf"),
	}
	handler := NewControlHandler(mb, "egress-1", time.Second, false)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/request",
		strings.NewReader(`{"id":"req-1","method":"GET","url":"https://example.com"}`),
	)
	rec := httptest.NewRecorder()

	handler.Handle(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestWriteControlResultWithEgressError(t *testing.T) {
	resp := &protocol.Response{
		StatusCode: http.StatusOK,
		Body:       []byte("ok"),
		Error: &protocol.ErrorInfo{
			Code:      protocol.ErrCodeEgressTimeout,
			Message:   "timeout",
			Retryable: true,
		},
	}

	rec := httptest.NewRecorder()
	writeControlResult(rec, resp)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestWriteControlResultFilteredHeaders(t *testing.T) {
	resp := &protocol.Response{
		StatusCode: http.StatusOK,
		Headers: protocol.HeaderMap{
			{Key: "Content-Type", Value: testContentType},
			{Key: "Connection", Value: "close"},
			{Key: "Transfer-Encoding", Value: "chunked"},
		},
		Body: []byte("ok"),
	}

	rec := httptest.NewRecorder()
	writeControlResult(rec, resp)

	if rec.Header().Get("Content-Type") != testContentType {
		t.Errorf("Content-Type = %q, want %s", rec.Header().Get("Content-Type"), testContentType)
	}
	if rec.Header().Get("Connection") != "" {
		t.Errorf("Connection should be filtered, got %q", rec.Header().Get("Connection"))
	}
	if rec.Header().Get("Transfer-Encoding") != "" {
		t.Errorf("Transfer-Encoding should be filtered, got %q", rec.Header().Get("Transfer-Encoding"))
	}
}

func TestIsFilteredResponseHeader(t *testing.T) {
	if !isFilteredResponseHeader("Connection") {
		t.Error("Connection should be filtered")
	}
	if isFilteredResponseHeader("content-type") {
		t.Error("content-type should be filtered (case-insensitive)")
	}
	if isFilteredResponseHeader("X-Custom") {
		t.Error("X-Custom should not be filtered")
	}
}

func TestReadJSON(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(`{"key":"value"}`))
	var dst map[string]any
	err := readJSON(req, &dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst["key"] != "value" {
		t.Fatalf("key = %v, want value", dst["key"])
	}
}

func TestReadJSONDecodeFailure(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader("not json"))
	var dst map[string]any
	err := readJSON(req, &dst)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if rec.Header().Get("Content-Type") != testJSONContentType {
		t.Errorf("Content-Type = %q, want %s", rec.Header().Get("Content-Type"), testJSONContentType)
	}
}

func TestWriteTimeoutResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTimeoutResponse(w, "req-123")
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}
	if rec.Header().Get("Content-Type") != testJSONContentType {
		t.Errorf("Content-Type = %q, want %s", rec.Header().Get("Content-Type"), testJSONContentType)
	}
}

func TestWriteEgressError(t *testing.T) {
	errInfo := &protocol.ErrorInfo{
		Code:      protocol.ErrCodeEgressTimeout,
		Message:   "connection refused",
		Retryable: false,
	}

	rec := httptest.NewRecorder()
	writeEgressError(rec, http.StatusBadGateway, errInfo)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if rec.Header().Get("Content-Type") != testJSONContentType {
		t.Errorf("Content-Type = %q, want %s", rec.Header().Get("Content-Type"), testJSONContentType)
	}
}

type controlMockBroker struct {
	publishSubject string
	publishBody    []byte
	publishErr     error
	consumeSubject string
	consumeErr     error
	result         []byte
}

func (m *controlMockBroker) Publish(_ context.Context, subject string, body []byte) error {
	m.publishSubject = subject
	m.publishBody = append([]byte(nil), body...)

	return m.publishErr
}

func (m *controlMockBroker) ConsumeOnce(_ context.Context, subject string, _ time.Duration) ([]byte, error) {
	m.consumeSubject = subject

	return m.result, m.consumeErr
}
