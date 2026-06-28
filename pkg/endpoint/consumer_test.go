package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	endpointhttp "github.com/beremaran/straw/internal/endpoint/http"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

type mockBroker struct {
	subscribeQueue   string
	subscribeHandler broker.Handler
	publishedMsgs    []publishedMsg
	mu               sync.Mutex
}

type publishedMsg struct {
	Exchange   string
	RoutingKey string
	Body       []byte
}

func (m *mockBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, publishedMsg{
		Exchange:   exchange,
		RoutingKey: routingKey,
		Body:       body,
	})
	return nil
}

func (m *mockBroker) Subscribe(ctx context.Context, queue string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	m.subscribeQueue = queue
	m.subscribeHandler = handler

	<-ctx.Done()
	return nil
}

func (m *mockBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
	m.subscribeQueue = queue
	m.subscribeHandler = handler

	<-ctx.Done()
	return nil
}

func (m *mockBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockBroker) Close() error {
	return nil
}

func (m *mockBroker) DeclareExchange(ctx context.Context, name, kind string) error {
	return nil
}

func (m *mockBroker) DeclareQueue(ctx context.Context, name string) error {
	return nil
}

func (m *mockBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	return nil
}

func (m *mockBroker) IsConnected() bool {
	return true
}

func (m *mockBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	return 0, nil
}

type mockTransportProvider struct {
	transport *fhttp.Transport
}

func (m *mockTransportProvider) GetTransport(host string, preset fingerprint.Preset) *fhttp.Transport {
	if m.transport != nil {
		return m.transport
	}
	return &fhttp.Transport{}
}

func TestConsumer_New(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	c := NewConsumer(mb, httpClient, secret, "test-endpoint")

	if c.endpointID != "test-endpoint" {
		t.Errorf("expected endpointID 'test-endpoint', got %s", c.endpointID)
	}

	if c.queueName != "endpoint.test-endpoint.tasks" {
		t.Errorf("expected queueName 'endpoint.test-endpoint.tasks', got %s", c.queueName)
	}

	if c.concurrencyLimit != DefaultConcurrencyLimit {
		t.Errorf("expected concurrencyLimit %d, got %d", DefaultConcurrencyLimit, c.concurrencyLimit)
	}

	if c.maxTaskAge != DefaultMaxTaskAge {
		t.Errorf("expected maxTaskAge %v, got %v", DefaultMaxTaskAge, c.maxTaskAge)
	}
}

func TestConsumer_WithOptions(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	c := NewConsumer(mb, httpClient, secret, "test-endpoint",
		WithConcurrencyLimit(50),
		WithMaxTaskAge(30*time.Second),
	)

	if c.concurrencyLimit != 50 {
		t.Errorf("expected concurrencyLimit 50, got %d", c.concurrencyLimit)
	}

	if c.maxTaskAge != 30*time.Second {
		t.Errorf("expected maxTaskAge 30s, got %v", c.maxTaskAge)
	}
}

func TestConsumer_QueueName(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	c := NewConsumer(mb, httpClient, secret, "my-endpoint")

	expected := "endpoint.my-endpoint.tasks"
	if c.QueueName() != expected {
		t.Errorf("expected QueueName %q, got %q", expected, c.QueueName())
	}
}

func TestConsumer_InvalidSignature(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	c := NewConsumer(mb, httpClient, secret, "test-endpoint")
	c.ctx = context.Background()

	req := &protocol.Request{
		ID:     "test-req-1",
		Method: "GET",
		URL:    "https://example.com",
	}

	wrongSecret := []byte("wrong-secret")
	signedTask, err := protocol.NewSignedTask(req, wrongSecret)
	if err != nil {
		t.Fatalf("failed to create signed task: %v", err)
	}

	body, err := json.Marshal(signedTask)
	if err != nil {
		t.Fatalf("failed to marshal signed task: %v", err)
	}

	err = c.processTask(context.Background(), body)
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}

	var taskErr *TaskError
	ok := errors.As(err, &taskErr)
	if !ok {
		t.Fatalf("expected TaskError, got %T", err)
	}

	if taskErr.Code != protocol.ErrCodeSignatureInvalid {
		t.Errorf("expected error code %q, got %q", protocol.ErrCodeSignatureInvalid, taskErr.Code)
	}
}

func TestConsumer_ReplayAttack(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	c := NewConsumer(mb, httpClient, secret, "test-endpoint",
		WithMaxTaskAge(1*time.Millisecond),
	)
	c.ctx = context.Background()

	req := &protocol.Request{
		ID:     "test-req-1",
		Method: "GET",
		URL:    "https://example.com",
	}

	signedTask, err := protocol.NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("failed to create signed task: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	body, err := json.Marshal(signedTask)
	if err != nil {
		t.Fatalf("failed to marshal signed task: %v", err)
	}

	err = c.processTask(context.Background(), body)
	if err == nil {
		t.Fatal("expected error for old timestamp, got nil")
	}

	var taskErr *TaskError
	ok := errors.As(err, &taskErr)
	if !ok {
		t.Fatalf("expected TaskError, got %T", err)
	}

	if taskErr.Code != protocol.ErrCodeReplayAttack {
		t.Errorf("expected error code %q, got %q", protocol.ErrCodeReplayAttack, taskErr.Code)
	}
}

func TestConsumer_ConcurrencyLimit(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	c := NewConsumer(mb, httpClient, secret, "test-endpoint",
		WithConcurrencyLimit(2),
	)

	var currentConcurrent int32
	var maxConcurrent int32
	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = c.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			c.semaphore <- struct{}{}
			current := atomic.AddInt32(&currentConcurrent, 1)

			for {
				theMaximum := atomic.LoadInt32(&maxConcurrent)
				if current <= theMaximum {
					break
				}
				if atomic.CompareAndSwapInt32(&maxConcurrent, theMaximum, current) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)

			atomic.AddInt32(&currentConcurrent, -1)
			<-c.semaphore
		}()
	}

	wg.Wait()

	if maxConcurrent > 2 {
		t.Errorf("expected max concurrent <= 2, got %d", maxConcurrent)
	}
}

func TestConsumer_InvalidJSON(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	c := NewConsumer(mb, httpClient, secret, "test-endpoint")
	c.ctx = context.Background()

	err := c.processTask(context.Background(), []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}

	var taskErr *TaskError
	ok := errors.As(err, &taskErr)
	if !ok {
		t.Fatalf("expected TaskError, got %T", err)
	}

	if taskErr.Code != protocol.ErrCodeInternalError {
		t.Errorf("expected error code %q, got %q", protocol.ErrCodeInternalError, taskErr.Code)
	}
}

func TestConsumer_ResultHandler(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	var receivedResponse *protocol.Response
	handler := func(ctx context.Context, resp *protocol.Response, replyTo string) error {
		receivedResponse = resp
		return nil
	}

	c := NewConsumer(mb, httpClient, secret, "test-endpoint",
		WithResultHandler(handler),
	)
	c.ctx = context.Background()

	req := &protocol.Request{
		ID:     "test-req-1",
		Method: "GET",
		URL:    "https://httpbin.org/get",
	}

	signedTask, err := protocol.NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("failed to create signed task: %v", err)
	}

	body, err := json.Marshal(signedTask)
	if err != nil {
		t.Fatalf("failed to marshal signed task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = c.processTask(ctx, body)

	if receivedResponse == nil {
		t.Fatal("expected result handler to be called")
	}

	if receivedResponse.RequestID != "test-req-1" {
		t.Errorf("expected request ID 'test-req-1', got %s", receivedResponse.RequestID)
	}

	if receivedResponse.EndpointID != "test-endpoint" {
		t.Errorf("expected endpoint ID 'test-endpoint', got %s", receivedResponse.EndpointID)
	}
}
