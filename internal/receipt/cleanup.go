// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/beremaran/straw-oss/internal/objectstore"
)

// Cleanup expires stale records and deletes their payloads.
func (s *Service) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	objects, err := s.store.List(ctx, "deployments/")
	if err != nil {
		return fmt.Errorf("list receipt records: %w", err)
	}

	for _, object := range objects {
		cleanupObject(ctx, s, object)
	}

	return nil
}

func cleanupObject(ctx context.Context, s *Service, object objectstore.Object) {
	if !strings.HasSuffix(object.Key, "/record.json") {
		return
	}

	record, ok := readCleanupRecord(ctx, s, object.Key)
	if !ok {
		return
	}

	if s.expired(record) && record.State != StateExpired {
		record.State = StateExpired

		s.expiredCount.Add(1)
		_ = s.deletePayload(ctx, record)
		_ = s.save(ctx, record)

		return
	}

	if record.State == StateAssigned && !s.cfg.Now().Before(record.AssignmentExpiresAt) {
		record.State = StateVerified
		record.AssignedRequestID = ""
		record.AssignmentExpiresAt = time.Time{}
		record.UpdatedAt = s.cfg.Now().UTC()
		_ = s.save(ctx, record)
	}
}

func readCleanupRecord(ctx context.Context, s *Service, key string) (Record, bool) {
	reader, _, err := s.store.Open(ctx, key)
	if err != nil {
		return Record{}, false
	}
	defer func() { _ = reader.Close() }()

	record, err := decodeRecord(reader)
	if err != nil {
		return Record{}, false
	}

	record.DeploymentID = deploymentFromRecordKey(key)

	return record, record.DeploymentID != ""
}

// RunCleanup periodically performs receipt retention cleanup.
func (s *Service) RunCleanup(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.Cleanup(ctx)
		}
	}
}
