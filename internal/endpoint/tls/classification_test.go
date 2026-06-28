package tls

import (
	"crypto/x509"
	"errors"
	"fmt"
	"testing"
)

var (
	testErrTLSNoApplicationProtocol = errors.New("tls: no application protocol")
	testErrTLSProtocolVersion       = errors.New("tls: protocol version not supported")
	testErrConnectionReset          = errors.New("connection reset")
	testErrX509UnknownAuthority     = errors.New("x509: certificate signed by unknown authority")
	testErrX509Expired              = errors.New("x509: certificate has expired")
	testErrGeneric                  = errors.New("generic error")
	testErrNoCipherSuite            = errors.New("tls: no cipher suite supported")
	testErrHandshakeFailure         = errors.New("tls: handshake failure")
	testErrALPNFailed               = errors.New("ALPN negotiation failed")
	testErrSomeOther                = errors.New("some other error")
)

func TestClassifyHandshakeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{
			name:     "Certificate Invalid",
			err:      x509.CertificateInvalidError{},
			expected: ErrCertificateValidation,
		},
		{
			name:     "Unknown Authority",
			err:      x509.UnknownAuthorityError{},
			expected: ErrCertificateValidation,
		},
		{
			name:     "Hostname Mismatch",
			err:      x509.HostnameError{Certificate: &x509.Certificate{}, Host: "example.com"},
			expected: ErrCertificateValidation,
		},
		{
			name:     "Protocol Error - ALPN",
			err:      fmt.Errorf("test: %w", testErrTLSNoApplicationProtocol),
			expected: ErrProtocolNegotiation,
		},
		{
			name:     "Protocol Error - Version",
			err:      fmt.Errorf("test: %w", testErrTLSProtocolVersion),
			expected: ErrProtocolNegotiation,
		},
		{
			name:     "Generic Dial Error",
			err:      fmt.Errorf("test: %w", testErrConnectionReset),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := "example.com:443"
			classified := classifyHandshakeError(addr, tt.err)

			if tt.expected != nil {
				if !errors.Is(classified, tt.expected) {
					t.Errorf("classifyHandshakeError(%v) = %v; want %v", tt.err, classified, tt.expected)
				}
			} else {
				var hsErr *HandshakeError
				if !errors.As(classified, &hsErr) {
					t.Errorf("expected HandshakeError, got %T", classified)
				}

				if errors.Is(classified, ErrCertificateValidation) || errors.Is(classified, ErrProtocolNegotiation) {
					t.Errorf("unexpected specific error type for %v", classified)
				}
			}
		})
	}
}

func TestIsCertificateError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"x509.CertificateInvalidError", x509.CertificateInvalidError{}, true},
		{"x509.UnknownAuthorityError", x509.UnknownAuthorityError{}, true},
		{"x509.HostnameError", x509.HostnameError{Certificate: &x509.Certificate{}, Host: "example.com"}, true},
		{"x509.SystemRootsError", x509.SystemRootsError{}, true},
		{"String match: certificate signed by unknown authority", fmt.Errorf("test: %w", testErrX509UnknownAuthority), true},
		{"String match: certificate has expired", fmt.Errorf("test: %w", testErrX509Expired), true},
		{"Generic error", fmt.Errorf("test: %w", testErrGeneric), false},
		{"Nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCertificateError(tt.err); got != tt.expected {
				t.Errorf("isCertificateError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestIsProtocolError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"no application protocol", fmt.Errorf("test: %w", testErrTLSNoApplicationProtocol), true},
		{"no cipher suite supported", fmt.Errorf("test: %w", testErrNoCipherSuite), true},
		{"protocol version not supported", fmt.Errorf("test: %w", testErrTLSProtocolVersion), true},
		{"handshake failure", fmt.Errorf("test: %w", testErrHandshakeFailure), true},

		{"protocol version not supported", fmt.Errorf("test: %w", testErrTLSProtocolVersion), true},
		{"ALPN error", fmt.Errorf("test: %w", testErrALPNFailed), true},
		{"Cipher suite error", fmt.Errorf("test: %w", testErrNoCipherSuite), true},
		{"Generic error", fmt.Errorf("test: %w", testErrSomeOther), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProtocolError(tt.err); got != tt.expected {
				t.Errorf("isProtocolError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}
