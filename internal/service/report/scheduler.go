// Package report provides saved-report scheduling services.
package report

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/domain"
)

const dueClaimLimit = 10

// Runner runs a saved report for a schedule.
type Runner interface {
	RunScheduledReport(ctx context.Context, reportID string, scheduleID string) (*domain.ReportRun, error)
}

// Scheduler claims due report schedules and runs them.
type Scheduler struct {
	repo     domain.ReportScheduleRepository
	runner   Runner
	interval time.Duration
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	running  bool
}

// NewScheduler creates a report scheduler.
func NewScheduler(repo domain.ReportScheduleRepository, runner Runner, interval time.Duration) *Scheduler {
	return &Scheduler{repo: repo, runner: runner, interval: interval}
}

// Start starts the scheduler loop.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	if s.repo == nil || s.runner == nil || s.interval <= 0 {
		return nil
	}

	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	s.running = true

	go s.run(ctx)

	return nil
}

// Stop stops the scheduler loop.
func (s *Scheduler) Stop() {
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
}

// RunDueOnce claims and runs due schedules once.
func (s *Scheduler) RunDueOnce(ctx context.Context) error {
	schedules, err := s.repo.ClaimDue(ctx, time.Now().UTC(), dueClaimLimit)
	if err != nil {
		return fmt.Errorf("claim due report schedules: %w", err)
	}

	for _, schedule := range schedules {
		_, _ = s.runner.RunScheduledReport(ctx, schedule.ReportID, schedule.ID)

		now := time.Now().UTC()
		nextRunAt := nextScheduleRun(now, schedule.Cron)

		err = s.repo.MarkRun(ctx, schedule.ID, now, nextRunAt)
		if err != nil {
			return fmt.Errorf("mark report schedule run: %w", err)
		}
	}

	return nil
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.RunDueOnce(ctx)
		}
	}
}

func nextScheduleRun(now time.Time, _ string) time.Time {
	// ponytail: hourly-ish fallback; handler validation owns cron shape, exact cron math can replace this later.
	return now.Add(time.Hour)
}
