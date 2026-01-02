package broker

import (
	"crypto/tls"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
)

// Options contains configuration for the broker.
type Options struct {
	Addrs          []string
	Secure         bool
	TLSConfig      *tls.Config
	ReconnectWait  time.Duration
	PrefetchCount  int // QoS prefetch count for consumers
	CircuitBreaker *circuitbreaker.CircuitBreaker
}

// Option is a functional option for configuring the broker.
type Option func(*Options)

// Addrs sets the broker addresses.
func Addrs(addrs ...string) Option {
	return func(o *Options) {
		o.Addrs = addrs
	}
}

// Secure sets whether to use TLS.
func Secure(b bool) Option {
	return func(o *Options) {
		o.Secure = b
	}
}

// TLSConfig sets the TLS configuration.
func TLSConfig(t *tls.Config) Option {
	return func(o *Options) {
		o.TLSConfig = t
	}
}

// ReconnectWait sets the duration to wait before reconnecting.
func ReconnectWait(d time.Duration) Option {
	return func(o *Options) {
		o.ReconnectWait = d
	}
}

// PrefetchCount sets the QoS prefetch count for consumers.
// This limits the number of unacknowledged messages per consumer.
func PrefetchCount(n int) Option {
	return func(o *Options) {
		o.PrefetchCount = n
	}
}
