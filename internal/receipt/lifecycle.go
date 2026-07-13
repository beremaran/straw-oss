// Package receipt implements durable receipt-and-check body transport.
package receipt

import (
	"context"
	"crypto/hmac"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

// Get returns a deployment-scoped receipt record.
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

	return &strawpb.BodyRefFrame{Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{ObjectKey: "deployment/" + deploymentID + "/request/" + requestID + "/request/" + id, SignedUrl: signedURL, ExpiresUnixMs: expires.UnixMilli()}}, ExpectedSizeBytes: expectedSize, Sha256Hex: record.SHA256Hex}, nil
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
