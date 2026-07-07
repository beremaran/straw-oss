package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

const (
	testControlHost  = "127.0.0.1"
	testMITMLeafHost = "example.com"
)

func TestOpenRedisMissingURLEnvFails(t *testing.T) {
	t.Setenv("STRAW_TEST_MAIN_REDIS_URL_UNSET", "")

	_, err := openRedis(config.RedisConfig{URLEnv: "STRAW_TEST_MAIN_REDIS_URL_UNSET"})
	if err == nil {
		t.Fatal("openRedis() error = nil, want error for unset STRAW_REDIS_URL")
	}
}

func TestOpenRedisInvalidURLFails(t *testing.T) {
	t.Setenv("STRAW_TEST_MAIN_REDIS_URL", "not-a-valid-redis-url")

	_, err := openRedis(config.RedisConfig{URLEnv: "STRAW_TEST_MAIN_REDIS_URL"})
	if err == nil {
		t.Fatal("openRedis() error = nil, want error for malformed url")
	}
}

// TestOpenRedisUnreachableStillReturnsClient proves a configured-but-down
// Redis does not fail Control startup (docs/planning/29 "Redis unavailable:
// Apply configured fail policy"); only a bad/missing URL does.
func TestOpenRedisUnreachableStillReturnsClient(t *testing.T) {
	t.Setenv("STRAW_TEST_MAIN_REDIS_URL", "redis://127.0.0.1:1/0")

	client, err := openRedis(config.RedisConfig{URLEnv: "STRAW_TEST_MAIN_REDIS_URL", DialTimeoutMS: 50})
	if err != nil {
		t.Fatalf("openRedis() error = %v, want nil for an unreachable-but-configured redis", err)
	}
	defer func() { _ = client.Close() }()

	pingCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pingErr := client.Ping(pingCtx).Err()
	if pingErr == nil {
		t.Fatal("client.Ping() error = nil, want error against an unreachable address")
	}
}

func TestBuildProxyHandlerOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.ControlConfig{
		Server: config.ControlServerConfig{Host: testControlHost, APIPort: 8080, MetricsPort: 9090},
	}
	if got := buildProxyHandler(cfg, nil, nil, nil, nil); got != nil {
		t.Fatal("buildProxyHandler disabled = non-nil, want nil")
	}

	cfg.Server.ProxyEnabled = true
	cfg.Server.ProxyPort = 8081
	if got := buildProxyHandler(cfg, nil, nil, nil, nil); got == nil {
		t.Fatal("buildProxyHandler enabled = nil, want handler")
	}
}

func TestBuildConnectHandlerOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.ControlConfig{
		Server: config.ControlServerConfig{Host: testControlHost, APIPort: 8080, MetricsPort: 9090},
	}
	if got := buildConnectHandler(cfg, nil, nil); got != nil {
		t.Fatal("buildConnectHandler disabled = non-nil, want nil")
	}

	cfg.Server.ConnectEnabled = true
	cfg.Server.ConnectPort = 8082
	if got := buildConnectHandler(cfg, nil, nil); got == nil {
		t.Fatal("buildConnectHandler enabled = nil, want handler")
	}
}

func TestBuildMITMHandlerOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.ControlConfig{
		Server: config.ControlServerConfig{Host: testControlHost, APIPort: 8080, MetricsPort: 9090},
	}
	if got := buildMITMHandler(cfg, nil, nil, nil, nil); got != nil {
		t.Fatal("buildMITMHandler disabled = non-nil, want nil")
	}

	cfg.Server.MITMEnabled = true
	cfg.Server.MITMPort = 8083
	if got := buildMITMHandler(cfg, nil, nil, nil, nil); got == nil {
		t.Fatal("buildMITMHandler enabled = nil, want handler")
	}
}

func TestBuildMITMLeafBundleProviderConfig(t *testing.T) {
	t.Parallel()

	cfg := config.ControlConfig{}
	got, err := buildMITMLeafBundleProviderConfig(cfg)
	if err != nil || got != nil {
		t.Fatalf("buildMITMLeafBundleProviderConfig(empty) = %+v, %v; want nil, nil", got, err)
	}

	cfg.Server.MITMLeafKMSProvider = "aws-kms"
	cfg.Server.MITMLeafKMSKeyID = "arn:test"
	got, err = buildMITMLeafBundleProviderConfig(cfg)
	if err != nil {
		t.Fatalf("buildMITMLeafBundleProviderConfig() error = %v", err)
	}
	if got.ProviderName != "aws-kms" || got.KeyID != "arn:test" {
		t.Fatalf("provider config = %+v", got)
	}
}

func TestGenerateMITMLeafSignsServerCertificate(t *testing.T) {
	t.Parallel()

	ca := newTestMITMCA(t)
	cert, err := generateMITMLeaf(ca, testMITMLeafHost)
	if err != nil {
		t.Fatalf("generateMITMLeaf() error = %v", err)
	}
	if len(cert.Certificate) != 2 {
		t.Fatalf("certificate chain length = %d, want leaf plus ca", len(cert.Certificate))
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != testMITMLeafHost {
		t.Fatalf("DNSNames = %v, want %s", leaf.DNSNames, testMITMLeafHost)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatalf("ExtKeyUsage = %v, want server auth", leaf.ExtKeyUsage)
	}

	ipCert, err := generateMITMLeaf(ca, testControlHost)
	if err != nil {
		t.Fatalf("generateMITMLeaf(ip) error = %v", err)
	}
	ipLeaf, err := x509.ParseCertificate(ipCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse ip leaf: %v", err)
	}
	if len(ipLeaf.IPAddresses) != 1 || ipLeaf.IPAddresses[0].String() != testControlHost {
		t.Fatalf("IPAddresses = %v, want %s", ipLeaf.IPAddresses, testControlHost)
	}
}

func TestMITMServerTerminatesTLSAndShutsDown(t *testing.T) {
	t.Parallel()

	ca := newTestMITMCA(t)
	server := &http.Server{ReadHeaderTimeout: readHeaderTimeout}
	configureMITMServer(server, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Error("request TLS state is nil")
		}
		_, _ = w.Write([]byte("ok"))
	}), ca)

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", testControlHost+":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ServeTLS(listener, "", "")
	}()

	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: testMITMLeafHost}}}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+listener.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET MITM server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("ServeTLS() error = %v, want ErrServerClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ServeTLS did not return after shutdown")
	}
}

func newTestMITMCA(t *testing.T) *mitmCA {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Straw Test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}

	return &mitmCA{cert: cert, key: key}
}
