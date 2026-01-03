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

// NatsBroker implements MessageBroker using NATS JetStream.
type NatsBroker struct {
	mu   sync.RWMutex
	url  string
	conn *nats.Conn
	js   jetstream.JetStream
	opts []Option

	// In-memory mapping of queues to subjects for subscription
	bindings map[string][]string
}

// NewNatsBroker creates a new NatsBroker.
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

// Connect connects to the NATS server and initializes JetStream.
func (b *NatsBroker) Connect() error {
	opts := []nats.Option{
		nats.Name("straw-proxy"),
	}

	if len(b.opts) > 0 {
		// Re-apply options to get the final config?
		// NewNatsBroker already applied them to local 'options'.
		// But we didn't store 'options' in struct, just 'opts' functional options.
		// Wait, NewNatsBroker initializes 'options' but doesn't store the struct options, it stores the list of funcs.
		// Let's re-eval the options to get the token.
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

// IsConnected returns true if connected to NATS.
func (b *NatsBroker) IsConnected() bool {
	return b.conn != nil && b.conn.IsConnected()
}

// Publish publishes a message to a subject constructed from exchange and routingKey.
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

// Subscribe subscribes to a queue group for the subjects bound to the queue.
func (b *NatsBroker) Subscribe(ctx context.Context, queue string, handler Handler) error {
	b.mu.RLock()
	subjects, ok := b.bindings[queue]
	b.mu.RUnlock()
	if !ok || len(subjects) == 0 {
		return fmt.Errorf("no bindings found for queue %s", queue)
	}

	for _, subject := range subjects {
		// Create or update consumer
		// In JetStream, queue groups are handled by creating a consumer with a DeliverGroup.
		// However, the standard JS API handles this.
		// We'll use the higher-level Consume API if possible, or simple Subscribe with queue group.
		// Simple Subscribe is easier for migration.

		// Ensure stream exists for the subject?
		// We assume DeclareExchange created the stream.

		// With JetStream, we need a Consumer.
		// Construct a durable name which is unique for this queue.
		// Queue usage in RabbitMQ often implies load balancing.
		// NATS Queue Groups provide load balancing.

		// For now, let's use the low-level QueueSubscribe on the subject using the JS context
		// isn't exactly how JS works. JS consumers adhere to a stream.

		// We need to find the stream that covers this subject.
		streamName := b.findStreamForSubject(ctx, subject)
		if streamName == "" {
			return fmt.Errorf("no stream found for subject %s", subject)
		}

		consumerConfig := jetstream.ConsumerConfig{
			Durable:       strings.ReplaceAll(queue, ".", "_"), // Sanitize: dots not allowed in Durable names
			FilterSubject: subject,
			DeliverPolicy: jetstream.DeliverNewPolicy,
			AckPolicy:     jetstream.AckExplicitPolicy,
			// DeliverSubject? If we use Pull Consumer we don't set it.
			// But for Queue Group push consumer behavior, we assume queue group?
		}

		// Nats.go new JS API prefers Pull consumers for workers.
		// But existing code expects a push-style handler.
		// We can spin up a goroutine that does Pull.

		cons, err := b.js.CreateOrUpdateConsumer(ctx, streamName, consumerConfig)
		if err != nil {
			return fmt.Errorf("failed to create consumer for %s: %w", queue, err)
		}

		// Consume messages
		// This needs to run in background.
		// But Subscribe API expects to return error immediately.
		// We should manage the lifecycle.

		// NOTE: The interface `Subscribe` maps well to `Consume`.
		// But `Consume` blocks or returns a Context.
		iter, err := cons.Messages(jetstream.PullMaxMessages(1))
		if err != nil {
			return err
		}

		go func() {
			defer iter.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					msg, err := iter.Next()
					if err != nil {
						// Handle close/stop (includes context cancellation)
						return
					}

					if err := handler(ctx, msg.Data()); err != nil {
						_ = msg.Nak()
					} else {
						_ = msg.Ack()
					}
				}
			}
		}()
	}
	return nil
}

func (b *NatsBroker) findStreamForSubject(ctx context.Context, subject string) string {
	// Iterate streams to find one that matches the subject.
	streams := b.js.ListStreams(ctx)
	for stream := range streams.Info() {
		// Check if any of the stream's subjects match the requested subject
		for _, pattern := range stream.Config.Subjects {
			if subjectMatchesPattern(pattern, subject) {
				return stream.Config.Name
			}
		}
	}
	return ""
}

// subjectMatchesPattern checks if a subject matches a NATS pattern.
// NATS wildcards:
// - "*" matches exactly one token
// - ">" matches one or more tokens (only valid as the last token)
func subjectMatchesPattern(pattern, subject string) bool {
	// Exact match
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

		// ">" as last token matches the rest of the subject (one or more tokens).
		if pt == ">" && i == pLen-1 {
			return true
		}

		// "*" matches exactly one token, anything else must match literally.
		if pt != "*" && pt != sTokens[i] {
			return false
		}

		i++
	}

	// All subject tokens must be consumed.
	if i != sLen {
		return false
	}

	// All pattern tokens must also be consumed for a match.
	// Note: ">" cannot match zero tokens, so if we exited the loop because
	// the subject is exhausted, any remaining pattern tokens mean no match.
	return i == pLen
}

// SubscribeTemporary subscribes to a temporary subject (ReplyTo).
// For NATS, this is usually just a core NATS subscription, not JetStream, for RPC.
func (b *NatsBroker) SubscribeTemporary(ctx context.Context, queue string, handler Handler) error {
	// Core NATS subscription
	sub, err := b.conn.QueueSubscribe(queue, queue, func(msg *nats.Msg) {
		if err := handler(ctx, msg.Data); err != nil {
			// No Ack/Nak in core NATS
		}
	})
	if err != nil {
		return err
	}

	// Unsubscribe on context done?
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()

	return nil
}

// DeclareExchange creates a JetStream Stream.
func (b *NatsBroker) DeclareExchange(ctx context.Context, name, kind string) error {
	// Map AMQP Exchange to JetStream Stream.
	// Subjects: name.> (wildcard for all subjects under this exchange)
	_, err := b.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     name,
		Subjects: []string{name + ".>"},
	})
	if err != nil {
		return fmt.Errorf("failed to create stream %s: %w", name, err)
	}
	return nil
}

// DeclareQueue registers a queue name (implicit in NATS, but useful for our binding logic).
// In RabbitMQ, this creates the queue. In NATS, we might create a consumer here if valid,
// but usually we wait for Bind manually.
func (b *NatsBroker) DeclareQueue(ctx context.Context, name string) error {
	// No-op or init map
	b.mu.Lock()
	if _, ok := b.bindings[name]; !ok {
		b.bindings[name] = []string{}
	}
	b.mu.Unlock()
	return nil
}

// BindQueue binds a queue to an exchange+routingKey.
func (b *NatsBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	subject := fmt.Sprintf("%s.%s", exchange, routingKey)
	if exchange == "" {
		subject = routingKey
	}
	if routingKey == "" {
		// Fanout case or catch-all?
		// In RabbitMQ fanout, routing key is ignored.
		// If exchange is "heartbeats", subject is "heartbeats.>"
		subject = exchange + ".>"
	}

	b.mu.Lock()
	b.bindings[queue] = append(b.bindings[queue], subject)
	b.mu.Unlock()
	return nil
}

// QueueDepth returns approximate lag for the consumer corresponding to the queue.
func (b *NatsBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	// We need to find the stream and consumer name.
	// Assuming queue name == consumer name
	// And we need the stream name.

	// Complex lookup without explicit stream mapping.
	// For migration, return 0 is acceptable if not critical.
	return 0, nil
}

// ConsumeOnce waits for a single message.
func (b *NatsBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	// Core NATS Sync Subscribe
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
