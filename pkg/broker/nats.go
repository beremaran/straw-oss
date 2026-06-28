//nolint:funcorder
package broker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	ErrNoBindingsForQueue = errors.New("no bindings found for queue")
	ErrNoStreamForSubject = errors.New("no stream found for subject")
)

type NatsBroker struct {
	mu   sync.RWMutex
	url  string
	conn *nats.Conn
	js   jetstream.JetStream
	opts []Option

	bindings map[string][]string
}

func NewNatsBroker(opts ...Option) *NatsBroker {
	options := Options{
		Addrs: []string{nats.DefaultURL},
	}
	for _, o := range opts {
		o(&options)
	}

	url := options.Addrs[0]
	if len(options.Addrs) > 0 {
		url = strings.Join(options.Addrs, ",")
	}

	b := &NatsBroker{
		url:      url,
		bindings: make(map[string][]string),
		opts:     opts,
	}

	return b
}

func (b *NatsBroker) Connect() error {
	opts := []nats.Option{
		nats.Name("straw"),
	}

	if len(b.opts) > 0 {
		config := Options{}
		for _, o := range b.opts {
			o(&config)
		}
		if config.Token != "" {
			opts = append(opts, nats.Token(config.Token))
		}
	}

	nc, err := nats.Connect(b.url, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	b.conn = nc

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}
	b.js = js

	return nil
}

func (b *NatsBroker) Close() error {
	if b.conn != nil {
		b.conn.Close()
	}

	return nil
}

func (b *NatsBroker) IsConnected() bool {
	return b.conn != nil && b.conn.IsConnected()
}

func (b *NatsBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	subject := fmt.Sprintf("%s.%s", exchange, routingKey)
	if exchange == "" {
		subject = routingKey
	}
	_, err := b.js.Publish(ctx, subject, body)
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	return nil
}

func (b *NatsBroker) Subscribe(ctx context.Context, queue string, handler Handler, opts ...SubscribeOption) error {
	b.mu.RLock()
	subjects, ok := b.bindings[queue]
	b.mu.RUnlock()
	if !ok || len(subjects) == 0 {
		return fmt.Errorf("%w: %s", ErrNoBindingsForQueue, queue)
	}

	subOpts := SubscribeOptions{}
	for _, o := range opts {
		o(&subOpts)
	}

	for _, subject := range subjects {
		streamName := b.findStreamForSubject(ctx, subject)
		if streamName == "" {
			return fmt.Errorf("%w: %s", ErrNoStreamForSubject, subject)
		}

		var durableName string
		if subOpts.Durable != nil {
			durableName = *subOpts.Durable
		} else {
			durableName = strings.ReplaceAll(queue, ".", "_")
		}

		consumerConfig := jetstream.ConsumerConfig{
			Durable:       durableName,
			FilterSubject: subject,
			DeliverPolicy: jetstream.DeliverNewPolicy,
			AckPolicy:     jetstream.AckExplicitPolicy,
			MaxAckPending: subOpts.MaxAckPending,
		}

		cons, err := b.js.CreateOrUpdateConsumer(ctx, streamName, consumerConfig)
		if err != nil {
			return fmt.Errorf("failed to create consumer for %s: %w", queue, err)
		}

		cc, err := cons.Consume(func(msg jetstream.Msg) {
			err := handler(ctx, msg.Data())
			if err != nil {
				_ = msg.Nak()
			} else {
				_ = msg.Ack()
			}
		})
		if err != nil {
			return fmt.Errorf("failed to start consumer for %s: %w", queue, err)
		}

		go func() {
			<-ctx.Done()
			cc.Stop()
		}()
	}

	return nil
}

func (b *NatsBroker) findStreamForSubject(ctx context.Context, subject string) string {
	streams := b.js.ListStreams(ctx)
	for stream := range streams.Info() {
		for _, pattern := range stream.Config.Subjects {
			if subjectMatchesPattern(pattern, subject) {
				return stream.Config.Name
			}
		}
	}

	return ""
}

func subjectMatchesPattern(pattern, subject string) bool {
	if pattern == subject {
		return true
	}

	pTokens := strings.Split(pattern, ".")
	sTokens := strings.Split(subject, ".")

	pLen := len(pTokens)
	sLen := len(sTokens)

	i := 0
	for i < pLen && i < sLen {
		pt := pTokens[i]

		if pt == ">" && i == pLen-1 {
			return true
		}

		if pt != "*" && pt != sTokens[i] {
			return false
		}

		i++
	}

	if i != sLen {
		return false
	}

	return i == pLen
}

func (b *NatsBroker) SubscribeTemporary(ctx context.Context, queue string, handler Handler) error {
	sub, err := b.conn.QueueSubscribe(queue, queue, func(msg *nats.Msg) {
		_ = handler(ctx, msg.Data)
	})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()

	return nil
}

func (b *NatsBroker) DeclareExchange(ctx context.Context, name, kind string) error {
	_, err := b.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     name,
		Subjects: []string{name + ".>"},
	})
	if err != nil {
		return fmt.Errorf("failed to create stream %s: %w", name, err)
	}

	return nil
}

func (b *NatsBroker) DeclareQueue(ctx context.Context, name string) error {
	b.mu.Lock()
	if _, ok := b.bindings[name]; !ok {
		b.bindings[name] = []string{}
	}
	b.mu.Unlock()

	return nil
}

func (b *NatsBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	subject := fmt.Sprintf("%s.%s", exchange, routingKey)
	if exchange == "" {
		subject = routingKey
	}
	if routingKey == "" {
		subject = exchange + ".>"
	}

	b.mu.Lock()
	b.bindings[queue] = append(b.bindings[queue], subject)
	b.mu.Unlock()

	return nil
}

func (b *NatsBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	return 0, nil
}

func (b *NatsBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	sub, err := b.conn.SubscribeSync(queue)
	if err != nil {
		return nil, err
	}
	defer func(sub *nats.Subscription) {
		_ = sub.Unsubscribe()
	}(sub)

	msg, err := sub.NextMsg(timeout)
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil, ErrTimeout
		}

		return nil, err
	}

	return msg.Data, nil
}
