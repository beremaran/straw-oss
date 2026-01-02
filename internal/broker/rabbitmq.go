package broker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	instrumentationName = "github.com/kwilabs/straw-proxy-server/internal/broker"
)

// RabbitMQBroker implements MessageBroker using RabbitMQ.
type RabbitMQBroker struct {
	conn            *amqp.Connection
	opts            Options
	mu              sync.RWMutex
	isConnected     bool
	notifyConnClose chan *amqp.Error
	done            chan struct{}
	breaker         *circuitbreaker.CircuitBreaker

	// Channels
	mgmtChannel *amqp.Channel // For topology operations
	mgmtMu      sync.Mutex

	pubChannel *amqp.Channel // For publishing
	pubMu      sync.Mutex
}

// WithCircuitBreaker sets the circuit breaker for the broker.
func WithCircuitBreaker(cb *circuitbreaker.CircuitBreaker) Option {
	return func(o *Options) {
		o.CircuitBreaker = cb
	}
}

// NewRabbitMQBroker creates a new RabbitMQBroker.
func NewRabbitMQBroker(opts ...Option) *RabbitMQBroker {
	options := Options{
		ReconnectWait: 5 * time.Second,
	}
	for _, o := range opts {
		o(&options)
	}
	return &RabbitMQBroker{
		opts:    options,
		done:    make(chan struct{}),
		breaker: options.CircuitBreaker,
	}
}

// Connect establishes the connection to RabbitMQ.
func (b *RabbitMQBroker) Connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.isConnected {
		return nil
	}

	return b.connect()
}

func (b *RabbitMQBroker) connect() error {
	var err error
	var conn *amqp.Connection

	url := "amqp://guest:guest@localhost:5672/"
	if len(b.opts.Addrs) > 0 {
		url = b.opts.Addrs[0]
	}

	config := amqp.Config{
		TLSClientConfig: b.opts.TLSConfig,
	}

	conn, err = amqp.DialConfig(url, config)
	if err != nil {
		return fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	// Create Management Channel
	mgmtCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open mgmt channel: %w", err)
	}

	// Create Publisher Channel
	pubCh, err := conn.Channel()
	if err != nil {
		mgmtCh.Close()
		conn.Close()
		return fmt.Errorf("failed to open pub channel: %w", err)
	}

	b.conn = conn
	b.mgmtChannel = mgmtCh
	b.pubChannel = pubCh
	b.isConnected = true
	b.notifyConnClose = make(chan *amqp.Error, 1)
	b.conn.NotifyClose(b.notifyConnClose)

	go b.handleReconnect(b.notifyConnClose)

	return nil
}

func (b *RabbitMQBroker) handleReconnect(notifyClose <-chan *amqp.Error) {
	select {
	case <-b.done:
		return
	case err := <-notifyClose:
		if err == nil {
			// Clean shutdown
			return
		}

		b.mu.Lock()
		b.isConnected = false
		b.mu.Unlock()

		log.Printf("RabbitMQ connection lost: %v. Reconnecting...", err)

		for {
			select {
			case <-b.done:
				return
			case <-time.After(b.opts.ReconnectWait):
				b.mu.Lock()
				err := b.connect()
				b.mu.Unlock()
				if err == nil {
					log.Println("RabbitMQ reconnected")
					return
				}
				log.Printf("Failed to reconnect to RabbitMQ: %v. Retrying...", err)
			}
		}
	}
}

// DeclareExchange declares an exchange with the given name and kind.
// DeclareExchange declares an exchange with the given name and kind.
func (b *RabbitMQBroker) DeclareExchange(ctx context.Context, name, kind string) error {
	b.mu.RLock()
	if !b.isConnected {
		b.mu.RUnlock()
		return errors.New("broker not connected")
	}
	b.mu.RUnlock()

	b.mgmtMu.Lock()
	defer b.mgmtMu.Unlock()

	if b.mgmtChannel == nil || b.mgmtChannel.IsClosed() {
		ch, err := b.conn.Channel()
		if err != nil {
			return fmt.Errorf("failed to reopen mgmt channel: %w", err)
		}
		b.mgmtChannel = ch
	}

	return b.mgmtChannel.ExchangeDeclare(
		name,  // name
		kind,  // kind
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
}

// DeclareQueue declares a queue with the given name.
// DeclareQueue declares a queue with the given name.
func (b *RabbitMQBroker) DeclareQueue(ctx context.Context, name string) error {
	b.mu.RLock()
	if !b.isConnected {
		b.mu.RUnlock()
		return errors.New("broker not connected")
	}
	b.mu.RUnlock()

	b.mgmtMu.Lock()
	defer b.mgmtMu.Unlock()

	if b.mgmtChannel == nil || b.mgmtChannel.IsClosed() {
		ch, err := b.conn.Channel()
		if err != nil {
			return fmt.Errorf("failed to reopen mgmt channel: %w", err)
		}
		b.mgmtChannel = ch
	}

	_, err := b.mgmtChannel.QueueDeclare(
		name,  // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	return err
}

// BindQueue binds a queue to an exchange with the given routing key.
// BindQueue binds a queue to an exchange with the given routing key.
func (b *RabbitMQBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	b.mu.RLock()
	if !b.isConnected {
		b.mu.RUnlock()
		return errors.New("broker not connected")
	}
	b.mu.RUnlock()

	b.mgmtMu.Lock()
	defer b.mgmtMu.Unlock()

	if b.mgmtChannel == nil || b.mgmtChannel.IsClosed() {
		ch, err := b.conn.Channel()
		if err != nil {
			return fmt.Errorf("failed to reopen mgmt channel: %w", err)
		}
		b.mgmtChannel = ch
	}

	return b.mgmtChannel.QueueBind(
		queue,      // queue name
		routingKey, // routing key
		exchange,   // exchange
		false,      // no-wait
		nil,        // args
	)
}

// Publish publishes a message to the exchange with the given routing key.
func (b *RabbitMQBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	b.mu.RLock()
	if !b.isConnected {
		b.mu.RUnlock()
		return errors.New("broker not connected")
	}
	b.mu.RUnlock()

	publishFn := func() error {
		tracer := otel.Tracer(instrumentationName)
		ctx, span := tracer.Start(ctx, "mq.publish",
			trace.WithSpanKind(trace.SpanKindProducer),
			trace.WithAttributes(
				semconv.MessagingSystem("rabbitmq"),
				semconv.MessagingDestinationName(exchange),
				attribute.String("messaging.destination.kind", "topic"), // semconv.MessagingDestinationKindTopic might be missing in this version
				attribute.String("messaging.rabbitmq.routing_key", routingKey),
			),
		)
		defer span.End()

		headers := amqp.Table{}
		Inject(ctx, headers)

		b.pubMu.Lock()
		defer b.pubMu.Unlock()

		if b.pubChannel == nil || b.pubChannel.IsClosed() {
			// Try to reopen
			ch, err := b.conn.Channel()
			if err != nil {
				return fmt.Errorf("failed to reopen pub channel: %w", err)
			}
			b.pubChannel = ch
		}

		err := b.pubChannel.PublishWithContext(ctx,
			exchange,   // exchange
			routingKey, // routing key
			false,      // mandatory
			false,      // immediate
			amqp.Publishing{
				Headers:     headers,
				ContentType: "application/json", // Default to JSON, maybe make configurable
				Body:        body,
			})

		return err
	}

	if b.breaker != nil {
		return b.breaker.Execute(publishFn)
	}
	return publishFn()
}

// Subscribe subscribes to a queue and consumes messages.
func (b *RabbitMQBroker) Subscribe(ctx context.Context, queue string, handler Handler) error {
	b.mu.RLock()
	if !b.isConnected {
		b.mu.RUnlock()
		return errors.New("broker not connected")
	}
	conn := b.conn
	b.mu.RUnlock()

	// Create dedicated channel for this subscription
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open consumer channel: %w", err)
	}

	q, err := ch.QueueDeclare(
		queue, // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	// Set QoS prefetch if configured
	if b.opts.PrefetchCount > 0 {
		if err := ch.Qos(b.opts.PrefetchCount, 0, false); err != nil {
			_ = ch.Close()
			return fmt.Errorf("failed to set QoS: %w", err)
		}
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		_ = ch.Close()
		return err
	}

	// Instrument handler
	instrumentedHandler := func(ctx context.Context, body []byte) error {
		return handler(ctx, body)
	}

	go func() {
		// Close the channel when the context is cancelled or consumer exits,
		// cleaning up resources on the broker.
		defer ch.Close()

		tracer := otel.Tracer(instrumentationName)

		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-msgs:
				if !ok {
					return // Channel closed
				}

				// Extract context
				msCtx := Extract(context.Background(), d.Headers)
				msCtx, span := tracer.Start(msCtx, "mq.consume",
					trace.WithSpanKind(trace.SpanKindConsumer),
					trace.WithAttributes(
						semconv.MessagingSystem("rabbitmq"),
						semconv.MessagingDestinationName(queue),
						semconv.MessagingOperationProcess,
					),
				)

				if err := instrumentedHandler(msCtx, d.Body); err != nil {
					span.RecordError(err)
					// Nack on error
					d.Nack(false, false)
				} else {
					d.Ack(false)
				}
				span.End()
			}
		}
	}()

	return nil
}

// SubscribeTemporary creates a temporary (exclusive, auto-delete, non-durable) queue
// and subscribes to it. This is useful for RPC response queues.
func (b *RabbitMQBroker) SubscribeTemporary(ctx context.Context, queue string, handler Handler) error {
	b.mu.RLock()
	if !b.isConnected {
		b.mu.RUnlock()
		return errors.New("broker not connected")
	}
	conn := b.conn
	b.mu.RUnlock()

	// Create dedicated channel
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open consumer channel: %w", err)
	}

	q, err := ch.QueueDeclare(
		queue, // name
		false, // durable (false for temp)
		true,  // delete when unused (true for temp)
		false, // exclusive (false - must be accessible by other connections for RPC)
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("failed to declare temporary queue: %w", err)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack (ok for temp/RPC usually, or false if we want reliability)
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		_ = ch.Close()
		return err
	}

	go func() {
		defer ch.Close()
		tracer := otel.Tracer(instrumentationName)

		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-msgs:
				if !ok {
					return
				}

				// Extract context
				msCtx := Extract(context.Background(), d.Headers)
				msCtx, span := tracer.Start(msCtx, "mq.consume_temp",
					trace.WithSpanKind(trace.SpanKindConsumer),
					trace.WithAttributes(
						semconv.MessagingSystem("rabbitmq"),
						semconv.MessagingDestinationName(queue),
						semconv.MessagingOperationProcess,
						attribute.Bool("messaging.temp_queue", true),
					),
				)

				if err := handler(msCtx, d.Body); err != nil {
					span.RecordError(err)
					// For temporary/RPC queues, nack behavior depends on needs.
					// If auto-ack is true, we don't need to do anything.
				}
				span.End()
			}
		}
	}()

	return nil
}

// ErrTimeout is returned when ConsumeOnce times out waiting for a message.
var ErrTimeout = errors.New("timeout waiting for message")

// ConsumeOnce waits for a single message on a temporary queue with timeout.
// The queue is declared as exclusive and auto-delete. After receiving the message
// or on timeout, the queue is cleaned up.
func (b *RabbitMQBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	b.mu.RLock()
	if !b.isConnected {
		b.mu.RUnlock()
		return nil, errors.New("broker not connected")
	}
	conn := b.conn
	b.mu.RUnlock()

	// Create dedicated channel
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open consumer channel: %w", err)
	}
	defer ch.Close()

	// Declare temporary queue (exclusive, auto-delete)
	q, err := ch.QueueDeclare(
		queue, // name
		false, // durable
		true,  // delete when unused
		false, // exclusive (false - must be accessible by other connections for RPC)
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare temporary queue: %w", err)
	}

	// Start consuming
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		true,   // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to consume from queue: %w", err)
	}

	// Wait for message or timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case msg, ok := <-msgs:
		if !ok {
			return nil, errors.New("channel closed")
		}
		return msg.Body, nil
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrTimeout
	}
}

// Close closes the connection.
func (b *RabbitMQBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isConnected {
		return nil
	}

	close(b.done)

	// Close management channel
	b.mgmtMu.Lock()
	if b.mgmtChannel != nil {
		_ = b.mgmtChannel.Close()
	}
	b.mgmtMu.Unlock()

	// Close publisher channel
	b.pubMu.Lock()
	if b.pubChannel != nil {
		_ = b.pubChannel.Close()
	}
	b.pubMu.Unlock()

	if err := b.conn.Close(); err != nil {
		return err
	}
	b.isConnected = false
	return nil
}
