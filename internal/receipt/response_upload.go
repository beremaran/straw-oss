// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
)

// ResponseUpload stores response frames as bounded durable parts and composes
// them only after Egress sends a successful terminal frame.
type ResponseUpload struct {
	service          *Service
	ctx              context.Context
	deploymentID, id string
	parts            []Part
	buffer           []byte
	hash             hash.Hash
	size             int64
	closed           bool
}

// BeginResponse starts a bounded stored-response upload.
func (s *Service) BeginResponse(ctx context.Context, deploymentID string) (*ResponseUpload, error) {
	if deploymentID == "" {
		return nil, ErrInvalid
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}

	return &ResponseUpload{service: s, ctx: ctx, deploymentID: deploymentID, id: id, hash: sha256.New()}, nil
}

func (u *ResponseUpload) Write(data []byte) error {
	if u.closed {
		return ErrConflict
	}

	if u.size+int64(len(data)) > u.service.cfg.MaxObjectBytes {
		return ErrInvalid
	}

	_, _ = u.hash.Write(data)

	u.size += int64(len(data))
	for len(data) > 0 {
		space := u.service.cfg.MaxPartBytes - int64(len(u.buffer))

		n := len(data)
		if int64(n) > space {
			n = int(space)
		}

		u.buffer = append(u.buffer, data[:n]...)
		data = data[n:]

		if int64(len(u.buffer)) == u.service.cfg.MaxPartBytes {
			err := u.flushPart(u.ctx)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Commit composes all response parts and publishes the verified record.
func (u *ResponseUpload) Commit(ctx context.Context) (Record, error) {
	if u.closed {
		return Record{}, ErrConflict
	}

	err := u.flushPart(ctx)
	if err != nil {
		return Record{}, err
	}

	u.closed = true
	u.service.mu.Lock()
	defer u.service.mu.Unlock()

	now := u.service.cfg.Now().UTC()
	checksum := hex.EncodeToString(u.hash.Sum(nil))
	reader := &partReader{ctx: ctx, store: u.service.store, keys: partKeys(u.deploymentID, u.id, u.parts)}
	err = u.service.store.Put(ctx, bodyKey(u.deploymentID, u.id), reader, u.size, map[string]string{"sha256": checksum})
	_ = reader.Close()

	if err != nil {
		u.deleteParts(ctx)

		return Record{}, fmt.Errorf("compose response receipt: %w", err)
	}

	u.deleteParts(ctx)

	record := Record{ID: u.id, DeploymentID: u.deploymentID, Direction: DirectionResponse, State: StateVerified, SizeBytes: u.size, SHA256Hex: checksum, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(u.service.cfg.Retention)}

	err = u.service.save(ctx, record)
	if err != nil {
		return Record{}, err
	}

	u.service.created.Add(1)
	u.service.verified.Add(1)

	return record, nil
}

// Abort deletes durable response parts without publishing a receipt.
func (u *ResponseUpload) Abort(ctx context.Context) {
	if u.closed {
		return
	}

	u.closed = true
	u.deleteParts(ctx)
}

func (u *ResponseUpload) deleteParts(ctx context.Context) {
	for _, part := range u.parts {
		_ = u.service.store.Delete(ctx, partKey(u.deploymentID, u.id, part.Number))
	}
}

func (u *ResponseUpload) flushPart(ctx context.Context) error {
	if len(u.buffer) == 0 {
		return nil
	}

	number := len(u.parts) + 1
	sum := sha256.Sum256(u.buffer)

	err := u.service.store.Put(ctx, partKey(u.deploymentID, u.id, number), bytes.NewReader(u.buffer), int64(len(u.buffer)), nil)
	if err != nil {
		return fmt.Errorf("store response receipt part: %w", err)
	}

	u.parts = append(u.parts, Part{Number: number, SizeBytes: int64(len(u.buffer)), SHA256Hex: hex.EncodeToString(sum[:])})
	u.service.parts.Add(1)
	u.buffer = u.buffer[:0]

	return nil
}

// New creates a receipt service over store.
