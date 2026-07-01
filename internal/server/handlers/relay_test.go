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

func TestRelayHandlerPublishesAndWritesEndpointResult(t *testing.T) {
	mb := &relayMockBroker{
		result: `{
			"request_id":"req-1",
			"endpoint_id":"endpoint-1",
			"status_code":202,
			"headers":[{"key":"Content-Type","value":"text/plain"}],
			"body":"b2s="
		}`,
	}
	handler := NewRelayHandler(mb, "endpoint-1", []byte("secret"), time.Second)

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
	if mb.publishSubject != "tasks.endpoint-1.tasks" {
		t.Fatalf("publish subject = %q", mb.publishSubject)
	}
	if mb.consumeSubject != "results.req-1" {
		t.Fatalf("consume subject = %q", mb.consumeSubject)
	}

	var task protocol.SignedTask
	err := json.Unmarshal(mb.publishBody, &task)
	if err != nil {
		t.Fatalf("published task is not JSON: %v", err)
	}

	signedReq, err := protocol.ValidateSignedTask(&task, []byte("secret"), time.Minute)
	if err != nil {
		t.Fatalf("published task signature invalid: %v", err)
	}
	if signedReq.ReplyTo != "results.req-1" {
		t.Fatalf("reply_to = %q, want results.req-1", signedReq.ReplyTo)
	}
}

type relayMockBroker struct {
	publishSubject string
	publishBody    []byte
	consumeSubject string
	result         string
}

func (m *relayMockBroker) Publish(_ context.Context, subject string, body []byte) error {
	m.publishSubject = subject
	m.publishBody = append([]byte(nil), body...)

	return nil
}

func (m *relayMockBroker) Subscribe(context.Context, string, broker.Handler, ...broker.SubscribeOption) error {
	return nil
}

func (m *relayMockBroker) ConsumeOnce(_ context.Context, subject string, _ time.Duration) ([]byte, error) {
	m.consumeSubject = subject

	return []byte(m.result), nil
}

func (m *relayMockBroker) DeclareStream(context.Context, string, ...string) error { return nil }

func (m *relayMockBroker) IsConnected() bool { return true }

func (m *relayMockBroker) Close() error { return nil }
