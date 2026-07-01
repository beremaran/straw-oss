// Package broker provides a message broker abstraction with a NATS JetStream implementation.
package broker

import (
	"context"
	"errors"
	"time"
)

// ErrTimeout is returned when ConsumeOnce waits longer than the provided timeout.
var ErrTimeout = errors.New("timeout")

// Handler processes a received message body.
type Handler func(ctx context.Context, body []byte) error

// SubscribeOptions configures subscription behavior.
type SubscribeOptions struct {
	MaxAckPending int
	Durable       *string
}

// SubscribeOption sets a subscription option.
type SubscribeOption func(*SubscribeOptions)

// WithMaxAckPending sets the maximum number of unacknowledged messages.
func WithMaxAckPending(maxPending int) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.MaxAckPending = maxPending
	}
}

// WithTransient sets the subscription to use a transient (non-durable) consumer.
func WithTransient() SubscribeOption {
	return func(o *SubscribeOptions) {
		empty := ""
		o.Durable = &empty
	}
}

// WithDurableName sets the consumer to use the given durable name.
func WithDurableName(name string) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.Durable = &name
	}
}

// MessageBroker is the contract for message broker implementations.
type MessageBroker interface {
	Publish(ctx context.Context, subject string, body []byte) error

	Subscribe(ctx context.Context, subject string, handler Handler, opts ...SubscribeOption) error

	ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error)

	DeclareStream(ctx context.Context, name string, subjects ...string) error

	IsConnected() bool

	Close() error
}
