package broker

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTimeout is returned when an operation times out.
	ErrTimeout = errors.New("timeout")
)

// Handler is a function that handles incoming messages.
// It returns an error if processing fails, which may trigger a retry/nack.
type Handler func(ctx context.Context, body []byte) error

// SubscribeOptions contains configuration for a subscription.
type SubscribeOptions struct {
	MaxAckPending int
	Durable       *string // Pointer to distinguish between unset (default logic) and empty (transient)
}

// SubscribeOption is a functional option for configuring a subscription.
type SubscribeOption func(*SubscribeOptions)

// WithMaxAckPending sets the maximum number of pending acknowledgments.
// This is used for backpressure control.
func WithMaxAckPending(max int) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.MaxAckPending = max
	}
}

// WithTransient forces the subscription to be non-durable (ephemeral).
// This is useful for consumers that don't need history across restarts (e.g. RPC results).
func WithTransient() SubscribeOption {
	return func(o *SubscribeOptions) {
		empty := ""
		o.Durable = &empty
	}
}

// WithDurableName sets a specific durable name for the consumer.
func WithDurableName(name string) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.Durable = &name
	}
}

// MessageBroker defines the interface for a message broker.
type MessageBroker interface {
	// Publish publishes a specific message to a topic/exchange with a routing key.
	// The ctx can be used to control the timeout of the publish operation.
	Publish(ctx context.Context, exchange, routingKey string, body []byte) error

	// Subscribe subscribes to a queue and invokes the handler for each message.
	// The ctx controls the lifecycle of the subscription. cancellation stops it.
	Subscribe(ctx context.Context, queue string, handler Handler, opts ...SubscribeOption) error

	// SubscribeTemporary creates a temporary (exclusive, auto-delete, non-durable) queue
	// and subscribes to it. This is useful for RPC response queues.
	SubscribeTemporary(ctx context.Context, queue string, handler Handler) error

	// DeclareExchange declares an exchange with the given name and kind.
	DeclareExchange(ctx context.Context, name, kind string) error

	// DeclareQueue declares a queue with the given name.
	DeclareQueue(ctx context.Context, name string) error

	// BindQueue binds a queue to an exchange with the given routing key.
	BindQueue(ctx context.Context, queue, exchange, routingKey string) error

	// ConsumeOnce waits for a single message on a temporary queue with timeout.
	// The queue is declared as exclusive and auto-delete. After receiving the message
	// or on timeout, the queue is cleaned up. Returns ErrTimeout if no message arrives
	// before the timeout expires.
	ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error)

	// IsConnected returns true if the broker is currently connected.
	IsConnected() bool

	// QueueDepth returns the number of messages currently in the queue.
	QueueDepth(ctx context.Context, name string) (int, error)

	// Close closes the broker connection.
	Close() error
}
