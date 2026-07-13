// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Create creates or idempotently returns an uploading receipt.
func (s *Service) Create(ctx context.Context, deploymentID string, in CreateInput) (Record, error) {
	err := validateCreateInput(s, deploymentID, in)
	if err != nil {
		return Record{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, found, err := resolveCreateIdempotency(ctx, s, deploymentID, in)
	if err != nil {
		return Record{}, err
	}

	if found {
		return existing, nil
	}

	id, err := newID()
	if err != nil {
		return Record{}, err
	}

	now := s.cfg.Now().UTC()

	record := Record{ID: id, DeploymentID: deploymentID, Direction: in.Direction, State: StateUploading, SizeBytes: in.SizeBytes, SHA256Hex: strings.ToLower(in.SHA256Hex), CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(s.cfg.Retention), IdempotencyKey: in.IdempotencyKey}

	err = s.save(ctx, record)
	if err != nil {
		return Record{}, err
	}

	if in.IdempotencyKey != "" {
		err = s.saveIdempotency(ctx, record)
		if err != nil {
			return Record{}, err
		}
	}

	s.created.Add(1)

	return record, nil
}

func validateCreateInput(s *Service, deploymentID string, in CreateInput) error {
	if deploymentID == "" {
		return ErrInvalid
	}

	if in.Direction != DirectionRequest && in.Direction != DirectionResponse {
		return ErrInvalid
	}

	if in.SizeBytes < 0 || in.SizeBytes > s.cfg.MaxObjectBytes {
		return ErrInvalid
	}

	if !validSHA256(in.SHA256Hex) {
		return ErrInvalid
	}

	return nil
}

func resolveCreateIdempotency(ctx context.Context, s *Service, deploymentID string, in CreateInput) (Record, bool, error) {
	if in.IdempotencyKey == "" {
		return Record{}, false, nil
	}

	existing, found, err := s.findIdempotentReceipt(ctx, deploymentID, in.IdempotencyKey)
	if err != nil || !found {
		return existing, found, err
	}

	if existing.Direction != in.Direction || existing.SizeBytes != in.SizeBytes {
		return Record{}, false, ErrConflict
	}

	if !strings.EqualFold(existing.SHA256Hex, in.SHA256Hex) {
		return Record{}, false, ErrConflict
	}

	return existing, true, nil
}

// PutPart uploads or replaces one numbered receipt part.
func (s *Service) PutPart(ctx context.Context, deploymentID, id string, number int, body io.Reader, size int64, checksum string) (Record, error) {
	err := validatePartInput(s, id, number, size, checksum)
	if err != nil {
		return Record{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.load(ctx, deploymentID, id)
	if err != nil {
		return Record{}, err
	}

	if record.State != StateUploading || s.expired(record) {
		return Record{}, ErrConflict
	}

	hash := sha256.New()

	key := partKey(deploymentID, id, number)

	err = s.store.Put(ctx, key, io.TeeReader(body, hash), size, nil)
	if err != nil {
		return Record{}, fmt.Errorf("store receipt part: %w", err)
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	if checksum != "" && !strings.EqualFold(checksum, actual) {
		_ = s.store.Delete(ctx, key)

		return Record{}, fmt.Errorf("%w: part checksum mismatch", ErrInvalid)
	}

	upsertPart(&record, Part{Number: number, SizeBytes: size, SHA256Hex: actual})

	sort.Slice(record.Parts, func(i, j int) bool { return record.Parts[i].Number < record.Parts[j].Number })
	record.UpdatedAt = s.cfg.Now().UTC()
	s.parts.Add(1)

	return record, s.save(ctx, record)
}

func validatePartInput(s *Service, id string, number int, size int64, checksum string) error {
	if !validID(id) || number < 1 {
		return ErrInvalid
	}

	if size < 0 || size > s.cfg.MaxPartBytes {
		return ErrInvalid
	}

	if checksum != "" && !validSHA256(checksum) {
		return ErrInvalid
	}

	return nil
}

func upsertPart(record *Record, part Part) {
	for i := range record.Parts {
		if record.Parts[i].Number == part.Number {
			record.Parts[i] = part

			return
		}
	}

	record.Parts = append(record.Parts, part)
}

// Complete composes and verifies all contiguous upload parts.
