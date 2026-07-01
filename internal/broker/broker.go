// Package broker provides a message broker abstraction with a NATS JetStream implementation.
package broker

import (
	"context"
	"errors"
)

// ErrTimeout is returned when ConsumeOnce waits longer than the provided timeout.
var ErrTimeout = errors.New("timeout")

// Handler processes a received message body.
type Handler func(ctx context.Context, body []byte) error

// RequestHandler processes a core NATS request and returns the reply body.
type RequestHandler func(ctx context.Context, body []byte) ([]byte, error)

// Subscription is a live core NATS subscription.
type Subscription interface {
	Unsubscribe() error
}
