package tls

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/internal/endpoint/metrics"
	utls "github.com/refraction-networking/utls"
)

type DialOptions struct {
	InsecureSkipVerify bool

	ServerName string

	HandshakeTimeout time.Duration
}

type DialOption func(*DialOptions)

func WithInsecureSkipVerify(skip bool) DialOption {
	return func(o *DialOptions) {
		o.InsecureSkipVerify = skip
	}
}

func WithServerName(name string) DialOption {
	return func(o *DialOptions) {
		o.ServerName = name
	}
}

func WithHandshakeTimeout(d time.Duration) DialOption {
	return func(o *DialOptions) {
		o.HandshakeTimeout = d
	}
}

var (
	sessionCache     = utls.NewLRUClientSessionCache(1000)
	sessionCacheInit sync.Once
)

func getSessionCache() utls.ClientSessionCache {
	sessionCacheInit.Do(func() {
		sessionCache = utls.NewLRUClientSessionCache(1000)
	})

	return sessionCache
}

//nolint:cyclop,funlen
func Dial(ctx context.Context, network, addr string, presetID string, opts ...DialOption) (net.Conn, error) {
	options := &DialOptions{}
	for _, opt := range opts {
		opt(options)
	}

	presetObj, ok := fingerprint.Get(presetID)
	if !ok {
		return nil, &FingerprintError{
			Fingerprint: presetID,
			Err:         ErrUnknownFingerprint,
		}
	}
	clientHelloID := presetObj.TLSClientHello

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if options.ServerName != "" {
		host = options.ServerName
	}

	var d net.Dialer
	rawConn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, &DialError{
			Addr: addr,
			Err:  err,
		}
	}

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: options.InsecureSkipVerify,
	}

	uConn := utls.UClient(rawConn, &utls.Config{
		ServerName:         tlsConfig.ServerName,
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
		ClientSessionCache: getSessionCache(),
	}, clientHelloID)

	if options.HandshakeTimeout > 0 {
		deadline := time.Now().Add(options.HandshakeTimeout)
		err := uConn.SetDeadline(deadline)
		if err != nil {
			_ = rawConn.Close()

			return nil, &HandshakeError{
				Addr: addr,
				Err:  err,
			}
		}
	}

	err = uConn.HandshakeContext(ctx)
	if err != nil {
		_ = rawConn.Close()

		return nil, classifyHandshakeError(addr, err)
	}

	if options.HandshakeTimeout > 0 {
		err := uConn.SetDeadline(time.Time{})
		if err != nil {
			_ = uConn.Close()

			return nil, err
		}
	}

	metrics.TLSFingerprintUsed.WithLabelValues(presetID).Inc()

	return uConn, nil
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
