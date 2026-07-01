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
