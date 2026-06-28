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
	Publish(ctx context.Context, subject string, body []byte) error

	Subscribe(ctx context.Context, subject string, handler Handler, opts ...SubscribeOption) error

	ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error)

	DeclareStream(ctx context.Context, name string, subjects ...string) error

	IsConnected() bool

	Close() error
}
