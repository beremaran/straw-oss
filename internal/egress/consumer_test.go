package egress

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/broker"
	egresshttp "github.com/beremaran/straw/internal/egress/http"
	"github.com/beremaran/straw/internal/protocol"
)

type mockBroker struct {
	subscribeQueue   string
	subscribeHandler broker.Handler
	publishedMsgs    []publishedMsg
	mu               sync.Mutex
}

type publishedMsg struct {
	Subject string
	Body    []byte
}

func (m *mockBroker) Publish(_ context.Context, subject string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, publishedMsg{
		Subject: subject,
		Body:    body,
	})

	return nil
}

func (m *mockBroker) Subscribe(ctx context.Context, subject string, handler broker.Handler, _ int) error {
	m.subscribeQueue = subject
	m.subscribeHandler = handler

	<-ctx.Done()

	return nil
}

func (m *mockBroker) ConsumeOnce(_ context.Context, _ string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockBroker) Close() error {
	return nil
}

func (m *mockBroker) DeclareStream(_ context.Context, _ string, _ ...string) error {
	return nil
}

func (m *mockBroker) IsConnected() bool { return true }

const (
	testRequestID = "test-req-1"
	testMethod    = "GET"
)

func TestConsumer_New(t *testing.T) {
	mb := &mockBroker{}
	httpClient := egresshttp.NewClient("")

	c := NewConsumer(mb, httpClient, "test-egress", 0, nil)

	if c.egressID != "test-egress" {
		t.Errorf("expected egressID 'test-egress', got %s", c.egressID)
	}

	if c.taskSubject != "tasks.test-egress.tasks" {
		t.Errorf("expected taskSubject 'tasks.test-egress.tasks', got %s", c.taskSubject)
	}

	if c.concurrencyLimit != DefaultConcurrencyLimit {
		t.Errorf("expected concurrencyLimit %d, got %d", DefaultConcurrencyLimit, c.concurrencyLimit)
	}
}

func TestConsumer_WithConcurrencyLimit(t *testing.T) {
	mb := &mockBroker{}
	httpClient := egresshttp.NewClient("")

	c := NewConsumer(mb, httpClient, "test-egress", 50, nil)

	if c.concurrencyLimit != 50 {
		t.Errorf("expected concurrencyLimit 50, got %d", c.concurrencyLimit)
	}
}

func TestConsumer_TaskSubject(t *testing.T) {
	mb := &mockBroker{}
	httpClient := egresshttp.NewClient("")

	c := NewConsumer(mb, httpClient, "my-egress", 0, nil)

	expected := "tasks.my-egress.tasks"
	if c.TaskSubject() != expected {
		t.Errorf("expected TaskSubject %q, got %q", expected, c.TaskSubject())
	}
}

func TestConsumer_InvalidProtobuf(t *testing.T) {
	mb := &mockBroker{}
	httpClient := egresshttp.NewClient("")

	c := NewConsumer(mb, httpClient, "test-egress", 0, nil)
	c.ctx = context.Background()

	err := c.processTask(context.Background(), []byte{0xff, 0xff, 0xff})
	if err == nil {
		t.Fatal("expected error for invalid protobuf, got nil")
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

func TestConsumer_ConcurrencyLimit(t *testing.T) {
	mb := &mockBroker{}
	httpClient := egresshttp.NewClient("")

	c := NewConsumer(mb, httpClient, "test-egress", 2, nil)

	var currentConcurrent atomic.Int32
	var maxConcurrent int32
	var wg sync.WaitGroup

	ctx := t.Context()

	go func() {
		_ = c.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	for range 5 {
		wg.Go(func() {
			c.semaphore <- struct{}{}
			current := currentConcurrent.Add(1)

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

			currentConcurrent.Add(-1)
			<-c.semaphore
		})
	}

	wg.Wait()

	if maxConcurrent > 2 {
		t.Errorf("expected max concurrent <= 2, got %d", maxConcurrent)
	}
}

func TestConsumer_ResultHandler(t *testing.T) {
	mb := &mockBroker{}
	httpClient := egresshttp.NewClient("")

	var receivedResponse *protocol.Response
	handler := func(_ context.Context, resp *protocol.Response, _ string) error {
		receivedResponse = resp

		return nil
	}

	c := NewConsumer(mb, httpClient, "test-egress", 0, handler)
	c.ctx = context.Background()

	req := &protocol.Request{
		ID:     testRequestID,
		Method: testMethod,
		URL:    "https://httpbin.org/get",
	}

	body, err := protocol.MarshalRequest(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = c.processTask(ctx, body)

	if receivedResponse == nil {
		t.Fatal("expected result handler to be called")
	}

	if receivedResponse.RequestID != testRequestID {
		t.Errorf("expected request ID %q, got %s", testRequestID, receivedResponse.RequestID)
	}

	if receivedResponse.EgressID != "test-egress" {
		t.Errorf("expected egress ID 'test-egress', got %s", receivedResponse.EgressID)
	}
}
