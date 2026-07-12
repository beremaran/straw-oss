// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
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

	return &Service{store: store, cfg: cfg}, nil
}

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
func (s *Service) Get(ctx context.Context, deploymentID, id string) (Record, error) {
	if !validID(id) {
		return Record{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load(ctx, deploymentID, id)
}

// Cancel marks an eligible receipt cancelled and deletes its payload.
func (s *Service) Cancel(ctx context.Context, deploymentID, id string) (Record, error) {
	if !validID(id) {
		return Record{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.load(ctx, deploymentID, id)
	if err != nil {
		return Record{}, err
	}

	if record.State == StateAssigned || record.State == StateConsumed {
		return Record{}, ErrConflict
	}

	record.State = StateCancelled
	record.UpdatedAt = s.cfg.Now().UTC()
	_ = s.deletePayload(ctx, record)

	return record, s.save(ctx, record)
}

// PrepareRequest atomically claims a verified request receipt and returns the
// assignment-scoped reference sent to Egress.
func (s *Service) PrepareRequest(ctx context.Context, deploymentID, id, requestID string) (*strawpb.BodyRefFrame, error) {
	if !validAssignmentInput(id, deploymentID, requestID) {
		return nil, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.load(ctx, deploymentID, id)
	if err != nil {
		return nil, err
	}

	releaseExpiredAssignment(s, &record)

	if !assignmentEligible(s, record) {
		return nil, ErrConflict
	}

	expires := s.cfg.Now().UTC().Add(s.cfg.AssignmentTTL)
	record.State = StateAssigned

	s.assigned.Add(1)

	record.AssignedRequestID = requestID
	record.AssignmentExpiresAt = expires

	record.UpdatedAt = s.cfg.Now().UTC()

	err = s.save(ctx, record)
	if err != nil {
		return nil, err
	}

	values := url.Values{"deployment_id": {deploymentID}, "request_id": {requestID}, "expires": {strconv.FormatInt(expires.UnixMilli(), 10)}}
	values.Set("signature", s.signature(id, values))
	signedURL := strings.TrimRight(s.cfg.DownloadBaseURL, "/") + "/api/v1/receipt-objects/" + url.PathEscape(id) + "?" + values.Encode()

	expectedSize, sizeErr := strconv.ParseUint(strconv.FormatInt(record.SizeBytes, 10), 10, 64)
	if sizeErr != nil {
		return nil, fmt.Errorf("encode receipt size: %w", sizeErr)
	}

	return &strawpb.BodyRefFrame{Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{ObjectKey: "tenant/" + deploymentID + "/request/" + requestID + "/request/" + id, SignedUrl: signedURL, ExpiresUnixMs: expires.UnixMilli()}}, ExpectedSizeBytes: expectedSize, Sha256Hex: record.SHA256Hex}, nil
}

func releaseExpiredAssignment(s *Service, record *Record) {
	if record.State != StateAssigned || s.cfg.Now().Before(record.AssignmentExpiresAt) {
		return
	}

	record.State = StateVerified
	record.AssignedRequestID = ""
	record.AssignmentExpiresAt = time.Time{}
}

func assignmentEligible(s *Service, record Record) bool {
	return record.Direction == DirectionRequest && record.State == StateVerified && !s.expired(record)
}

// FinishRequest consumes a successful request receipt or releases a failed assignment.
func (s *Service) FinishRequest(ctx context.Context, deploymentID, id, requestID string, consumed bool) error {
	if !validID(id) || requestID == "" {
		return ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.load(ctx, deploymentID, id)
	if err != nil {
		return err
	}

	if record.State != StateAssigned || record.AssignedRequestID != requestID {
		return ErrConflict
	}

	if consumed {
		record.State = StateConsumed

		s.consumed.Add(1)
	} else {
		record.State = StateVerified
	}

	record.AssignedRequestID = ""
	record.AssignmentExpiresAt = time.Time{}
	record.UpdatedAt = s.cfg.Now().UTC()

	return s.save(ctx, record)
}

// OpenAssigned validates and opens an assignment-scoped request body.
func (s *Service) OpenAssigned(ctx context.Context, id, deploymentID, requestID, expires, signature string) (io.ReadCloser, Record, error) {
	expiresMS, err := validateAssignmentSignature(s, id, deploymentID, requestID, expires, signature)
	if err != nil {
		return nil, Record{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.load(ctx, deploymentID, id)
	if err != nil {
		return nil, Record{}, err
	}

	if record.State != StateAssigned || record.AssignedRequestID != requestID || record.AssignmentExpiresAt.UnixMilli() != expiresMS {
		return nil, Record{}, ErrUnauthorized
	}

	r, _, err := s.store.Open(ctx, bodyKey(deploymentID, id))
	if err != nil {
		return nil, Record{}, fmt.Errorf("open assigned receipt: %w", err)
	}

	return r, record, nil
}

func validateAssignmentSignature(s *Service, id, deploymentID, requestID, expires, signature string) (int64, error) {
	if !validAssignmentInput(id, deploymentID, requestID) {
		return 0, ErrUnauthorized
	}

	expiresMS, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || !s.cfg.Now().Before(time.UnixMilli(expiresMS)) {
		return 0, ErrUnauthorized
	}

	values := url.Values{"deployment_id": {deploymentID}, "request_id": {requestID}, "expires": {expires}}
	if !hmac.Equal([]byte(signature), []byte(s.signature(id, values))) {
		return 0, ErrUnauthorized
	}

	return expiresMS, nil
}

func validAssignmentInput(id, deploymentID, requestID string) bool {
	return validID(id) && deploymentID != "" && requestID != ""
}

// OpenResponse opens an authorized verified response receipt.
func (s *Service) OpenResponse(ctx context.Context, deploymentID, id string) (io.ReadCloser, Record, error) {
	if !validID(id) {
		return nil, Record{}, ErrInvalid
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.load(ctx, deploymentID, id)
	if err != nil {
		return nil, Record{}, err
	}

	if record.Direction != DirectionResponse || (record.State != StateVerified && record.State != StateConsumed) || s.expired(record) {
		return nil, Record{}, ErrConflict
	}

	r, _, err := s.store.Open(ctx, bodyKey(deploymentID, id))
	if err != nil {
		return nil, Record{}, fmt.Errorf("open response receipt: %w", err)
	}

	return r, record, nil
}

// Cleanup expires old receipts and releases stale assignments.
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

	var record Record

	err = json.NewDecoder(reader).Decode(&record)
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

func (s *Service) save(ctx context.Context, r Record) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("encode receipt record: %w", err)
	}

	err = s.store.Put(ctx, recordKey(r.DeploymentID, r.ID), strings.NewReader(string(raw)), int64(len(raw)), nil)
	if err != nil {
		return fmt.Errorf("store receipt record: %w", err)
	}

	return nil
}

func (s *Service) load(ctx context.Context, deploymentID, id string) (Record, error) {
	r, _, err := s.store.Open(ctx, recordKey(deploymentID, id))
	if errors.Is(err, objectstore.ErrNotFound) {
		return Record{}, ErrNotFound
	}

	if err != nil {
		return Record{}, fmt.Errorf("open receipt record: %w", err)
	}

	defer func() { _ = r.Close() }()

	var record Record

	err = json.NewDecoder(r).Decode(&record)
	if err != nil {
		return Record{}, fmt.Errorf("decode receipt record: %w", err)
	}

	record.DeploymentID = deploymentID

	return record, nil
}

func (s *Service) saveIdempotency(ctx context.Context, r Record) error {
	raw := []byte(r.ID)

	err := s.store.Put(ctx, idempotencyKey(r.DeploymentID, r.IdempotencyKey), strings.NewReader(r.ID), int64(len(raw)), nil)
	if err != nil {
		return fmt.Errorf("store receipt idempotency key: %w", err)
	}

	return nil
}

func (s *Service) loadIdempotency(ctx context.Context, deploymentID, key string) (Record, error) {
	r, _, err := s.store.Open(ctx, idempotencyKey(deploymentID, key))
	if err != nil {
		return Record{}, fmt.Errorf("open receipt idempotency key: %w", err)
	}

	raw, err := io.ReadAll(io.LimitReader(r, maxStoredReceiptIDBytes))
	_ = r.Close()

	if err != nil {
		return Record{}, fmt.Errorf("read receipt idempotency key: %w", err)
	}

	return s.load(ctx, deploymentID, string(raw))
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

type partReader struct {
	ctx     context.Context
	store   objectstore.Store
	keys    []string
	index   int
	current io.ReadCloser
}

func (r *partReader) Read(p []byte) (int, error) {
	for {
		if r.current == nil {
			if r.index >= len(r.keys) {
				return 0, io.EOF
			}

			var err error

			r.current, _, err = r.store.Open(r.ctx, r.keys[r.index])
			if err != nil {
				return 0, fmt.Errorf("open receipt part: %w", err)
			}

			r.index++
		}

		n, err := r.current.Read(p)
		if errors.Is(err, io.EOF) {
			_ = r.current.Close()
			r.current = nil

			if n > 0 {
				return n, nil
			}

			continue
		}

		if err != nil {
			return n, fmt.Errorf("read receipt part: %w", err)
		}

		return n, nil
	}
}

func (r *partReader) Close() error {
	if r.current != nil {
		err := r.current.Close()
		if err != nil {
			return fmt.Errorf("close receipt part: %w", err)
		}
	}

	return nil
}
