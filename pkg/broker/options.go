package broker

import (
	"crypto/tls"
	"time"

	"github.com/beremaran/straw/internal/infra/circuitbreaker"
)

type Options struct {
	Addrs          []string
	Token          string
	Secure         bool
	TLSConfig      *tls.Config
	ReconnectWait  time.Duration
	PrefetchCount  int
	CircuitBreaker *circuitbreaker.CircuitBreaker
}

type Option func(*Options)

func Addrs(addrs ...string) Option {
	return func(o *Options) {
		o.Addrs = addrs
	}
}

func Token(t string) Option {
	return func(o *Options) {
		o.Token = t
	}
}

func Secure(b bool) Option {
	return func(o *Options) {
		o.Secure = b
	}
}

func TLSConfig(t *tls.Config) Option {
	return func(o *Options) {
		o.TLSConfig = t
	}
}

func ReconnectWait(d time.Duration) Option {
	return func(o *Options) {
		o.ReconnectWait = d
	}
}

func PrefetchCount(n int) Option {
	return func(o *Options) {
		o.PrefetchCount = n
	}
}
