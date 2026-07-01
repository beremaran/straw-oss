package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/protocol"
)

func TestControlHandlerPublishesAndWritesEgressResult(t *testing.T) {
	resultBody, err := protocol.MarshalResponse(&protocol.Response{
		RequestID:  "req-1",
		EgressID:   "egress-1",
		StatusCode: http.StatusAccepted,
		Headers: protocol.HeaderMap{
			{Key: "Content-Type", Value: "text/plain"},
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

type controlMockBroker struct {
	publishSubject string
	publishBody    []byte
	consumeSubject string
	result         []byte
}

func (m *controlMockBroker) Publish(_ context.Context, subject string, body []byte) error {
	m.publishSubject = subject
	m.publishBody = append([]byte(nil), body...)

	return nil
}

func (m *controlMockBroker) ConsumeOnce(_ context.Context, subject string, _ time.Duration) ([]byte, error) {
	m.consumeSubject = subject

	return m.result, nil
}
