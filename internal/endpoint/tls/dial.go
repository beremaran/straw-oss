// Package tls provides TLS fingerprinting utilities for the endpoint.
// It uses the utls library to spoof browser TLS fingerprints when connecting
// to target websites.
package tls

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/metrics"
	utls "github.com/refraction-networking/utls"
)

// DialOptions configures the TLS dial behavior.
type DialOptions struct {
	// InsecureSkipVerify skips TLS certificate verification.
	// Use with caution - only for targets with self-signed certs.
	InsecureSkipVerify bool

	// ServerName overrides the SNI hostname sent in the ClientHello.
	// If empty, it's extracted from the address.
	ServerName string

	// Timeout for the TLS handshake. If zero, uses context deadline.
	HandshakeTimeout time.Duration
}

// DialOption is a functional option for configuring TLS dial.
type DialOption func(*DialOptions)

// WithInsecureSkipVerify returns an option to skip certificate verification.
func WithInsecureSkipVerify(skip bool) DialOption {
	return func(o *DialOptions) {
		o.InsecureSkipVerify = skip
	}
}

// WithServerName returns an option to override the SNI hostname.
func WithServerName(name string) DialOption {
	return func(o *DialOptions) {
		o.ServerName = name
	}
}

// WithHandshakeTimeout returns an option to set a handshake timeout.
func WithHandshakeTimeout(d time.Duration) DialOption {
	return func(o *DialOptions) {
		o.HandshakeTimeout = d
	}
}

// sessionCache provides TLS session resumption support.
var (
	sessionCache     = utls.NewLRUClientSessionCache(1000)
	sessionCacheInit sync.Once
)

// getSessionCache returns the shared session cache for TLS resumption.
func getSessionCache() utls.ClientSessionCache {
	sessionCacheInit.Do(func() {
		sessionCache = utls.NewLRUClientSessionCache(1000)
	})
	return sessionCache
}

// Dial establishes a TLS connection to the specified address using the given
// fingerprint preset. It returns a net.Conn on success or a wrapped error
// indicating the failure type.
//
// The fingerprint parameter should match one of the preset IDs from GetPreset().
// If the fingerprint is not recognized, ErrUnknownFingerprint is returned.
//
// Example:
//
//	conn, err := tls.Dial(ctx, "tcp", "example.com:443", "chrome-130")
//	if err != nil {
//	    if errors.Is(err, tls.ErrCertificateValidation) {
//	        // Handle certificate error
//	    }
//	    return err
//	}
//	defer conn.Close()
func Dial(ctx context.Context, network, addr string, fingerprint string, opts ...DialOption) (net.Conn, error) {
	// Apply options
	options := &DialOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Look up fingerprint preset
	clientHelloID, ok := GetPreset(fingerprint)
	if !ok {
		return nil, &FingerprintError{
			Fingerprint: fingerprint,
			Err:         ErrUnknownFingerprint,
		}
	}

	// Extract host for SNI
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Assume it's just a host without port
		host = addr
	}
	if options.ServerName != "" {
		host = options.ServerName
	}

	// Establish TCP connection with context
	var d net.Dialer
	rawConn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, &DialError{
			Addr: addr,
			Err:  err,
		}
	}

	// Set up TLS config
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: options.InsecureSkipVerify, //nolint:gosec // Configurable by caller
	}

	// Create utls client with specified fingerprint
	uConn := utls.UClient(rawConn, &utls.Config{
		ServerName:         tlsConfig.ServerName,
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
		ClientSessionCache: getSessionCache(),
	}, clientHelloID)

	// Apply handshake timeout if specified
	if options.HandshakeTimeout > 0 {
		deadline := time.Now().Add(options.HandshakeTimeout)
		if err := uConn.SetDeadline(deadline); err != nil {
			_ = rawConn.Close()
			return nil, &HandshakeError{
				Addr: addr,
				Err:  err,
			}
		}
	}

	// Perform TLS handshake
	if err := uConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, classifyHandshakeError(addr, err)
	}

	// Clear deadline after successful handshake
	if options.HandshakeTimeout > 0 {
		if err := uConn.SetDeadline(time.Time{}); err != nil {
			_ = uConn.Close()
			return nil, err
		}
	}

	// Record metric
	metrics.TLSFingerprintUsed.WithLabelValues(fingerprint).Inc()

	return uConn, nil
}

// classifyHandshakeError categorizes the handshake error into a specific error type.
func classifyHandshakeError(addr string, err error) error {
	// Check for context errors first
	if err == context.DeadlineExceeded || err == context.Canceled {
		return &HandshakeError{
			Addr: addr,
			Err:  ErrHandshakeTimeout,
		}
	}

	// Check for certificate errors
	if isCertificateError(err) {
		return &CertificateError{
			Addr: addr,
			Err:  err,
		}
	}

	// Check for protocol negotiation errors
	if isProtocolError(err) {
		return &ProtocolError{
			Addr: addr,
			Err:  err,
		}
	}

	// Generic handshake error
	return &HandshakeError{
		Addr: addr,
		Err:  err,
	}
}

// isCertificateError checks if the error is related to certificate validation.
func isCertificateError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Common certificate error patterns
	return contains(errStr, "certificate") ||
		contains(errStr, "x509") ||
		contains(errStr, "verify") ||
		contains(errStr, "expired") ||
		contains(errStr, "untrusted")
}

// isProtocolError checks if the error is related to protocol negotiation.
func isProtocolError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Common protocol error patterns
	return contains(errStr, "protocol") ||
		contains(errStr, "ALPN") ||
		contains(errStr, "version") ||
		contains(errStr, "cipher") ||
		contains(errStr, "handshake")
}

// contains checks if s contains substr (case-insensitive would be better but keeping simple).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
