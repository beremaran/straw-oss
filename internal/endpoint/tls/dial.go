package tls

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/internal/endpoint/metrics"
)

// DialOptions configures the behavior of TLS dial operations.
type DialOptions struct {
	InsecureSkipVerify bool
	ServerName         string
	HandshakeTimeout   time.Duration
}

// DialOption is a function that modifies DialOptions.
type DialOption func(*DialOptions)

// WithInsecureSkipVerify configures the dialer to skip TLS certificate verification.
func WithInsecureSkipVerify(skip bool) DialOption {
	return func(o *DialOptions) {
		o.InsecureSkipVerify = skip
	}
}

// WithServerName sets the SNI server name to use during the TLS handshake.
func WithServerName(name string) DialOption {
	return func(o *DialOptions) {
		o.ServerName = name
	}
}

// WithHandshakeTimeout sets the maximum duration for the TLS handshake.
func WithHandshakeTimeout(d time.Duration) DialOption {
	return func(o *DialOptions) {
		o.HandshakeTimeout = d
	}
}

const defaultSessionCacheSize = 1000

var (
	sessionCache     = utls.NewLRUClientSessionCache(defaultSessionCacheSize)
	sessionCacheInit sync.Once
)

func getSessionCache() utls.ClientSessionCache {
	sessionCacheInit.Do(func() {
		sessionCache = utls.NewLRUClientSessionCache(defaultSessionCacheSize)
	})

	return sessionCache
}

// Dial connects to the given address using a TLS connection with the specified preset.
func Dial(ctx context.Context, network, addr string, presetID string, opts ...DialOption) (net.Conn, error) {
	options := newDialOptions(opts)

	presetObj, ok := fingerprint.Get(presetID)
	if !ok {
		return nil, &FingerprintError{
			Fingerprint: presetID,
			Err:         ErrUnknownFingerprint,
		}
	}

	clientHelloID := presetObj.TLSClientHello

	host := serverName(addr, options.ServerName)

	var d net.Dialer

	rawConn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, &DialError{
			Addr: addr,
			Err:  err,
		}
	}

	uConn := utls.UClient(rawConn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: options.InsecureSkipVerify,
		ClientSessionCache: getSessionCache(),
	}, clientHelloID)

	err = setHandshakeDeadline(uConn, options.HandshakeTimeout)
	if err != nil {
		_ = rawConn.Close()

		return nil, &HandshakeError{
			Addr: addr,
			Err:  err,
		}
	}

	err = uConn.HandshakeContext(ctx)
	if err != nil {
		_ = rawConn.Close()

		return nil, classifyHandshakeError(addr, err)
	}

	err = clearHandshakeDeadline(uConn, options.HandshakeTimeout)
	if err != nil {
		_ = uConn.Close()

		return nil, err
	}

	metrics.TLSFingerprintUsed.WithLabelValues(presetID).Inc()

	return uConn, nil
}

func newDialOptions(opts []DialOption) *DialOptions {
	options := &DialOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return options
}

func serverName(addr, override string) string {
	if override != "" {
		return override
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}

	return host
}

func setHandshakeDeadline(conn net.Conn, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}

	err := conn.SetDeadline(time.Now().Add(timeout))
	if err != nil {
		return fmt.Errorf("set handshake deadline: %w", err)
	}

	return nil
}

func clearHandshakeDeadline(conn net.Conn, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}

	err := conn.SetDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("clear handshake deadline: %w", err)
	}

	return nil
}

func classifyHandshakeError(addr string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &HandshakeError{
			Addr: addr,
			Err:  ErrHandshakeTimeout,
		}
	}

	if isCertificateError(err) {
		return &CertificateError{
			Addr: addr,
			Err:  err,
		}
	}

	if isProtocolError(err) {
		return &ProtocolError{
			Addr: addr,
			Err:  err,
		}
	}

	return &HandshakeError{
		Addr: addr,
		Err:  err,
	}
}

func isCertificateError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	return strings.Contains(errStr, "certificate") ||
		strings.Contains(errStr, "x509") ||
		strings.Contains(errStr, "verify") ||
		strings.Contains(errStr, "expired") ||
		strings.Contains(errStr, "untrusted")
}

func isProtocolError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	return strings.Contains(errStr, "protocol") ||
		strings.Contains(errStr, "ALPN") ||
		strings.Contains(errStr, "version") ||
		strings.Contains(errStr, "cipher") ||
		strings.Contains(errStr, "handshake")
}
