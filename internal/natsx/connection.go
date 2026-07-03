package natsx

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// Connection wraps a live NATS connection for Straw services.
type Connection struct {
	*nats.Conn
}

const (
	defaultReconnectWait = 2 * time.Second
	defaultPingInterval  = 30 * time.Second
)

var (
	errNATSConnectionRequired = errors.New("nats connection is required")
	errNATSMaxPayloadNegative = errors.New("nats max payload must be non-negative")
)

// ConnectOptions configures a Straw NATS client connection.
type ConnectOptions struct {
	Servers             []string
	UserCredentialsFile string
	ReconnectAttempts   int
	ReconnectWait       time.Duration
	PingInterval        time.Duration
	MaxPingFailures     int
}

// Connect dials the configured NATS servers and returns a live connection.
func Connect(opts ConnectOptions) (*Connection, error) {
	err := ValidateServers(opts.Servers)
	if err != nil {
		return nil, err
	}

	if opts.ReconnectAttempts <= 0 {
		opts.ReconnectAttempts = 10
	}

	if opts.ReconnectWait <= 0 {
		opts.ReconnectWait = defaultReconnectWait
	}

	if opts.PingInterval <= 0 {
		opts.PingInterval = defaultPingInterval
	}

	if opts.MaxPingFailures <= 0 {
		opts.MaxPingFailures = 3
	}

	natsOpts := []nats.Option{
		nats.DontRandomize(),
		nats.MaxReconnects(opts.ReconnectAttempts),
		nats.ReconnectWait(opts.ReconnectWait),
		nats.PingInterval(opts.PingInterval),
		nats.MaxPingsOutstanding(opts.MaxPingFailures),
	}

	if opts.UserCredentialsFile != "" {
		natsOpts = append(natsOpts, nats.UserCredentials(opts.UserCredentialsFile))
	}

	conn, err := nats.Connect(joinServers(opts.Servers), natsOpts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	return &Connection{Conn: conn}, nil
}

// ValidateConnectedMaxPayload checks the live server-advertised payload limit
// against Straw's configured frame and body limits.
func ValidateConnectedMaxPayload(conn maxPayloadProvider, maxFrameDataBytes, maxInlineRequestBodyBytes, maxInlineResponseBodyBytes uint64) error {
	if conn == nil {
		return errNATSConnectionRequired
	}

	maxPayload := conn.MaxPayload()
	if maxPayload < 0 {
		return fmt.Errorf("%w: %d", errNATSMaxPayloadNegative, maxPayload)
	}

	maxPayloadBytes := uint64(maxPayload)

	return ValidateMaxPayload(&maxPayloadBytes, maxFrameDataBytes, maxInlineRequestBodyBytes, maxInlineResponseBodyBytes)
}

type maxPayloadProvider interface {
	MaxPayload() int64
}

func joinServers(servers []string) string {
	return strings.Join(servers, ",")
}
