package tls

import (
	"crypto/x509"
	"errors"
	"testing"
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
			err:      errors.New("tls: no application protocol"),
			expected: ErrProtocolNegotiation,
		},
		{
			name:     "Protocol Error - Version",
			err:      errors.New("tls: protocol version not supported"),
			expected: ErrProtocolNegotiation,
		},
		{
			name:     "Generic Dial Error",
			err:      errors.New("connection reset"),
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
		{"String match: certificate signed by unknown authority", errors.New("x509: certificate signed by unknown authority"), true},
		{"String match: certificate has expired", errors.New("x509: certificate has expired"), true},
		{"Generic error", errors.New("generic error"), false},
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
		{"no application protocol", errors.New("tls: no application protocol"), true},
		{"no cipher suite supported", errors.New("tls: no cipher suite supported"), true},
		{"protocol version not supported", errors.New("tls: protocol version not supported"), true},
		{"handshake failure", errors.New("tls: handshake failure"), true},

		{"protocol version not supported", errors.New("tls: protocol version not supported"), true},
		{"ALPN error", errors.New("ALPN negotiation failed"), true},
		{"Cipher suite error", errors.New("no cipher suite supported"), true},
		{"Generic error", errors.New("some other error"), false},
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
