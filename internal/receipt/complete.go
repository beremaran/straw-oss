// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// Complete composes and verifies all uploaded parts.
func (s *Service) Complete(ctx context.Context, deploymentID, id string) (Record, error) {
	if !validID(id) {
		return Record{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.load(ctx, deploymentID, id)
	if err != nil {
		return Record{}, err
	}

	if record.State == StateVerified {
		return record, nil
	}

	total, err := completionSize(s, record)
	if err != nil {
		return Record{}, err
	}

	if total != record.SizeBytes {
		return rejectReceipt(ctx, s, record, "size_mismatch", "receipt size mismatch")
	}

	record.State = StateVerifying

	record.UpdatedAt = s.cfg.Now().UTC()

	err = s.save(ctx, record)
	if err != nil {
		return Record{}, err
	}

	actual, err := composeParts(ctx, s, deploymentID, id, record)
	if err != nil {
		record.State = StateUploading
		record.Failure = "storage_error"
		_ = s.save(ctx, record)

		return record, fmt.Errorf("compose receipt body: %w", err)
	}

	if !strings.EqualFold(actual, record.SHA256Hex) {
		_ = s.store.Delete(ctx, bodyKey(deploymentID, id))

		return rejectReceipt(ctx, s, record, "checksum_mismatch", "receipt checksum mismatch")
	}

	return finalizeVerifiedReceipt(ctx, s, deploymentID, id, record)
}

func completionSize(s *Service, record Record) (int64, error) {
	if record.State != StateUploading && record.State != StateVerifying {
		return 0, ErrConflict
	}

	if len(record.Parts) == 0 || s.expired(record) {
		return 0, ErrConflict
	}

	var total int64

	for i, part := range record.Parts {
		if part.Number != i+1 {
			return 0, fmt.Errorf("%w: parts must be contiguous from 1", ErrInvalid)
		}

		total += part.SizeBytes
	}

	return total, nil
}

func composeParts(ctx context.Context, s *Service, deploymentID, id string, record Record) (string, error) {
	reader := &partReader{ctx: ctx, store: s.store, keys: partKeys(deploymentID, id, record.Parts)}
	defer func() { _ = reader.Close() }()

	hash := sha256.New()

	err := s.store.Put(ctx, bodyKey(deploymentID, id), io.TeeReader(reader, hash), record.SizeBytes, map[string]string{"sha256": record.SHA256Hex})
	if err != nil {
		return "", fmt.Errorf("compose receipt body: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func rejectReceipt(ctx context.Context, s *Service, record Record, failure, message string) (Record, error) {
	record.State = StateRejected
	record.Failure = failure
	record.UpdatedAt = s.cfg.Now().UTC()
	_ = s.save(ctx, record)
	s.rejected.Add(1)

	return record, fmt.Errorf("%w: %s", ErrInvalid, message)
}

func finalizeVerifiedReceipt(ctx context.Context, s *Service, deploymentID, id string, record Record) (Record, error) {
	for _, part := range record.Parts {
		_ = s.store.Delete(ctx, partKey(deploymentID, id, part.Number))
	}

	record.State = StateVerified
	record.Parts = nil
	record.Failure = ""
	record.UpdatedAt = s.cfg.Now().UTC()
	s.verified.Add(1)

	return record, s.save(ctx, record)
}

// Get returns durable receipt status.
