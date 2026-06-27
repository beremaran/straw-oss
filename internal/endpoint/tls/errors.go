package tls

import (
	"errors"
	"fmt"
)

var (
	ErrUnknownFingerprint = errors.New("unknown fingerprint preset")

	ErrHandshakeTimeout = errors.New("TLS handshake timeout")

	ErrCertificateValidation = errors.New("certificate validation failed")

	ErrProtocolNegotiation = errors.New("protocol negotiation failed")
)

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

func (e *CertificateError) Is(target error) bool {
	return target == ErrCertificateValidation
}

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

func (e *ProtocolError) Is(target error) bool {
	return target == ErrProtocolNegotiation
}
