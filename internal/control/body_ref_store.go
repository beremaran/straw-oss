package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/objectstore"
)

var errBodyRefObjectStatus = errors.New("body object request returned non-2xx status")

// RequestBodyRefStore uploads a request body and returns the scoped BodyRef
// frame Control sends to the assigned executor.
type RequestBodyRefStore interface {
	UploadRequestBody(ctx context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error)
	DeleteRequestBody(ctx context.Context, frame *strawpb.BodyRefFrame) error
}

// ResponseBodyRefStore uploads a fully-buffered response body to object storage
// (the P2 "tee") and returns the BodyRef frame that backs the REST download
// reference. Control tees only after the synchronous stream from the executor
// completes, so a cancelled or errored request never creates an object.
type ResponseBodyRefStore interface {
	UploadResponseBody(ctx context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error)
	DeleteResponseBody(ctx context.Context, frame *strawpb.BodyRefFrame) error
}

// S3RequestBodyRefStore backs both request and response BodyRef with the
// objectstore SigV4 single-object presigned PUT/GET primitives. The name is
// historical: it implements both RequestBodyRefStore and ResponseBodyRefStore.
type S3RequestBodyRefStore struct {
	Client     *objectstore.Client
	HTTPClient *http.Client
	Expiry     time.Duration
	Now        func() time.Time
}

// NewS3RequestBodyRefStore builds the production request BodyRef store.
func NewS3RequestBodyRefStore(client *objectstore.Client) *S3RequestBodyRefStore {
	return &S3RequestBodyRefStore{Client: client}
}

// UploadRequestBody uploads body and returns a scoped S3 BodyRef frame.
func (s *S3RequestBodyRefStore) UploadRequestBody(ctx context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error) {
	return s.uploadBody(ctx, tenantID, requestID, objectstore.DirectionRequest, body)
}

// UploadResponseBody uploads a buffered response body and returns a scoped S3
// BodyRef frame (the P2 response-body tee, docs/planning/18 S3 Response Body
// Flow).
func (s *S3RequestBodyRefStore) UploadResponseBody(ctx context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error) {
	return s.uploadBody(ctx, tenantID, requestID, objectstore.DirectionResponse, body)
}

// DeleteRequestBody deletes a previously uploaded request body object.
func (s *S3RequestBodyRefStore) DeleteRequestBody(ctx context.Context, frame *strawpb.BodyRefFrame) error {
	return s.deleteBody(ctx, frame)
}

// DeleteResponseBody deletes a previously uploaded response body object.
func (s *S3RequestBodyRefStore) DeleteResponseBody(ctx context.Context, frame *strawpb.BodyRefFrame) error {
	return s.deleteBody(ctx, frame)
}

func (s *S3RequestBodyRefStore) uploadBody(ctx context.Context, tenantID, requestID string, dir objectstore.Direction, body []byte) (*strawpb.BodyRefFrame, error) {
	if s == nil || s.Client == nil {
		return nil, fmt.Errorf("body ref store: %w", objectstore.ErrUnavailable)
	}

	key, err := s.Client.ObjectKey(tenantID, requestID, dir)
	if err != nil {
		return nil, fmt.Errorf("body ref key: %w", objectstore.Unavailable(err))
	}

	expiry := bodyRefExpiry(s.Expiry)

	put, err := s.Client.PresignPut(key, expiry)
	if err != nil {
		return nil, fmt.Errorf("body ref put: %w", objectstore.Unavailable(err))
	}

	err = s.putObject(ctx, put, body)
	if err != nil {
		return nil, err
	}

	getURL, err := s.Client.PresignGet(key, expiry)
	if err != nil {
		return nil, fmt.Errorf("body ref get: %w", objectstore.Unavailable(err))
	}

	return s.bodyRefFrame(key, getURL, expiry, body), nil
}

func (s *S3RequestBodyRefStore) deleteBody(ctx context.Context, frame *strawpb.BodyRefFrame) error {
	if s == nil || s.Client == nil {
		return fmt.Errorf("body ref store: %w", objectstore.ErrUnavailable)
	}

	if frame == nil || frame.GetS3() == nil {
		return nil
	}

	key := frame.GetS3().GetObjectKey()
	if key == "" {
		return nil
	}

	del, err := s.Client.PresignDelete(key, bodyRefExpiry(s.Expiry))
	if err != nil {
		return fmt.Errorf("request body ref delete: %w", objectstore.Unavailable(err))
	}

	return s.deleteObject(ctx, del)
}

func (s *S3RequestBodyRefStore) deleteObject(ctx context.Context, del objectstore.PresignedDelete) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, del.URL, nil)
	if err != nil {
		return fmt.Errorf("request body ref delete request: %w", objectstore.Unavailable(err))
	}

	resp, err := s.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("request body ref delete do: %w", objectstore.Unavailable(err))
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err = fmt.Errorf("%w: %d", errBodyRefObjectStatus, resp.StatusCode)

		return fmt.Errorf("request body ref delete status: %w", objectstore.Unavailable(err))
	}

	return nil
}

func (s *S3RequestBodyRefStore) putObject(ctx context.Context, put objectstore.PresignedPut, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, put.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("request body ref put request: %w", objectstore.Unavailable(err))
	}

	for name, value := range put.Headers {
		req.Header.Set(name, value)
	}

	resp, err := s.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("request body ref put do: %w", objectstore.Unavailable(err))
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err = fmt.Errorf("%w: %d", errBodyRefObjectStatus, resp.StatusCode)

		return fmt.Errorf("request body ref put status: %w", objectstore.Unavailable(err))
	}

	return nil
}

func (s *S3RequestBodyRefStore) bodyRefFrame(key, getURL string, expiry time.Duration, body []byte) *strawpb.BodyRefFrame {
	sum := sha256.Sum256(body)

	return &strawpb.BodyRefFrame{
		Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{
			ObjectKey:     key,
			SignedUrl:     getURL,
			ExpiresUnixMs: s.now().Add(expiry).UnixMilli(),
		}},
		ExpectedSizeBytes: uint64(len(body)),
		Sha256Hex:         hex.EncodeToString(sum[:]),
	}
}

func (s *S3RequestBodyRefStore) httpClient() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}

	return http.DefaultClient
}

func (s *S3RequestBodyRefStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}

	return time.Now()
}

func bodyRefExpiry(v time.Duration) time.Duration {
	if v <= 0 {
		return objectstore.DefaultPresignExpiry
	}

	if v > objectstore.MaxPresignExpiry {
		return objectstore.MaxPresignExpiry
	}

	return v
}
