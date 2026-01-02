package broker

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
)

// AMQPHeaderCarrier implements propagation.TextMapCarrier for AMQP headers.
type AMQPHeaderCarrier amqp.Table

// Get returns the value associated with the passed key.
func (c AMQPHeaderCarrier) Get(key string) string {
	val, ok := c[key]
	if !ok {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

// Set stores the key-value pair.
func (c AMQPHeaderCarrier) Set(key string, value string) {
	c[key] = value
}

// Keys lists the keys stored in this carrier.
func (c AMQPHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// Inject injects the tracing context into the AMQP headers.
func Inject(ctx context.Context, headers amqp.Table) {
	otel.GetTextMapPropagator().Inject(ctx, AMQPHeaderCarrier(headers))
}

// Extract extracts the tracing context from the AMQP headers.
func Extract(ctx context.Context, headers amqp.Table) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, AMQPHeaderCarrier(headers))
}
