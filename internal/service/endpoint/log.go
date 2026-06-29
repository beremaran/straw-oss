package endpoint

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

type LogService struct {
	broker      broker.MessageBroker
	logRepo     domain.EndpointLogRepository
	logger      *slog.Logger
	mu          sync.Mutex
	running     bool
	cancel      context.CancelFunc
	done        chan struct{}
}

func NewLogService(b broker.MessageBroker, repo domain.EndpointLogRepository, logger *slog.Logger) *LogService {
	if logger == nil {
		logger = slog.Default()
	}

	return &LogService{
		broker:  b,
		logRepo: repo,
		logger:  logger.WithGroup("log_service"),
	}
}

func (s *LogService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	s.running = true

	s.logger.Info("starting log service")

	go s.run(ctx)

	return nil
}

func (s *LogService) Stop() {
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

	s.logger.Info("log service stopped")
}

func (s *LogService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running
}

func (s *LogService) run(ctx context.Context) {
	defer close(s.done)

	err := s.broker.Subscribe(ctx, "endpoint.logs.>", s.handleLog, broker.WithTransient())
	if err != nil {
		s.logger.Error("failed to subscribe to endpoint logs", "error", err)

		return
	}

	// Ticker for retention cleanup (runs every 1 hour)
	// ponytail: hourly cleanup task to prevent DB growth
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run initial cleanup immediately
	s.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCleanup(ctx)
		}
	}
}

func (s *LogService) handleLog(ctx context.Context, body []byte) error {
	var entry protocol.LogEntry
	err := json.Unmarshal(body, &entry)
	if err != nil {
		s.logger.Error("failed to unmarshal log entry", "error", err)

		return nil
	}

	domainEntry := &domain.EndpointLogEntry{
		EndpointID: entry.EndpointID,
		ObservedAt: entry.ObservedAt,
		Level:      entry.Level,
		Message:    entry.Message,
		Attrs:      entry.Attrs,
	}

	if entry.TraceID != "" {
		domainEntry.TraceID = &entry.TraceID
	}
	if entry.RequestID != "" {
		domainEntry.RequestID = &entry.RequestID
	}

	err = s.logRepo.Create(ctx, domainEntry)
	if err != nil {
		s.logger.Error("failed to create log entry in repository", "endpoint_id", entry.EndpointID, "error", err)

		return err
	}

	return nil
}

func (s *LogService) runCleanup(ctx context.Context) {
	// Retention limits: 7 days or 5 GB
	maxAge := 7 * 24 * time.Hour
	maxSizeBytes := int64(5 * 1024 * 1024 * 1024)

	s.logger.Info("running logs retention cleanup")
	err := s.logRepo.Cleanup(ctx, maxAge, maxSizeBytes)
	if err != nil {
		s.logger.Error("failed to cleanup log entries", "error", err)
	} else {
		s.logger.Info("logs retention cleanup completed successfully")
	}
}
