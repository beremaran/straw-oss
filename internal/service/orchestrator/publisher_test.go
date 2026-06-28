package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

type mockBroker struct {
	publishedMsgs []publishedMsg
	subscriptions []subscription
	mu            sync.Mutex
	publishErr    error
	subscribeErr  error
}

type publishedMsg struct {
	Subject string
	Body    []byte
}

type subscription struct {
	Subject string
	IsTemp  bool
	Handler broker.Handler
}

func (m *mockBroker) Publish(ctx context.Context, subject string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.publishErr != nil {
		return m.publishErr
	}
	m.publishedMsgs = append(m.publishedMsgs, publishedMsg{
		Subject: subject,
		Body:    body,
	})

	return nil
}

func (m *mockBroker) Subscribe(ctx context.Context, subject string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subscribeErr != nil {
		return m.subscribeErr
	}
	m.subscriptions = append(m.subscriptions, subscription{
		Subject: subject,
		IsTemp:  false,
		Handler: handler,
	})

	return nil
}

func (m *mockBroker) ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockBroker) Close() error {
	return nil
}

func (m *mockBroker) DeclareStream(ctx context.Context, name string, subjects ...string) error {
	return nil
}

func (m *mockBroker) IsConnected() bool {
	return true
}

type mockSelector struct {
	endpointID string
	err        error
}

func (m *mockSelector) Select(ctx context.Context, rule *domain.RoutingRule) (string, error) {
	return m.endpointID, m.err
}

func (m *mockSelector) SelectWithSession(ctx context.Context, sessionID string) (string, error) {
	return m.endpointID, m.err
}

func TestPublisher_Publish(t *testing.T) {
	mb := &mockBroker{}
	ms := &mockSelector{endpointID: "ep-1"}
	secret := []byte("secret")
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, secret, cb)

	req := &protocol.Request{
		Method: "GET",
		URL:    "http://example.com",
	}
	rule := &domain.RoutingRule{ID: "rule-1"}

	replyQueue := "results.123"
	eid, err := p.Publish(context.Background(), req, rule, "", "", replyQueue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if eid != "ep-1" {
		t.Errorf("expected endpointID ep-1, got %s", eid)
	}

	if req.ReplyTo != replyQueue {
		t.Errorf("expected ReplyTo %s, got %s", replyQueue, req.ReplyTo)
	}

	mb.mu.Lock()
	defer mb.mu.Unlock()

	if len(mb.subscriptions) != 0 {
		t.Errorf("expected 0 subscriptions (Publish no longer subscribes), got %d", len(mb.subscriptions))
	}

	if len(mb.publishedMsgs) != 1 {
		t.Errorf("expected 1 published message, got %d", len(mb.publishedMsgs))
	} else {
		msg := mb.publishedMsgs[0]
		expectedSubject := "tasks.ep-1.tasks"
		if msg.Subject != expectedSubject {
			t.Errorf("expected subject %s, got %s", expectedSubject, msg.Subject)
		}

		var st protocol.SignedTask
		err := json.Unmarshal(msg.Body, &st)
		if err != nil {
			t.Errorf("failed to unmarshal signed task: %v", err)
		}

		_, err = protocol.ValidateSignedTask(&st, secret, 5*time.Second)
		if err != nil {
			t.Errorf("published task failed validation: %v", err)
		}
	}
}

func TestPublisher_Publish_SelectorError(t *testing.T) {
	mb := &mockBroker{}
	ms := &mockSelector{err: errors.New("no endpoints")}
	secret := []byte("secret")
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, secret, cb)

	req := &protocol.Request{}
	rule := &domain.RoutingRule{}

	_, err := p.Publish(context.Background(), req, rule, "", "", "res-q")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestPublisher_Publish_SubscribeError(t *testing.T) {
	mb := &mockBroker{subscribeErr: errors.New("sub failed")}
	ms := &mockSelector{endpointID: "ep-1"}
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, []byte("secret"), cb)

	req := &protocol.Request{ID: "req-1"}
	rule := &domain.RoutingRule{}

	_, err := p.Publish(context.Background(), req, rule, "", "", "res-q")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPublisher_Publish_BrokerError(t *testing.T) {
	mb := &mockBroker{publishErr: errors.New("publish failed")}
	ms := &mockSelector{endpointID: "ep-1"}
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, []byte("secret"), cb)

	req := &protocol.Request{ID: "req-1"}
	rule := &domain.RoutingRule{}

	_, err := p.Publish(context.Background(), req, rule, "", "", "res-q")
	if err == nil {
		t.Error("expected error, got nil")
	}

	expectedErrStr := "failed to publish task (circuit breaker): publish failed"
	if err != nil && err.Error() != expectedErrStr {
		t.Errorf("expected error %q, got %v", expectedErrStr, err)
	}
}

func TestPublisher_Publish_WithSessionID(t *testing.T) {
	mb := &mockBroker{}
	ms := &mockSelector{endpointID: "ep-session-1"}
	secret := []byte("secret")
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, secret, cb)

	req := &protocol.Request{Method: "GET", URL: "http://example.com"}
	rule := &domain.RoutingRule{ID: "rule-1"}

	eid, err := p.Publish(context.Background(), req, rule, "session-123", "", "res-q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if eid != "ep-session-1" {
		t.Errorf("expected endpointID 'ep-session-1', got %s", eid)
	}
}

func TestPublisher_Publish_WithTargetEndpointID(t *testing.T) {
	mb := &mockBroker{}
	ms := &mockSelector{endpointID: "ep-default"}
	secret := []byte("secret")
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, secret, cb)

	req := &protocol.Request{Method: "GET", URL: "http://example.com"}
	rule := &domain.RoutingRule{ID: "rule-1"}

	eid, err := p.Publish(context.Background(), req, rule, "", "ep-target-1", "res-q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if eid != "ep-target-1" {
		t.Errorf("expected endpointID 'ep-target-1', got %s", eid)
	}

	mb.mu.Lock()
	defer mb.mu.Unlock()
	if len(mb.publishedMsgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(mb.publishedMsgs))
	}

	msg := mb.publishedMsgs[0]
	expectedSubject := "tasks.ep-target-1.tasks"
	if msg.Subject != expectedSubject {
		t.Errorf("expected subject %s, got %s", expectedSubject, msg.Subject)
	}
}

func TestPublisher_Publish_GeneratesRequestID(t *testing.T) {
	mb := &mockBroker{}
	ms := &mockSelector{endpointID: "ep-1"}
	secret := []byte("secret")
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, secret, cb)

	req := &protocol.Request{Method: "GET", URL: "http://example.com"}
	rule := &domain.RoutingRule{ID: "rule-1"}

	_, err := p.Publish(context.Background(), req, rule, "", "", "res-q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.ID == "" {
		t.Error("expected request ID to be generated")
	}
}

func TestPublisher_Publish_SignedTask(t *testing.T) {
	mb := &mockBroker{}
	ms := &mockSelector{endpointID: "ep-1"}
	secret := []byte("test-secret-key")
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, secret, cb)

	req := &protocol.Request{ID: "req-123", Method: "GET", URL: "http://example.com"}
	rule := &domain.RoutingRule{ID: "rule-1"}

	_, err := p.Publish(context.Background(), req, rule, "", "", "res-q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mb.mu.Lock()
	defer mb.mu.Unlock()

	if len(mb.publishedMsgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(mb.publishedMsgs))
	}

	var signedTask protocol.SignedTask
	err = json.Unmarshal(mb.publishedMsgs[0].Body, &signedTask)
	if err != nil {
		t.Fatalf("failed to unmarshal signed task: %v", err)
	}

	_, err = protocol.ValidateSignedTask(&signedTask, secret, 5*time.Second)
	if err != nil {
		t.Errorf("signed task validation failed: %v", err)
	}

	if len(signedTask.Payload) == 0 {
		t.Error("expected signed task payload to be non-empty")
	}

	if signedTask.Timestamp == 0 {
		t.Error("expected signed task timestamp to be set")
	}
}

func TestPublisher_Publish_NoResultHandler(t *testing.T) {
	mb := &mockBroker{}
	ms := &mockSelector{endpointID: "ep-1"}
	secret := []byte("secret")
	cb := circuitbreaker.New(circuitbreaker.Config{Name: "test-publisher"})
	p := NewPublisher(mb, ms, secret, cb)

	req := &protocol.Request{ID: "req-1", Method: "GET", URL: "http://example.com"}
	rule := &domain.RoutingRule{ID: "rule-1"}

	eid, err := p.Publish(context.Background(), req, rule, "", "", "res-q-nofunc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if eid != "ep-1" {
		t.Errorf("expected endpointID 'ep-1', got %s", eid)
	}

	mb.mu.Lock()
	defer mb.mu.Unlock()
	if len(mb.subscriptions) != 0 {
		t.Errorf("expected no subscriptions, got %d", len(mb.subscriptions))
	}
}
