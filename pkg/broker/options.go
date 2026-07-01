package broker

// Options configures a NATS broker connection.
type Options struct {
	Addrs []string
	Token string
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
