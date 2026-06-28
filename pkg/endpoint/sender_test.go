package endpoint

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beremaran/straw/pkg/broker"
)

type mockHeartbeatBroker struct {
	publishedMsgs []publishedMsg
	mu            sync.Mutex
	publishErr    error
}

func (m *mockHeartbeatBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
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
func (m *mockHeartbeatBroker) Subscribe(ctx context.Context, queue string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	return nil
}

func (m *mockHeartbeatBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
	return nil
}

func (m *mockHeartbeatBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockHeartbeatBroker) Close() error {
	return nil
}

func (m *mockHeartbeatBroker) DeclareExchange(ctx context.Context, name, kind string) error {
	return nil
}

func (m *mockHeartbeatBroker) DeclareQueue(ctx context.Context, name string) error {
	return nil
}

func (m *mockHeartbeatBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	return nil
}

func (m *mockHeartbeatBroker) IsConnected() bool {
	return true
}

func (m *mockHeartbeatBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	return 0, nil
}

func (m *mockHeartbeatBroker) getMessages() []publishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.publishedMsgs
}

func (m *mockHeartbeatBroker) messageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.publishedMsgs)
}

func TestSender_New(t *testing.T) {
	mb := &mockHeartbeatBroker{}
	s := NewHeartbeatSender(mb, "endpoint-001")

	if s.broker != mb {
		t.Error("expected broker to be set")
	}

	if s.endpointID != "endpoint-001" {
		t.Errorf("expected endpoint ID 'endpoint-001', got %q", s.endpointID)
	}

	if s.interval != DefaultHeartbeatInterval {
		t.Errorf("expected default interval %v, got %v", DefaultHeartbeatInterval, s.interval)
	}

	if s.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestSender_NewWithOptions(t *testing.T) {
	mb := &mockHeartbeatBroker{}
	customInterval := 5 * time.Second
	tags := []string{"type:residential", "region:us"}

	activeCount := 42
	s := NewHeartbeatSender(mb, "endpoint-002",
		WithHeartbeatInterval(customInterval),
		WithHeartbeatVersion("1.2.3"),
		WithHeartbeatTags(tags),
		WithHeartbeatActiveTasksFunc(func() int { return activeCount }),
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
	mb := &mockHeartbeatBroker{}
	tags := []string{"type:residential"}

	s := NewHeartbeatSender(mb, "endpoint-003",
		WithHeartbeatVersion("2.0.0"),
		WithHeartbeatTags(tags),
		WithHeartbeatInterval(100*time.Millisecond),
		WithHeartbeatActiveTasksFunc(func() int { return 5 }),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	s.Stop()

	msgs := mb.getMessages()
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 heartbeat message")
	}

	msg := msgs[0]

	if msg.Exchange != "heartbeats" {
		t.Errorf("expected exchange 'heartbeats', got %q", msg.Exchange)
	}

	if msg.RoutingKey != "endpoint-003" {
		t.Errorf("expected routing key 'endpoint-003', got %q", msg.RoutingKey)
	}

	var hb HeartbeatMessage
	err := json.Unmarshal(msg.Body, &hb)
	if err != nil {
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
	mb := &mockHeartbeatBroker{}

	s := NewHeartbeatSender(mb, "endpoint-004",
		WithHeartbeatInterval(50*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(150 * time.Millisecond)

	s.Stop()

	msgs := mb.getMessages()

	if len(msgs) < 3 {
		t.Errorf("expected at least 3 heartbeats, got %d", len(msgs))
	}
}

func TestSender_GracefulShutdown(t *testing.T) {
	mb := &mockHeartbeatBroker{}

	s := NewHeartbeatSender(mb, "endpoint-005",
		WithHeartbeatInterval(1*time.Second),
	)

	ctx := context.Background()

	s.Start(ctx)

	if !s.IsRunning() {
		t.Error("expected sender to be running after Start")
	}

	s.Stop()

	if s.IsRunning() {
		t.Error("expected sender to not be running after Stop")
	}

	countBefore := mb.messageCount()

	s.Stop()

	time.Sleep(50 * time.Millisecond)
	countAfter := mb.messageCount()

	if countAfter != countBefore {
		t.Error("expected no more heartbeats after stop")
	}
}

func TestSender_DoubleStartIsSafe(t *testing.T) {
	mb := &mockHeartbeatBroker{}

	s := NewHeartbeatSender(mb, "endpoint-006",
		WithHeartbeatInterval(100*time.Millisecond),
	)

	ctx := context.Background()

	s.Start(ctx)
	s.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	s.Stop()

	if s.IsRunning() {
		t.Error("expected sender to not be running after Stop")
	}
}

func TestSender_ActiveTasksCallback(t *testing.T) {
	mb := &mockHeartbeatBroker{}

	var taskCount atomic.Int32
	taskCount.Store(10)

	s := NewHeartbeatSender(mb, "endpoint-007",
		WithHeartbeatInterval(50*time.Millisecond),
		WithHeartbeatActiveTasksFunc(func() int { return int(taskCount.Load()) }),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(30 * time.Millisecond)

	taskCount.Store(20)

	time.Sleep(60 * time.Millisecond)

	s.Stop()

	msgs := mb.getMessages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 heartbeats, got %d", len(msgs))
	}

	var hb1 HeartbeatMessage
	err := json.Unmarshal(msgs[0].Body, &hb1)
	if err != nil {
		t.Fatalf("failed to unmarshal first heartbeat: %v", err)
	}
	if hb1.ActiveTasks != 10 {
		t.Errorf("expected first heartbeat active tasks 10, got %d", hb1.ActiveTasks)
	}

	var hb2 HeartbeatMessage
	err = json.Unmarshal(msgs[1].Body, &hb2)
	if err != nil {
		t.Fatalf("failed to unmarshal second heartbeat: %v", err)
	}
	if hb2.ActiveTasks != 20 {
		t.Errorf("expected second heartbeat active tasks 20, got %d", hb2.ActiveTasks)
	}
}

func TestSender_ContextCancellation(t *testing.T) {
	mb := &mockHeartbeatBroker{}

	s := NewHeartbeatSender(mb, "endpoint-008",
		WithHeartbeatInterval(1*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	s.Start(ctx)

	cancel()

	time.Sleep(50 * time.Millisecond)

	s.Stop()

	if s.IsRunning() {
		t.Error("expected sender to not be running after context cancel and Stop")
	}
}

func TestSender_PublishErrorDoesNotCrash(t *testing.T) {
	mb := &mockHeartbeatBroker{
		publishErr: context.DeadlineExceeded,
	}

	s := NewHeartbeatSender(mb, "endpoint-009",
		WithHeartbeatInterval(50*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	s.Stop()

	if s.IsRunning() {
		t.Error("expected sender to not be running after Stop")
	}
}
