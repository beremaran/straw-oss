package broker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// ErrNoStreamForSubject is returned when no stream is found for a subject.
var ErrNoStreamForSubject = errors.New("no stream found for subject")

// NatsBroker is a NATS JetStream message broker implementation.
type NatsBroker struct {
	url  string
	conn *nats.Conn
	js   jetstream.JetStream
	opts []Option
}

// NewNatsBroker creates a new NATS broker with the given options.
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
		url:  url,
		opts: opts,
	}

	return b
}

// Connect establishes the NATS connection and JetStream context.
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

// Close closes the NATS connection.
func (b *NatsBroker) Close() error {
	if b.conn != nil {
		b.conn.Close()
	}

	return nil
}

// IsConnected reports whether the broker has an active NATS connection.
func (b *NatsBroker) IsConnected() bool {
	return b.conn != nil && b.conn.IsConnected()
}

// Publish sends a message to the given subject.
func (b *NatsBroker) Publish(ctx context.Context, subject string, body []byte) error {
	_, err := b.js.Publish(ctx, subject, body)
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	return nil
}

// Subscribe creates a JetStream consumer for the given subject and routes messages to the handler.
func (b *NatsBroker) Subscribe(ctx context.Context, subject string, handler Handler, opts ...SubscribeOption) error {
	subOpts := SubscribeOptions{}
	for _, o := range opts {
		o(&subOpts)
	}

	streamName := b.findStreamForSubject(ctx, subject)
	if streamName == "" {
		return fmt.Errorf("%w: %s", ErrNoStreamForSubject, subject)
	}

	var durableName string
	if subOpts.Durable != nil {
		durableName = *subOpts.Durable
	} else {
		durableName = strings.NewReplacer(".", "_", "*", "_", ">", "_").Replace(subject)
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
		return fmt.Errorf("failed to create consumer for %s: %w", subject, err)
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
		return fmt.Errorf("failed to start consumer for %s: %w", subject, err)
	}

	go func() {
		<-ctx.Done()
		cc.Stop()
	}()

	return nil
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

// DeclareStream creates or updates a stream for the given name and subjects.
func (b *NatsBroker) DeclareStream(ctx context.Context, name string, subjects ...string) error {
	if len(subjects) == 0 {
		subjects = []string{name + ".>"}
	}

	_, err := b.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     name,
		Subjects: subjects,
	})
	if err != nil {
		return fmt.Errorf("failed to create stream %s: %w", name, err)
	}

	return nil
}

// ConsumeOnce subscribes synchronously, returns the first message, then unsubscribes.
func (b *NatsBroker) ConsumeOnce(_ context.Context, subject string, timeout time.Duration) ([]byte, error) {
	sub, err := b.conn.SubscribeSync(subject)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	defer func(sub *nats.Subscription) {
		_ = sub.Unsubscribe()
	}(sub)

	msg, err := sub.NextMsg(timeout)
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil, ErrTimeout
		}

		return nil, fmt.Errorf("failed to receive message from %s: %w", subject, err)
	}

	return msg.Data, nil
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
