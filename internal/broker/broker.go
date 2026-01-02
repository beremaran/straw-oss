package broker

import (
	"context"
	"time"
)

// Handler is a function that handles incoming messages.
// It returns an error if processing fails, which may trigger a retry/nack.
type Handler func(ctx context.Context, body []byte) error

// MessageBroker defines the interface for a message broker.
type MessageBroker interface {
	// Publish publishes a message to a specific topic/exchange with a routing key.
	// The ctx can be used to control the timeout of the publish operation.
	Publish(ctx context.Context, exchange, routingKey string, body []byte) error

	// Subscribe subscribes to a queue and invokes the handler for each message.
	// The ctx controls the lifecycle of the subscription. cancellation stops it.
	Subscribe(ctx context.Context, queue string, handler Handler) error

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
