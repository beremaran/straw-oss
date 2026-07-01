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
	url   string
	token string
	conn  *nats.Conn
	js    jetstream.JetStream
}

// NewNatsBroker creates a new NATS broker.
func NewNatsBroker(addr, token string) *NatsBroker {
	if addr == "" {
		addr = nats.DefaultURL
	}

	return &NatsBroker{
		url:   addr,
		token: token,
	}
}

// Connect establishes the NATS connection and JetStream context.
func (b *NatsBroker) Connect() error {
	opts := []nats.Option{
		nats.Name("straw"),
	}

	if b.token != "" {
		opts = append(opts, nats.Token(b.token))
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
func (b *NatsBroker) Subscribe(ctx context.Context, subject string, handler Handler, maxAckPending int) error {
	streamName := b.findStreamForSubject(ctx, subject)
	if streamName == "" {
		return fmt.Errorf("%w: %s", ErrNoStreamForSubject, subject)
	}

	durableName := strings.NewReplacer(".", "_", "*", "_", ">", "_").Replace(subject)

	consumerConfig := jetstream.ConsumerConfig{
		Durable:       durableName,
		FilterSubject: subject,
		DeliverPolicy: jetstream.DeliverNewPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: maxAckPending,
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

	wildcard := pTokens[pLen-1] == ">"
	if wildcard {
		pLen--
	}

	if len(sTokens) < pLen {
		return false
	}

	for i := 0; i < pLen; i++ {
		if !tokenMatches(pTokens[i], sTokens[i]) {
			return false
		}
	}

	return matchLength(wildcard, len(sTokens), pLen)
}

func tokenMatches(patternToken, subjectToken string) bool {
	if patternToken == "*" {
		return true
	}

	if patternToken == ">" {
		return true
	}

	return patternToken == subjectToken
}

func matchLength(wildcard bool, subjectLen, patternLen int) bool {
	if wildcard {
		return subjectLen > patternLen
	}

	return subjectLen == patternLen
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

// ConsumeOnce returns the first persisted message for subject.
func (b *NatsBroker) ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error) {
	streamName := b.findStreamForSubject(ctx, subject)
	if streamName == "" {
		return nil, fmt.Errorf("%w: %s", ErrNoStreamForSubject, subject)
	}

	consumer, err := b.js.CreateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		FilterSubject:     subject,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		InactiveThreshold: timeout + time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer for %s: %w", subject, err)
	}

	batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(timeout))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil, ErrTimeout
		}

		return nil, fmt.Errorf("failed to fetch from %s: %w", subject, err)
	}

	for msg := range batch.Messages() {
		data := append([]byte(nil), msg.Data()...)
		_ = msg.Ack()

		return data, nil
	}

	err = batch.Error()
	if err != nil && !errors.Is(err, nats.ErrTimeout) {
		return nil, fmt.Errorf("failed to receive message from %s: %w", subject, err)
	}

	return nil, ErrTimeout
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
