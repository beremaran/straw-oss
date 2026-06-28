package broker

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTimeout = errors.New("timeout")
)

type Handler func(ctx context.Context, body []byte) error

type SubscribeOptions struct {
	MaxAckPending int
	Durable       *string
}

type SubscribeOption func(*SubscribeOptions)

func WithMaxAckPending(max int) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.MaxAckPending = max
	}
}

func WithTransient() SubscribeOption {
	return func(o *SubscribeOptions) {
		empty := ""
		o.Durable = &empty
	}
}

func WithDurableName(name string) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.Durable = &name
	}
}

type MessageBroker interface {
	Publish(ctx context.Context, exchange, routingKey string, body []byte) error

	Subscribe(ctx context.Context, queue string, handler Handler, opts ...SubscribeOption) error

	SubscribeTemporary(ctx context.Context, queue string, handler Handler) error

	DeclareExchange(ctx context.Context, name, kind string) error

	DeclareQueue(ctx context.Context, name string) error

	BindQueue(ctx context.Context, queue, exchange, routingKey string) error

	ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error)

	IsConnected() bool

	QueueDepth(ctx context.Context, name string) (int, error)

	Close() error
}
