// Package endpoint provides services for managing endpoint commands, health, and logs.
package endpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

const commandTickerInterval = 10 * time.Second

// CommandService manages the lifecycle of endpoint commands received via the message broker.
type CommandService struct {
	broker      broker.MessageBroker
	commandRepo domain.EndpointCommandRepository
	logger      *slog.Logger
	mu          sync.Mutex
	running     bool
	cancel      context.CancelFunc
	done        chan struct{}
}

// NewCommandService creates a new CommandService with the given broker, repository, and logger.
func NewCommandService(b broker.MessageBroker, repo domain.EndpointCommandRepository, logger *slog.Logger) *CommandService {
	if logger == nil {
		logger = slog.Default()
	}

	return &CommandService{
		broker:      b,
		commandRepo: repo,
		logger:      logger.WithGroup("command_service"),
	}
}

// Start begins processing commands and acks. It is safe to call multiple times.
func (s *CommandService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	s.running = true

	s.logger.Info("starting command service")

	go s.run(ctx)

	return nil
}

// Stop signals the service to stop and waits for goroutines to exit.
func (s *CommandService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()

		return
	}

	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	s.logger.Info("command service stopped")
}

// IsRunning reports whether the service is currently running.
func (s *CommandService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running
}

func (s *CommandService) run(ctx context.Context) {
	defer close(s.done)

	err := s.broker.Subscribe(ctx, "endpoint.control.ack.>", s.handleAck, broker.WithTransient())
	if err != nil {
		s.logger.Error("failed to subscribe to command acks", "error", err)

		return
	}

	ticker := time.NewTicker(commandTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkTimeouts(ctx)
		}
	}
}

func (s *CommandService) handleAck(ctx context.Context, body []byte) error {
	var ack protocol.CommandAck

	err := json.Unmarshal(body, &ack)
	if err != nil {
		s.logger.Error("failed to unmarshal command ack", "error", err)

		return nil
	}

	if ack.CommandID == "" {
		s.logger.Warn("received command ack with empty command_id")

		return nil
	}

	cmd, err := s.commandRepo.GetByID(ctx, ack.CommandID)
	if err != nil {
		s.logger.Error("failed to retrieve command", "command_id", ack.CommandID, "error", err)

		return fmt.Errorf("get command by id: %w", err)
	}

	if cmd == nil {
		s.logger.Warn("received ack for unknown command", "command_id", ack.CommandID)

		return nil
	}

	if isTerminalState(cmd.Status) {
		return nil
	}

	now := time.Now().UTC()

	ts := ack.Timestamp
	if ts.IsZero() {
		ts = now
	}

	if !s.applyAckStatus(cmd, &ack, ts) {
		return nil
	}

	err = s.commandRepo.Update(ctx, cmd)
	if err != nil {
		s.logger.Error("failed to update command status", "command_id", ack.CommandID, "error", err)

		return fmt.Errorf("update command: %w", err)
	}

	s.logger.Info("updated command status", "command_id", ack.CommandID, "status", cmd.Status)

	return nil
}

func isTerminalState(status domain.CommandStatus) bool {
	return status == domain.CommandStatusSucceeded ||
		status == domain.CommandStatusFailed ||
		status == domain.CommandStatusTimedOut
}

func (s *CommandService) applyAckStatus(cmd *domain.EndpointCommand, ack *protocol.CommandAck, ts time.Time) bool {
	switch ack.Status {
	case string(domain.CommandStatusAcknowledged):
		cmd.Status = domain.CommandStatusAcknowledged
		cmd.AcceptedAt = &ts
	case string(domain.CommandStatusRunning):
		cmd.Status = domain.CommandStatusRunning
	case string(domain.CommandStatusSucceeded):
		cmd.Status = domain.CommandStatusSucceeded
		cmd.CompletedAt = &ts
	case string(domain.CommandStatusFailed):
		cmd.Status = domain.CommandStatusFailed

		cmd.CompletedAt = &ts
		if ack.Message != "" {
			cmd.Error = &ack.Message
		} else {
			defaultErr := "command failed"
			cmd.Error = &defaultErr
		}
	default:
		s.logger.Warn("unknown command ack status", "status", ack.Status, "command_id", ack.CommandID)

		return false
	}

	return true
}

func (s *CommandService) checkTimeouts(ctx context.Context) {
	// ponytail: 2-minute static threshold for command timeout before transitioning to timed_out state.
	cutoff := time.Now().UTC().Add(-2 * time.Minute)

	cmds, err := s.commandRepo.ListPending(ctx, cutoff)
	if err != nil {
		s.logger.Error("failed to list pending commands for timeout check", "error", err)

		return
	}

	for _, cmd := range cmds {
		now := time.Now().UTC()
		cmd.Status = domain.CommandStatusTimedOut
		cmd.CompletedAt = &now
		errMsg := "command timed out"
		cmd.Error = &errMsg

		err = s.commandRepo.Update(ctx, &cmd)
		if err != nil {
			s.logger.Error("failed to mark command as timed out", "command_id", cmd.ID, "error", err)

			continue
		}

		s.logger.Info("command marked as timed out", "command_id", cmd.ID)
	}
}
