package broker

import (
	"crypto/tls"
	"time"

	"github.com/beremaran/straw/internal/infra/circuitbreaker"
)

// Options configures a NATS broker connection.
type Options struct {
	Addrs          []string
	Token          string
	Secure         bool
	TLSConfig      *tls.Config
	ReconnectWait  time.Duration
	PrefetchCount  int
	CircuitBreaker *circuitbreaker.CircuitBreaker
}

// Option sets an option on the broker configuration.
type Option func(*Options)

// Addrs sets the NATS server addresses.
func Addrs(addrs ...string) Option {
	return func(o *Options) {
		o.Addrs = addrs
	}
}

// Token sets the authentication token.
func Token(t string) Option {
	return func(o *Options) {
		o.Token = t
	}
}

// Secure enables TLS.
func Secure(b bool) Option {
	return func(o *Options) {
		o.Secure = b
	}
}

// TLSConfig sets a custom TLS configuration.
func TLSConfig(t *tls.Config) Option {
	return func(o *Options) {
		o.TLSConfig = t
	}
}

// ReconnectWait sets the time between reconnect attempts.
func ReconnectWait(d time.Duration) Option {
	return func(o *Options) {
		o.ReconnectWait = d
	}
}

// PrefetchCount sets the number of messages to prefetch on sync subscriptions.
func PrefetchCount(n int) Option {
	return func(o *Options) {
		o.PrefetchCount = n
	}
}
