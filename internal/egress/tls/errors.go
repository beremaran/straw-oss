package tls

import (
	"errors"
	"fmt"
)

var (
	// ErrUnknownFingerprint indicates a fingerprint preset was not recognized.
	ErrUnknownFingerprint = errors.New("unknown fingerprint preset")

	// ErrHandshakeTimeout indicates the TLS handshake exceeded the configured timeout.
	ErrHandshakeTimeout = errors.New("TLS handshake timeout")

	// ErrCertificateValidation indicates a certificate validation failure.
	ErrCertificateValidation = errors.New("certificate validation failed")

	// ErrProtocolNegotiation indicates a TLS protocol negotiation failure.
	ErrProtocolNegotiation = errors.New("protocol negotiation failed")
)

// FingerprintError indicates a fingerprint preset was not recognized.
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

// DialError indicates a failure to establish a TCP connection.
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

// HandshakeError indicates a TLS handshake failure.
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

// CertificateError indicates a certificate validation failure.
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

// Is implements the error interface for CertificateError.
func (e *CertificateError) Is(target error) bool {
	return target == ErrCertificateValidation
}

// ProtocolError indicates a TLS protocol negotiation failure.
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

// Is implements the error interface for ProtocolError.
func (e *ProtocolError) Is(target error) bool {
	return target == ErrProtocolNegotiation
}
