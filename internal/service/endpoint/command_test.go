package endpoint

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

type commandMockBroker struct {
	mu        sync.Mutex
	subs      map[string]broker.Handler
	published map[string][][]byte
}

func newCommandMockBroker() *commandMockBroker {
	return &commandMockBroker{
		subs:      make(map[string]broker.Handler),
		published: make(map[string][][]byte),
	}
}

func (m *commandMockBroker) Publish(ctx context.Context, subject string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published[subject] = append(m.published[subject], body)

	return nil
}

func (m *commandMockBroker) Subscribe(ctx context.Context, subject string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs[subject] = handler

	return nil
}

func (m *commandMockBroker) ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *commandMockBroker) DeclareStream(ctx context.Context, name string, subjects ...string) error {
	return nil
}

func (m *commandMockBroker) IsConnected() bool {
	return true
}

func (m *commandMockBroker) Close() error {
	return nil
}

type mockCommandRepo struct {
	mu       sync.Mutex
	commands map[string]*domain.EndpointCommand
}

func newMockCommandRepo() *mockCommandRepo {
	return &mockCommandRepo{
		commands: make(map[string]*domain.EndpointCommand),
	}
}

func (m *mockCommandRepo) Create(ctx context.Context, cmd *domain.EndpointCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[cmd.ID] = cmd

	return nil
}

func (m *mockCommandRepo) GetByID(ctx context.Context, id string) (*domain.EndpointCommand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.commands[id]; ok {
		return c, nil
	}

	return nil, nil
}

func (m *mockCommandRepo) Update(ctx context.Context, cmd *domain.EndpointCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[cmd.ID] = cmd

	return nil
}

func (m *mockCommandRepo) ListByEndpointID(ctx context.Context, endpointID string, limit, offset int) ([]domain.EndpointCommand, int, error) {
	return nil, 0, nil
}

func (m *mockCommandRepo) ListPending(ctx context.Context, before time.Time) ([]domain.EndpointCommand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []domain.EndpointCommand
	for _, cmd := range m.commands {
		if (cmd.Status == domain.CommandStatusAccepted ||
			cmd.Status == domain.CommandStatusAcknowledged ||
			cmd.Status == domain.CommandStatusRunning) && cmd.RequestedAt.Before(before) {
			list = append(list, *cmd)
		}
	}

	return list, nil
}

func TestCommandService_HandleAck(t *testing.T) {
	b := newCommandMockBroker()
	repo := newMockCommandRepo()
	srv := NewCommandService(b, repo, nil)

	ctx := context.Background()
	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	// Initial command
	cmd := &domain.EndpointCommand{
		ID:          "cmd-1",
		EndpointID:  "ep-1",
		Command:     "restart",
		Status:      domain.CommandStatusAccepted,
		RequestedAt: time.Now().Add(-5 * time.Second),
	}
	_ = repo.Create(ctx, cmd)

	// Simulate NATS acknowledgement message
	ackMsg := protocol.CommandAck{
		CommandID:  "cmd-1",
		EndpointID: "ep-1",
		Status:     "acknowledged",
		Timestamp:  time.Now(),
	}
	ackBytes, err := json.Marshal(ackMsg)
	if err != nil {
		t.Fatal(err)
	}

	b.mu.Lock()
	handler := b.subs["endpoint.control.ack.>"]
	b.mu.Unlock()

	if handler == nil {
		t.Fatal("subscription handler for endpoint.control.ack.> not registered")
	}

	err = handler(ctx, ackBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Verify command status updated to acknowledged
	updatedCmd, err := repo.GetByID(ctx, "cmd-1")
	if err != nil {
		t.Fatal(err)
	}
	if updatedCmd.Status != domain.CommandStatusAcknowledged {
		t.Errorf("expected status %s, got %s", domain.CommandStatusAcknowledged, updatedCmd.Status)
	}
	if updatedCmd.AcceptedAt == nil {
		t.Error("expected AcceptedAt to be set")
	}
}

func TestCommandService_Timeouts(t *testing.T) {
	b := newCommandMockBroker()
	repo := newMockCommandRepo()
	srv := NewCommandService(b, repo, nil)

	ctx := context.Background()

	// Command 1: timed out (created 3 minutes ago)
	cmd1 := &domain.EndpointCommand{
		ID:          "cmd-1",
		EndpointID:  "ep-1",
		Command:     "drain",
		Status:      domain.CommandStatusAccepted,
		RequestedAt: time.Now().Add(-3 * time.Minute),
	}
	_ = repo.Create(ctx, cmd1)

	// Command 2: not timed out (created 10 seconds ago)
	cmd2 := &domain.EndpointCommand{
		ID:          "cmd-2",
		EndpointID:  "ep-1",
		Command:     "restart",
		Status:      domain.CommandStatusAccepted,
		RequestedAt: time.Now().Add(-10 * time.Second),
	}
	_ = repo.Create(ctx, cmd2)

	srv.checkTimeouts(ctx)

	// Verify command 1 is timed_out
	updatedCmd1, _ := repo.GetByID(ctx, "cmd-1")
	if updatedCmd1.Status != domain.CommandStatusTimedOut {
		t.Errorf("expected cmd-1 status to be %s, got %s", domain.CommandStatusTimedOut, updatedCmd1.Status)
	}
	if updatedCmd1.Error == nil || *updatedCmd1.Error != "command timed out" {
		t.Errorf("expected timeout error message, got %v", updatedCmd1.Error)
	}

	// Verify command 2 is still accepted
	updatedCmd2, _ := repo.GetByID(ctx, "cmd-2")
	if updatedCmd2.Status != domain.CommandStatusAccepted {
		t.Errorf("expected cmd-2 status to be %s, got %s", domain.CommandStatusAccepted, updatedCmd2.Status)
	}
}
