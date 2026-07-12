// Package objectstore provides the optional durable body store used by receipts.
package objectstore

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotFound reports a missing object.
	ErrNotFound          = errors.New("object not found")
	errInvalidKey        = errors.New("invalid object key")
	errLocalRootRequired = errors.New("local object root is required")
	errObjectSize        = errors.New("stored object size does not match declaration")
	errS3Config          = errors.New("incomplete S3 configuration")
	errS3Endpoint        = errors.New("invalid S3 endpoint")
	errS3Status          = errors.New("S3 returned an unsuccessful status")
)

// Object describes one stored object.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
	Metadata     map[string]string
}

// Store is the bounded streaming storage surface needed by receipt transport.
type Store interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, metadata map[string]string) error
	Open(ctx context.Context, key string) (io.ReadCloser, Object, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]Object, error)
}
