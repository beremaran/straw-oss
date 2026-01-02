package tls

import (
	"errors"
	"fmt"
)

// Sentinel errors for TLS operations.
var (
	// ErrUnknownFingerprint is returned when an unrecognized fingerprint preset is requested.
	ErrUnknownFingerprint = errors.New("unknown fingerprint preset")

	// ErrHandshakeTimeout is returned when the TLS handshake times out.
	ErrHandshakeTimeout = errors.New("TLS handshake timeout")

	// ErrCertificateValidation is returned when TLS certificate validation fails.
	ErrCertificateValidation = errors.New("certificate validation failed")

	// ErrProtocolNegotiation is returned when ALPN or TLS version negotiation fails.
	ErrProtocolNegotiation = errors.New("protocol negotiation failed")
)

// FingerprintError represents an error related to fingerprint selection.
type FingerprintError struct {
	Fingerprint string
	Err         error
}

func (e *FingerprintError) Error() string {
	return fmt.Sprintf("fingerprint error for %q: %v", e.Fingerprint, e.Err)
}

func (e *FingerprintError) Unwrap() error {
	return e.Err
}

// DialError represents an error during TCP connection establishment.
type DialError struct {
	Addr string
	Err  error
}

func (e *DialError) Error() string {
	return fmt.Sprintf("dial error for %s: %v", e.Addr, e.Err)
}

func (e *DialError) Unwrap() error {
	return e.Err
}

// HandshakeError represents an error during TLS handshake.
type HandshakeError struct {
	Addr string
	Err  error
}

func (e *HandshakeError) Error() string {
	return fmt.Sprintf("TLS handshake error for %s: %v", e.Addr, e.Err)
}

func (e *HandshakeError) Unwrap() error {
	return e.Err
}

// CertificateError represents a certificate validation error.
type CertificateError struct {
	Addr string
	Err  error
}

func (e *CertificateError) Error() string {
	return fmt.Sprintf("certificate error for %s: %v", e.Addr, e.Err)
}

func (e *CertificateError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is for CertificateError.
func (e *CertificateError) Is(target error) bool {
	return target == ErrCertificateValidation
}

// ProtocolError represents a protocol negotiation error.
type ProtocolError struct {
	Addr string
	Err  error
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol negotiation error for %s: %v", e.Addr, e.Err)
}

func (e *ProtocolError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is for ProtocolError.
func (e *ProtocolError) Is(target error) bool {
	return target == ErrProtocolNegotiation
}
