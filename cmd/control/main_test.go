package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
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

	cfg.Server.MITMLeafKMSProvider = mitmLeafKMSProviderAWS
	cfg.Server.MITMLeafKMSKeyID = "arn:test"
	got, err = buildMITMLeafBundleProviderConfig(cfg)
	if err != nil {
		t.Fatalf("buildMITMLeafBundleProviderConfig() error = %v", err)
	}
	if got.ProviderName != mitmLeafKMSProviderAWS || got.KeyID != "arn:test" {
		t.Fatalf("provider config = %+v", got)
	}
}

func TestBuildMITMLeafBundleProvider(t *testing.T) {
	t.Parallel()

	cfg := config.ControlConfig{}
	cfg.Server.MITMLeafKMSProvider = mitmLeafKMSProviderAWS
	cfg.Server.MITMLeafKMSKeyID = "arn:aws:kms:us-west-2:123:key/abc"

	providerConfig, provider, err := buildMITMLeafBundleProvider(cfg)
	if err != nil {
		t.Fatalf("buildMITMLeafBundleProvider() error = %v", err)
	}
	if providerConfig == nil || providerConfig.ProviderName != mitmLeafKMSProviderAWS || provider == nil {
		t.Fatalf("provider = (%+v, %T), want aws provider", providerConfig, provider)
	}

	cfg.Server.MITMLeafKMSProvider = "other-kms"
	_, _, err = buildMITMLeafBundleProvider(cfg)
	if err == nil {
		t.Fatal("buildMITMLeafBundleProvider(unsupported) error = nil")
	}
}

func TestGenerateMITMLeafSignsServerCertificate(t *testing.T) {
	t.Parallel()

	ca := newTestMITMCA(t)
	cert, err := generateMITMLeaf(ca, testMITMLeafHost, 14*24*time.Hour)
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
	if leaf.NotAfter.Sub(leaf.NotBefore) > 15*24*time.Hour {
		t.Fatalf("leaf validity = %v, want capped near 14 days", leaf.NotAfter.Sub(leaf.NotBefore))
	}

	ipCert, err := generateMITMLeaf(ca, testControlHost, time.Hour)
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

func TestConfigureMITMServerUsesAuthenticatedConnectBootstrap(t *testing.T) {
	t.Parallel()

	ca := newTestMITMCA(t)
	mitm, token := newTestMainMITMHandler(t)
	leafLookup := func(_ *http.Request, _ control.Identity, sni, _ string) (*tls.Certificate, error) {
		return generateMITMLeaf(ca, sni, time.Hour)
	}
	server := httptest.NewServer(configureMITMServer(mitm, leafLookup, nil))
	defer server.Close()

	directTLS, err := (&tls.Dialer{Config: &tls.Config{RootCAs: x509.NewCertPool(), ServerName: testMITMLeafHost}}).DialContext(context.Background(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	if err == nil {
		_ = directTLS.Close()
		t.Fatal("direct TLS to MITM port succeeded, want CONNECT bootstrap")
	}

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err = io.WriteString(conn, "CONNECT "+testMITMLeafHost+":443 HTTP/1.1\r\nHost: "+testMITMLeafHost+":443\r\nProxy-Authorization: Bearer "+token+"\r\n\r\n")
	if err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("read CONNECT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want 200", resp.StatusCode)
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	tlsConn := tls.Client(&testBufferedConn{Conn: conn, r: br}, &tls.Config{RootCAs: pool, ServerName: testMITMLeafHost})
	err = tlsConn.HandshakeContext(context.Background())
	if err != nil {
		t.Fatalf("inner TLS handshake: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+testMITMLeafHost+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Host = testMITMLeafHost + ":443"
	err = req.Write(tlsConn)
	if err != nil {
		t.Fatalf("write inner request: %v", err)
	}
	innerResp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		t.Fatalf("read inner response: %v", err)
	}
	defer func() { _ = innerResp.Body.Close() }()
	if innerResp.StatusCode != http.StatusOK {
		t.Fatalf("inner status = %d, want 200", innerResp.StatusCode)
	}
}

func TestBuildMITMLeafLookupRequiresTenantIdentity(t *testing.T) {
	t.Parallel()

	ca := newTestMITMCA(t)
	cfg := config.ControlConfig{
		DeploymentID: "dep_main_test",
		Server: config.ControlServerConfig{
			MITMCertValidityDays: 1,
			MITMLeafKMSProvider:  mitmLeafKMSProviderAWS,
			MITMLeafKMSKeyID:     "arn:aws:kms:us-west-2:123:key/abc",
		},
	}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	lookup, err := buildMITMLeafLookup(cfg, ca, client, mainTestKMSProvider{})
	if err != nil {
		t.Fatalf("buildMITMLeafLookup() error = %v", err)
	}

	_, err = lookup(httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://proxy.test", nil), control.Identity{}, testMITMLeafHost, testMITMLeafHost+":443")
	if err == nil {
		t.Fatal("lookup without tenant identity error = nil")
	}
}

type testBufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *testBufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func newTestMainMITMHandler(t *testing.T) (*control.MITMHandler, string) {
	t.Helper()

	store := control.NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	generated, err := control.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	err = store.Create(context.Background(), control.APIKeyRecord{
		ID:         "key_main_mitm",
		ScopeType:  control.ScopeTenant,
		TenantID:   "ten_main_mitm",
		Role:       control.RoleRequester,
		Prefix:     generated.Prefix,
		SecretHash: control.HashAPIKeySecret(generated.Secret, pepper),
		Status:     control.APIKeyStatusActive,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	h := control.NewMITMHandler(1_048_576, 1_048_576, 120_000, control.NewAuthenticator(store, pepper))
	h.SetDispatcher(testMainMITMDispatcher{})

	return h, generated.Secret
}

type testMainMITMDispatcher struct{}

func (testMainMITMDispatcher) Dispatch(context.Context, control.DispatchInput) (control.SuccessResponse, *control.PipelineError) {
	return control.SuccessResponse{
		Status:  http.StatusOK,
		Headers: []control.HeaderPair{{Name: "Content-Type", Value: "text/plain"}},
		Body:    control.ResponseBody{Mode: "inline_base64", DataBase64: "b2s="},
	}, nil
}

type mainTestKMSProvider struct{}

func (mainTestKMSProvider) EncryptMITMLeafBundle(context.Context, string, control.MITMLeafBundleAAD, []byte) (control.MITMLeafBundleEnvelope, error) {
	return control.MITMLeafBundleEnvelope{}, nil
}

func (mainTestKMSProvider) DecryptMITMLeafBundle(context.Context, control.MITMLeafBundleEnvelope, control.MITMLeafBundleAAD) ([]byte, error) {
	return nil, nil
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
