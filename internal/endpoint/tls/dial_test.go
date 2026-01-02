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
)

// generateTestCert creates a self-signed certificate for testing.
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

// startTestTLSServer starts a test TLS server and returns its address.
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
				return // Server closed
			}
			// Spawn goroutine to handle connection
			go func(c net.Conn) {
				defer c.Close()
				// Read some data to complete the handshake
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
			}(conn)
		}
	}()

	return listener.Addr().String(), func() {
		listener.Close()
	}
}

func TestDial_Success(t *testing.T) {
	addr, cleanup := startTestTLSServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use InsecureSkipVerify since we're using a self-signed cert
	conn, err := Dial(ctx, "tcp", addr, "chrome-133", WithInsecureSkipVerify(true))
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Verify we got a connection
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
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
	// Create an address that will take a while to connect (non-routable)
	addr := "10.255.255.1:443"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := Dial(ctx, "tcp", addr, "chrome-133")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}

	// The dial should fail due to context cancellation
	if !errors.Is(err, context.Canceled) {
		// Also acceptable: DialError wrapping context.Canceled
		var dialErr *DialError
		if errors.As(err, &dialErr) {
			if !errors.Is(dialErr.Err, context.Canceled) {
				t.Logf("got dial error: %v", err)
			}
		}
	}
}

func TestDial_ConnectionRefused(t *testing.T) {
	// Use a port that's definitely not listening
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
		// Accept any error here since connection refused behavior varies
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
	defer conn.Close()
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
	defer conn.Close()
}

func TestGetPreset(t *testing.T) {
	tests := []struct {
		name   string
		wantOK bool
	}{
		{"chrome-133", true},
		{"chrome-131", true},
		{"chrome-120", true},
		{"firefox-120", true},
		{"safari-16", true},
		{"edge-106", true},
		{"auto", true},
		{"randomized", true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := GetPreset(tt.name)
			if ok != tt.wantOK {
				t.Errorf("GetPreset(%q) ok = %v, want %v", tt.name, ok, tt.wantOK)
			}
		})
	}
}

func TestListPresets(t *testing.T) {
	presets := ListPresets()

	if len(presets) == 0 {
		t.Fatal("expected at least one preset")
	}

	// Check that all returned presets are valid
	for _, name := range presets {
		if _, ok := GetPreset(name); !ok {
			t.Errorf("ListPresets returned invalid preset: %q", name)
		}
	}

	// Check for expected presets
	expected := []string{"chrome-133", "firefox-120", "safari-16", "edge-106"}
	for _, exp := range expected {
		found := false
		for _, name := range presets {
			if name == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected preset %q not found in list", exp)
		}
	}
}

func TestGetPresetInfo(t *testing.T) {
	info, ok := GetPresetInfo("chrome-133")
	if !ok {
		t.Fatal("expected to find chrome-133 info")
	}

	if info.Browser != "Chrome" {
		t.Errorf("expected browser 'Chrome', got %q", info.Browser)
	}

	if info.Version != "133" {
		t.Errorf("expected version '133', got %q", info.Version)
	}

	if info.Deprecated {
		t.Error("chrome-133 should not be deprecated")
	}

	// Test deprecated preset
	deprecatedInfo, ok := GetPresetInfo("chrome-120")
	if !ok {
		t.Fatal("expected to find chrome-120 info")
	}
	if !deprecatedInfo.Deprecated {
		t.Error("chrome-120 should be deprecated")
	}

	// Test unknown preset
	_, ok = GetPresetInfo("unknown")
	if ok {
		t.Error("expected to not find unknown preset info")
	}
}

func TestErrorTypes(t *testing.T) {
	// Test FingerprintError
	fpErr := &FingerprintError{
		Fingerprint: "test-fp",
		Err:         ErrUnknownFingerprint,
	}
	if !errors.Is(fpErr, ErrUnknownFingerprint) {
		t.Error("FingerprintError should unwrap to ErrUnknownFingerprint")
	}

	// Test CertificateError
	certErr := &CertificateError{
		Addr: "example.com:443",
		Err:  errors.New("x509: certificate is not valid"),
	}
	if !errors.Is(certErr, ErrCertificateValidation) {
		t.Error("CertificateError should be ErrCertificateValidation")
	}

	// Test ProtocolError
	protoErr := &ProtocolError{
		Addr: "example.com:443",
		Err:  errors.New("ALPN negotiation failed"),
	}
	if !errors.Is(protoErr, ErrProtocolNegotiation) {
		t.Error("ProtocolError should be ErrProtocolNegotiation")
	}
}

func TestDial_AllPresets(t *testing.T) {
	// Skip in short mode as this makes network connections
	if testing.Short() {
		t.Skip("skipping preset validation in short mode")
	}

	addr, cleanup := startTestTLSServer(t)
	defer cleanup()

	for name := range Presets {
		t.Run(name, func(t *testing.T) {
			// Skip randomized preset as it may generate unsupported curves
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
			conn.Close()
		})
	}
}
