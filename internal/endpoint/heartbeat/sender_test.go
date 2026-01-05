package heartbeat

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
)

// mockBroker implements broker.MessageBroker for testing.
type mockBroker struct {
	publishedMsgs []publishedMsg
	mu            sync.Mutex
	publishErr    error
}

type publishedMsg struct {
	Exchange   string
	RoutingKey string
	Body       []byte
}

func (m *mockBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}
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
	return nil
}

func (m *mockBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
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

func (m *mockBroker) getMessages() []publishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishedMsgs
}

func (m *mockBroker) messageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.publishedMsgs)
}

func TestSender_New(t *testing.T) {
	mb := &mockBroker{}
	s := New(mb, "endpoint-001")

	if s.broker != mb {
		t.Error("expected broker to be set")
	}

	if s.endpointID != "endpoint-001" {
		t.Errorf("expected endpoint ID 'endpoint-001', got %q", s.endpointID)
	}

	if s.interval != DefaultInterval {
		t.Errorf("expected default interval %v, got %v", DefaultInterval, s.interval)
	}

	if s.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestSender_NewWithOptions(t *testing.T) {
	mb := &mockBroker{}
	customInterval := 5 * time.Second
	tags := []string{"type:residential", "region:us"}

	activeCount := 42
	s := New(mb, "endpoint-002",
		WithInterval(customInterval),
		WithVersion("1.2.3"),
		WithTags(tags),
		WithActiveTasksFunc(func() int { return activeCount }),
	)

	if s.interval != customInterval {
		t.Errorf("expected interval %v, got %v", customInterval, s.interval)
	}

	if s.version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %q", s.version)
	}

	if len(s.tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(s.tags))
	}

	if s.activeTasksFunc() != 42 {
		t.Errorf("expected activeTasksFunc to return 42, got %d", s.activeTasksFunc())
	}
}

func TestSender_HeartbeatMessageFormat(t *testing.T) {
	mb := &mockBroker{}
	tags := []string{"type:residential"}

	s := New(mb, "endpoint-003",
		WithVersion("2.0.0"),
		WithTags(tags),
		WithInterval(100*time.Millisecond),
		WithActiveTasksFunc(func() int { return 5 }),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for initial heartbeat
	time.Sleep(50 * time.Millisecond)

	s.Stop()

	msgs := mb.getMessages()
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 heartbeat message")
	}

	msg := msgs[0]

	// Check exchange and routing key
	if msg.Exchange != "heartbeats" {
		t.Errorf("expected exchange 'heartbeats', got %q", msg.Exchange)
	}

	if msg.RoutingKey != "endpoint-003" {
		t.Errorf("expected routing key 'endpoint-003', got %q", msg.RoutingKey)
	}

	// Unmarshal and verify message content
	var hb Message
	if err := json.Unmarshal(msg.Body, &hb); err != nil {
		t.Fatalf("failed to unmarshal heartbeat: %v", err)
	}

	if hb.EndpointID != "endpoint-003" {
		t.Errorf("expected endpoint ID 'endpoint-003', got %q", hb.EndpointID)
	}

	if hb.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", hb.Version)
	}

	if len(hb.Tags) != 1 || hb.Tags[0] != "type:residential" {
		t.Errorf("unexpected tags: %v", hb.Tags)
	}

	if hb.ActiveTasks != 5 {
		t.Errorf("expected active tasks 5, got %d", hb.ActiveTasks)
	}

	if hb.Timestamp <= 0 {
		t.Error("expected timestamp to be set")
	}
}

func TestSender_PeriodicSending(t *testing.T) {
	mb := &mockBroker{}

	s := New(mb, "endpoint-004",
		WithInterval(50*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for multiple heartbeats (initial + at least 2 more)
	time.Sleep(150 * time.Millisecond)

	s.Stop()

	msgs := mb.getMessages()
	// Should have at least 3 messages: initial + 2 from ticker
	if len(msgs) < 3 {
		t.Errorf("expected at least 3 heartbeats, got %d", len(msgs))
	}
}

func TestSender_GracefulShutdown(t *testing.T) {
	mb := &mockBroker{}

	s := New(mb, "endpoint-005",
		WithInterval(1*time.Second),
	)

	ctx := context.Background()

	s.Start(ctx)

	if !s.IsRunning() {
		t.Error("expected sender to be running after Start")
	}

	// Stop should block until the sender is fully stopped
	s.Stop()

	if s.IsRunning() {
		t.Error("expected sender to not be running after Stop")
	}

	// Count messages before second stop
	countBefore := mb.messageCount()

	// Double stop should be safe
	s.Stop()

	// Verify no more messages are sent after stop
	time.Sleep(50 * time.Millisecond)
	countAfter := mb.messageCount()

	if countAfter != countBefore {
		t.Error("expected no more heartbeats after stop")
	}
}

func TestSender_DoubleStartIsSafe(t *testing.T) {
	mb := &mockBroker{}

	s := New(mb, "endpoint-006",
		WithInterval(100*time.Millisecond),
	)

	ctx := context.Background()

	s.Start(ctx)
	s.Start(ctx) // Should be a no-op

	time.Sleep(50 * time.Millisecond)

	s.Stop()

	// Should still only have one initial heartbeat (not two)
	// and sender should be cleanly stopped
	if s.IsRunning() {
		t.Error("expected sender to not be running after Stop")
	}
}

func TestSender_ActiveTasksCallback(t *testing.T) {
	mb := &mockBroker{}

	var taskCount atomic.Int32
	taskCount.Store(10)

	s := New(mb, "endpoint-007",
		WithInterval(50*time.Millisecond),
		WithActiveTasksFunc(func() int { return int(taskCount.Load()) }),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for initial heartbeat
	time.Sleep(30 * time.Millisecond)

	// Change task count
	taskCount.Store(20)

	// Wait for another heartbeat
	time.Sleep(60 * time.Millisecond)

	s.Stop()

	msgs := mb.getMessages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 heartbeats, got %d", len(msgs))
	}

	// First heartbeat should have 10 active tasks
	var hb1 Message
	if err := json.Unmarshal(msgs[0].Body, &hb1); err != nil {
		t.Fatalf("failed to unmarshal first heartbeat: %v", err)
	}
	if hb1.ActiveTasks != 10 {
		t.Errorf("expected first heartbeat active tasks 10, got %d", hb1.ActiveTasks)
	}

	// Second heartbeat should have 20 active tasks
	var hb2 Message
	if err := json.Unmarshal(msgs[1].Body, &hb2); err != nil {
		t.Fatalf("failed to unmarshal second heartbeat: %v", err)
	}
	if hb2.ActiveTasks != 20 {
		t.Errorf("expected second heartbeat active tasks 20, got %d", hb2.ActiveTasks)
	}
}

func TestSender_ContextCancellation(t *testing.T) {
	mb := &mockBroker{}

	s := New(mb, "endpoint-008",
		WithInterval(1*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	s.Start(ctx)

	// Cancel context should stop the sender
	cancel()

	// Give it a moment to stop
	time.Sleep(50 * time.Millisecond)

	// Calling Stop after context cancel should be safe
	s.Stop()

	if s.IsRunning() {
		t.Error("expected sender to not be running after context cancel and Stop")
	}
}

func TestSender_PublishErrorDoesNotCrash(t *testing.T) {
	mb := &mockBroker{
		publishErr: context.DeadlineExceeded,
	}

	s := New(mb, "endpoint-009",
		WithInterval(50*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic even with publish errors
	s.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	s.Stop()

	// Sender should have stopped gracefully despite errors
	if s.IsRunning() {
		t.Error("expected sender to not be running after Stop")
	}
}
