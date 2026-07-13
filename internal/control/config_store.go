package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/beremaran/straw-oss/internal/config"
)

const runtimeConfigKey = "deployment"

var (
	// ErrConfigConflict indicates a failed optimistic-concurrency comparison.
	ErrConfigConflict      = errors.New("runtime configuration revision conflict")
	errRuntimeNATSRequired = errors.New("nats connection is required")
)

// ConfigRecord is one durable, auditable activation of deployment configuration.
type ConfigRecord struct {
	Snapshot  config.Snapshot `json:"snapshot"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	CreatedAt time.Time       `json:"created_at"`
	Revision  uint64          `json:"revision,omitempty"`
}

// RuntimeConfigStore persists deployment configuration with optimistic concurrency.
type RuntimeConfigStore interface {
	Current() (ConfigRecord, error)
	Save(expectedRevision uint64, record ConfigRecord) (ConfigRecord, error)
	History() ([]ConfigRecord, error)
}

// NATSConfigStore uses a file-backed JetStream KV bucket. It adds no backing
// service beyond the NATS deployment Straw already requires.
type NATSConfigStore struct{ kv nats.KeyValue }

// NewNATSConfigStore opens or initializes the configured JetStream KV bucket.
func NewNATSConfigStore(conn *nats.Conn, bucket string, history uint8, initial config.Snapshot) (*NATSConfigStore, error) {
	if conn == nil {
		return nil, errRuntimeNATSRequired
	}

	js, err := conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("open jetstream: %w", err)
	}

	kv, err := js.KeyValue(bucket)
	if errors.Is(err, nats.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: bucket, Description: "Straw deployment runtime configuration", History: history, Storage: nats.FileStorage})
	}

	if err != nil {
		return nil, fmt.Errorf("open runtime configuration bucket: %w", err)
	}

	store := &NATSConfigStore{kv: kv}

	_, err = store.Current()
	if errors.Is(err, nats.ErrKeyNotFound) {
		initial.ConfigVersion = max(initial.ConfigVersion, 1)

		raw, marshalErr := json.Marshal(ConfigRecord{Snapshot: initial, Actor: "system", Action: "initialize", CreatedAt: time.Now().UTC()})
		if marshalErr != nil {
			return nil, fmt.Errorf("encode initial runtime configuration: %w", marshalErr)
		}

		_, err = kv.Create(runtimeConfigKey, raw)
	}

	if err != nil {
		return nil, fmt.Errorf("initialize runtime configuration: %w", err)
	}

	return store, nil
}

// Current returns the newest durable configuration record.
func (s *NATSConfigStore) Current() (ConfigRecord, error) {
	entry, err := s.kv.Get(runtimeConfigKey)
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("read runtime configuration key: %w", err)
	}

	return decodeConfigRecord(entry)
}

// Save updates the current record only when expectedRevision is current.
func (s *NATSConfigStore) Save(expectedRevision uint64, record ConfigRecord) (ConfigRecord, error) {
	record.Revision = 0
	record.CreatedAt = time.Now().UTC()

	raw, err := json.Marshal(record)
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("encode runtime configuration: %w", err)
	}

	revision, err := s.kv.Update(runtimeConfigKey, raw, expectedRevision)
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("%w: %w", ErrConfigConflict, err)
	}

	record.Revision = revision

	return record, nil
}

// History returns newest-first retained configuration records.
func (s *NATSConfigStore) History() ([]ConfigRecord, error) {
	entries, err := s.kv.History(runtimeConfigKey)
	if err != nil {
		return nil, fmt.Errorf("read runtime configuration key history: %w", err)
	}

	records := make([]ConfigRecord, 0, len(entries))
	for _, v := range slices.Backward(entries) {
		record, decodeErr := decodeConfigRecord(v)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode runtime configuration history: %w", decodeErr)
		}

		records = append(records, record)
	}

	return records, nil
}

func decodeConfigRecord(entry nats.KeyValueEntry) (ConfigRecord, error) {
	var record ConfigRecord

	err := json.Unmarshal(entry.Value(), &record)
	if err != nil {
		return ConfigRecord{}, fmt.Errorf("decode runtime configuration revision %d: %w", entry.Revision(), err)
	}

	record.Revision = entry.Revision()

	return record, nil
}

// MemoryConfigStore is the deterministic implementation used by unit tests.
type MemoryConfigStore struct {
	mu      sync.Mutex
	records []ConfigRecord
}

// NewMemoryConfigStore initializes deterministic in-process history.
func NewMemoryConfigStore(initial config.Snapshot) *MemoryConfigStore {
	return &MemoryConfigStore{records: []ConfigRecord{{Snapshot: initial.Clone(), Actor: "system", Action: "initialize", CreatedAt: time.Now().UTC(), Revision: 1}}}
}

// Current returns the latest in-memory record.
func (s *MemoryConfigStore) Current() (ConfigRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return cloneRecord(s.records[len(s.records)-1]), nil
}

// Save compare-and-swaps an in-memory record.
func (s *MemoryConfigStore) Save(expectedRevision uint64, record ConfigRecord) (ConfigRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if expectedRevision != s.records[len(s.records)-1].Revision {
		return ConfigRecord{}, ErrConfigConflict
	}

	record.Revision = expectedRevision + 1
	record.CreatedAt = time.Now().UTC()
	record.Snapshot = record.Snapshot.Clone()
	s.records = append(s.records, record)

	return cloneRecord(record), nil
}

// History returns newest-first in-memory records.
func (s *MemoryConfigStore) History() ([]ConfigRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]ConfigRecord, 0, len(s.records))
	for _, v := range slices.Backward(s.records) {
		out = append(out, cloneRecord(v))
	}

	return out, nil
}

func cloneRecord(record ConfigRecord) ConfigRecord {
	record.Snapshot = record.Snapshot.Clone()

	return record
}
