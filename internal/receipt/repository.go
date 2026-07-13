// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/beremaran/straw-oss/internal/objectstore"
)

// recordRepository isolates lifecycle policy from record and idempotency-key
// persistence. Payload composition continues through objectstore.Store.
type recordRepository interface {
	Save(ctx context.Context, record Record) error
	Load(ctx context.Context, deploymentID, id string) (Record, error)
	SaveIdempotency(ctx context.Context, record Record) error
	LoadIdempotency(ctx context.Context, deploymentID, key string) (Record, error)
}

type objectRecordRepository struct{ store objectstore.Store }

const maxReceiptRecordBytes = 1 << 20

func (s *Service) save(ctx context.Context, r Record) error {
	err := s.records.Save(ctx, r)
	if err == nil {
		return nil
	}

	return fmt.Errorf("save receipt repository record: %w", err)
}

func (r objectRecordRepository) Save(ctx context.Context, record Record) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode receipt record: %w", err)
	}

	err = r.store.Put(ctx, recordKey(record.DeploymentID, record.ID), strings.NewReader(string(raw)), int64(len(raw)), nil)
	if err != nil {
		return fmt.Errorf("store receipt record: %w", err)
	}

	return nil
}

func (s *Service) load(ctx context.Context, deploymentID, id string) (Record, error) {
	record, err := s.records.Load(ctx, deploymentID, id)
	if err != nil {
		return Record{}, fmt.Errorf("load receipt repository record: %w", err)
	}

	return record, nil
}

func (r objectRecordRepository) Load(ctx context.Context, deploymentID, id string) (Record, error) {
	reader, _, err := r.store.Open(ctx, recordKey(deploymentID, id))
	if errors.Is(err, objectstore.ErrNotFound) {
		return Record{}, ErrNotFound
	}

	if err != nil {
		return Record{}, fmt.Errorf("open receipt record: %w", err)
	}

	defer func() { _ = reader.Close() }()

	record, err := decodeRecord(reader)
	if err != nil {
		return Record{}, fmt.Errorf("decode receipt record: %w", err)
	}

	record.DeploymentID = deploymentID

	return record, nil
}

func decodeRecord(reader io.Reader) (Record, error) {
	var record Record

	err := json.NewDecoder(io.LimitReader(reader, maxReceiptRecordBytes)).Decode(&record)
	if err != nil {
		return Record{}, fmt.Errorf("decode bounded receipt record: %w", err)
	}

	return record, nil
}

func (s *Service) saveIdempotency(ctx context.Context, r Record) error {
	err := s.records.SaveIdempotency(ctx, r)
	if err == nil {
		return nil
	}

	return fmt.Errorf("save receipt repository index: %w", err)
}

func (r objectRecordRepository) SaveIdempotency(ctx context.Context, record Record) error {
	raw := []byte(record.ID)

	err := r.store.Put(ctx, idempotencyKey(record.DeploymentID, record.IdempotencyKey), strings.NewReader(record.ID), int64(len(raw)), nil)
	if err != nil {
		return fmt.Errorf("store receipt idempotency key: %w", err)
	}

	return nil
}

func (s *Service) loadIdempotency(ctx context.Context, deploymentID, key string) (Record, error) {
	record, err := s.records.LoadIdempotency(ctx, deploymentID, key)
	if err != nil {
		return Record{}, fmt.Errorf("load receipt repository index: %w", err)
	}

	return record, nil
}

func (r objectRecordRepository) LoadIdempotency(ctx context.Context, deploymentID, key string) (Record, error) {
	reader, _, err := r.store.Open(ctx, idempotencyKey(deploymentID, key))
	if err != nil {
		return Record{}, fmt.Errorf("open receipt idempotency key: %w", err)
	}

	raw, err := io.ReadAll(io.LimitReader(reader, maxStoredReceiptIDBytes))
	_ = reader.Close()

	if err != nil {
		return Record{}, fmt.Errorf("read receipt idempotency key: %w", err)
	}

	return r.Load(ctx, deploymentID, string(raw))
}

func (s *Service) findIdempotentReceipt(ctx context.Context, deploymentID, key string) (Record, bool, error) {
	record, err := s.loadIdempotency(ctx, deploymentID, key)
	if errors.Is(err, objectstore.ErrNotFound) || errors.Is(err, ErrNotFound) {
		return Record{}, false, nil
	}

	if err != nil {
		return Record{}, false, err
	}

	return record, true, nil
}

func (s *Service) deletePayload(ctx context.Context, r Record) error {
	_ = s.store.Delete(ctx, bodyKey(r.DeploymentID, r.ID))
	for _, part := range r.Parts {
		_ = s.store.Delete(ctx, partKey(r.DeploymentID, r.ID, part.Number))
	}

	return nil
}
func (s *Service) expired(r Record) bool { return !s.cfg.Now().Before(r.ExpiresAt) }
func (s *Service) signature(id string, values url.Values) string {
	h := hmac.New(sha256.New, s.cfg.SigningKey)
	_, _ = io.WriteString(h, id+"\n"+values.Encode())

	return hex.EncodeToString(h.Sum(nil))
}

func validSHA256(value string) bool {
	if len(value) != sha256HexBytes {
		return false
	}

	_, err := hex.DecodeString(value)

	return err == nil
}

func validID(value string) bool {
	if len(value) != len(receiptIDPrefix)+(receiptIDRandomBytes*hexCharsPerByte) || !strings.HasPrefix(value, receiptIDPrefix) {
		return false
	}

	_, err := hex.DecodeString(strings.TrimPrefix(value, receiptIDPrefix))

	return err == nil
}

func newID() (string, error) {
	var raw [receiptIDRandomBytes]byte

	_, err := rand.Read(raw[:])
	if err != nil {
		return "", fmt.Errorf("generate receipt id: %w", err)
	}

	return receiptIDPrefix + hex.EncodeToString(raw[:]), nil
}

func deploymentPrefix(deploymentID string) string {
	return "deployments/" + url.PathEscape(deploymentID) + "/receipts/"
}

func recordKey(deploymentID, id string) string {
	return deploymentPrefix(deploymentID) + id + "/record.json"
}
func bodyKey(deploymentID, id string) string { return deploymentPrefix(deploymentID) + id + "/body" }
func partKey(deploymentID, id string, number int) string {
	return deploymentPrefix(deploymentID) + id + "/parts/" + strconv.Itoa(number)
}

func partKeys(deploymentID, id string, parts []Part) []string {
	keys := make([]string, len(parts))
	for i, p := range parts {
		keys[i] = partKey(deploymentID, id, p.Number)
	}

	return keys
}

func idempotencyKey(deploymentID, key string) string {
	sum := sha256.Sum256([]byte(key))

	return "deployments/" + url.PathEscape(deploymentID) + "/idempotency/" + hex.EncodeToString(sum[:])
}

func deploymentFromRecordKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 5 || parts[0] != "deployments" || parts[2] != "receipts" {
		return ""
	}

	value, err := url.PathUnescape(parts[1])
	if err != nil {
		return ""
	}

	return value
}
