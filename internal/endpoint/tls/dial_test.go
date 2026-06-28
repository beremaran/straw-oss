package tls

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
)

func generateTestCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}, nil
}

func startTestTLSServer(t *testing.T) (string, func()) {
	t.Helper()

	cert, err := generateTestCert()
	if err != nil {
		t.Fatalf("failed to generate test cert: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("failed to start TLS listener: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer func() { _ = c.Close() }()

				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
			}(conn)
		}
	}()

	return listener.Addr().String(), func() {
		_ = listener.Close()
	}
}

func TestDial_Success(t *testing.T) {
	addr, cleanup := startTestTLSServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, "tcp", addr, "chrome-133", WithInsecureSkipVerify(true))
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)
}

func TestDial_UnknownFingerprint(t *testing.T) {
	ctx := context.Background()

	_, err := Dial(ctx, "tcp", "example.com:443", "unknown-fingerprint")
	if err == nil {
		t.Fatal("expected error for unknown fingerprint")
	}

	var fpErr *FingerprintError
	if !errors.As(err, &fpErr) {
		t.Fatalf("expected FingerprintError, got %T", err)
	}

	if !errors.Is(fpErr, ErrUnknownFingerprint) {
		t.Errorf("expected ErrUnknownFingerprint, got %v", fpErr.Err)
	}

	if fpErr.Fingerprint != "unknown-fingerprint" {
		t.Errorf("expected fingerprint 'unknown-fingerprint', got %q", fpErr.Fingerprint)
	}
}

func TestDial_ContextCancellation(t *testing.T) {
	addr := "10.255.255.1:443"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Dial(ctx, "tcp", addr, "chrome-133")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}

	if !errors.Is(err, context.Canceled) {
		var dialErr *DialError
		if errors.As(err, &dialErr) {
			if !errors.Is(dialErr.Err, context.Canceled) {
				t.Logf("got dial error: %v", err)
			}
		}
	}
}

func TestDial_ConnectionRefused(t *testing.T) {
	addr := "127.0.0.1:1"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := Dial(ctx, "tcp", addr, "chrome-133")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}

	var dialErr *DialError
	if !errors.As(err, &dialErr) {
		t.Logf("got error type: %T, error: %v", err, err)
	}
}

func TestDial_WithServerName(t *testing.T) {
	addr, cleanup := startTestTLSServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, "tcp", addr, "chrome-133",
		WithInsecureSkipVerify(true),
		WithServerName("custom.example.com"),
	)
	if err != nil {
		t.Fatalf("Dial with ServerName failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
}

func TestDial_WithHandshakeTimeout(t *testing.T) {
	addr, cleanup := startTestTLSServer(t)
	defer cleanup()

	ctx := context.Background()

	conn, err := Dial(ctx, "tcp", addr, "chrome-133",
		WithInsecureSkipVerify(true),
		WithHandshakeTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial with HandshakeTimeout failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
}

func TestErrorTypes(t *testing.T) {
	fpErr := &FingerprintError{
		Fingerprint: "test-fp",
		Err:         ErrUnknownFingerprint,
	}
	if !errors.Is(fpErr, ErrUnknownFingerprint) {
		t.Error("FingerprintError should unwrap to ErrUnknownFingerprint")
	}

	certErr := &CertificateError{
		Addr: "example.com:443",
		Err:  errors.New("x509: certificate is not valid"),
	}
	if !errors.Is(certErr, ErrCertificateValidation) {
		t.Error("CertificateError should be ErrCertificateValidation")
	}

	protoErr := &ProtocolError{
		Addr: "example.com:443",
		Err:  errors.New("ALPN negotiation failed"),
	}
	if !errors.Is(protoErr, ErrProtocolNegotiation) {
		t.Error("ProtocolError should be ErrProtocolNegotiation")
	}
}

func TestDial_AllPresets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping preset validation in short mode")
	}

	addr, cleanup := startTestTLSServer(t)
	defer cleanup()

	for _, name := range fingerprint.List() {
		t.Run(name, func(t *testing.T) {
			if name == "randomized" {
				t.Skip("randomized preset may generate unsupported curves")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, err := Dial(ctx, "tcp", addr, name, WithInsecureSkipVerify(true))
			if err != nil {
				t.Errorf("Dial with preset %q failed: %v", name, err)

				return
			}
			_ = conn.Close()
		})
	}
}
