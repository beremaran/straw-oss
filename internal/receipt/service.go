// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beremaran/straw-oss/internal/objectstore"
)

// Receipt directions and lifecycle states are stable public values.
const (
	DirectionRequest        = "request"
	DirectionResponse       = "response"
	StateUploading          = "uploading"
	StateVerifying          = "verifying"
	StateVerified           = "verified"
	StateAssigned           = "assigned"
	StateConsumed           = "consumed"
	StateRejected           = "rejected"
	StateCancelled          = "cancelled"
	StateExpired            = "expired"
	receiptIDPrefix         = "rcpt_"
	receiptIDRandomBytes    = 16
	sha256HexBytes          = 64
	minimumSigningKeyBytes  = 32
	defaultMaxObjectBytes   = int64(1 << 30)
	defaultMaxPartBytes     = int64(16 << 20)
	defaultRetention        = 24 * time.Hour
	defaultAssignmentTTL    = 5 * time.Minute
	maxStoredReceiptIDBytes = 256
	hexCharsPerByte         = 2
)

var (
	// ErrNotFound reports an unknown receipt.
	ErrNotFound = errors.New("receipt not found")
	// ErrConflict reports an invalid lifecycle transition.
	ErrConflict = errors.New("receipt state conflict")
	// ErrInvalid reports malformed receipt input.
	ErrInvalid = errors.New("invalid receipt")
	// ErrUnauthorized reports an invalid assignment download capability.
	ErrUnauthorized        = errors.New("receipt download unauthorized")
	errStoreRequired       = errors.New("receipt object store is required")
	errSigningKeyRequired  = errors.New("receipt signing key must be at least 32 bytes")
	errDownloadURLRequired = errors.New("receipt download base URL is required")
)

// Config defines receipt limits, retention, signing, and time behavior.
type Config struct {
	DownloadBaseURL string
	SigningKey      []byte
	MaxObjectBytes  int64
	MaxPartBytes    int64
	Retention       time.Duration
	AssignmentTTL   time.Duration
	Now             func() time.Time
}

// Part records one durable upload part.
type Part struct {
	Number    int    `json:"number"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256Hex string `json:"sha256_hex"`
}

// Record is the durable, client-visible receipt state.
type Record struct {
	ID                  string    `json:"receipt_id"`
	DeploymentID        string    `json:"-"`
	Direction           string    `json:"direction"`
	State               string    `json:"state"`
	SizeBytes           int64     `json:"size_bytes"`
	SHA256Hex           string    `json:"sha256_hex"`
	Parts               []Part    `json:"parts,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	AssignedRequestID   string    `json:"assigned_request_id,omitempty"`
	AssignmentExpiresAt time.Time `json:"assignment_expires_at,omitzero"`
	Failure             string    `json:"failure,omitempty"`
	IdempotencyKey      string    `json:"-"`
}

// CreateInput declares the immutable final object properties.
type CreateInput struct {
	Direction                 string
	SizeBytes                 int64
	SHA256Hex, IdempotencyKey string
}

// Service manages durable receipt records and bodies.
type Service struct {
	store                                                                objectstore.Store
	records                                                              recordRepository
	cfg                                                                  Config
	mu                                                                   sync.Mutex
	created, parts, verified, rejected, assigned, consumed, expiredCount atomic.Uint64
}

// Stats contains process-lifetime receipt lifecycle counters.
type Stats struct {
	Created, Parts, Verified, Rejected, Assigned, Consumed, Expired uint64
}

// Stats returns current lifecycle counters.
func (s *Service) Stats() Stats {
	return Stats{Created: s.created.Load(), Parts: s.parts.Load(), Verified: s.verified.Load(), Rejected: s.rejected.Load(), Assigned: s.assigned.Load(), Consumed: s.consumed.Load(), Expired: s.expiredCount.Load()}
}

// New constructs a receipt service over the supplied durable object store.
func New(store objectstore.Store, cfg Config) (*Service, error) {
	if store == nil {
		return nil, errStoreRequired
	}

	if len(cfg.SigningKey) < minimumSigningKeyBytes {
		return nil, errSigningKeyRequired
	}

	if cfg.DownloadBaseURL == "" {
		return nil, errDownloadURLRequired
	}

	if cfg.MaxObjectBytes <= 0 {
		cfg.MaxObjectBytes = defaultMaxObjectBytes
	}

	if cfg.MaxPartBytes <= 0 {
		cfg.MaxPartBytes = defaultMaxPartBytes
	}

	if cfg.Retention <= 0 {
		cfg.Retention = defaultRetention
	}

	if cfg.AssignmentTTL <= 0 {
		cfg.AssignmentTTL = defaultAssignmentTTL
	}

	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	return &Service{store: store, records: objectRecordRepository{store: store}, cfg: cfg}, nil
}

// Create creates or idempotently returns an uploading receipt.
