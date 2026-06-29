package endpoint

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

const testWorkerID = "test-worker-1"

type workerMockBroker struct {
	mu            sync.Mutex
	subs          map[string]broker.Handler
	publishedMsgs []publishedMsg
}

func newWorkerMockBroker() *workerMockBroker {
	return &workerMockBroker{
		subs: make(map[string]broker.Handler),
	}
}

func (m *workerMockBroker) Publish(_ context.Context, subject string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, publishedMsg{
		Subject: subject,
		Body:    body,
	})

	return nil
}

func (m *workerMockBroker) Subscribe(_ context.Context, subject string, handler broker.Handler, _ ...broker.SubscribeOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs[subject] = handler

	return nil
}

func (m *workerMockBroker) ConsumeOnce(_ context.Context, _ string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *workerMockBroker) DeclareStream(_ context.Context, _ string, _ ...string) error {
	return nil
}

func (m *workerMockBroker) IsConnected() bool {
	return true
}

func (m *workerMockBroker) Close() error {
	return nil
}

type dummyExecutor struct{}

func (d *dummyExecutor) Do(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	return &protocol.Response{
		RequestID:  req.ID,
		StatusCode: 200,
	}, nil
}

func TestWorker_HandleControlCommands(t *testing.T) {
	cfg := &config.EndpointConfig{
		ID:            testWorkerID,
		Security:      config.SecurityConfig{HMACSecret: "test-secret"},
		Observability: config.ObservabilityConfig{MetricsPort: 9099},
	}
	w := NewWorker(cfg)
	mb := newWorkerMockBroker()
	consumer := NewConsumer(mb, &dummyExecutor{}, []byte("test-secret"), testWorkerID)

	ctx := t.Context()

	w.handleControlCommands(ctx, mb, consumer, setupWorkerLogger(cfg))

	// Get registered handler
	mb.mu.Lock()
	handler := mb.subs["endpoint.control."+testWorkerID]
	mb.mu.Unlock()

	if handler == nil {
		t.Fatal("control command subscription handler not registered")
	}

	// Send a drain command
	cmd := protocol.ControlCommand{
		CommandID:  "cmd-drain",
		EndpointID: testWorkerID,
		Command:    "drain",
		IssuedAt:   time.Now(),
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}

	err = handler(ctx, cmdBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for command processing to reflect in published messages
	time.Sleep(100 * time.Millisecond)

	mb.mu.Lock()
	msgs := mb.publishedMsgs
	mb.mu.Unlock()

	// Verify we got the acknowledgements
	var ack1 protocol.CommandAck
	var gotAck, gotRunning, gotSucceeded bool

	for _, msg := range msgs {
		if msg.Subject == "endpoint.control.ack.cmd-drain" {
			var ack protocol.CommandAck
			_ = json.Unmarshal(msg.Body, &ack)
			switch ack.Status {
			case "acknowledged":
				gotAck = true
				ack1 = ack
			case "running":
				gotRunning = true
			case "succeeded":
				gotSucceeded = true
			}
		}
	}

	if !gotAck {
		t.Error("expected acknowledged ack status message")
	}
	if !gotRunning {
		t.Error("expected running ack status message")
	}
	if !gotSucceeded {
		t.Error("expected succeeded ack status message")
	}

	if ack1.CommandID != "cmd-drain" || ack1.EndpointID != testWorkerID {
		t.Errorf("ack properties incorrect: %+v", ack1)
	}
}
