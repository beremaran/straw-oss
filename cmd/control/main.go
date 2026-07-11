// Package main runs the Straw control service.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
	"github.com/beremaran/straw/v2/internal/logging"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/objectstore"
	"github.com/beremaran/straw/v2/internal/postgresx"
	"github.com/beremaran/straw/v2/internal/redisx"
	"github.com/beremaran/straw/v2/migrations"
)

const (
	exitUsage               = 2
	readHeaderTimeout       = 5 * time.Second
	controlShutdownTimeout  = 5 * time.Second
	redisPingTimeout        = 2 * time.Second
	invalidationPollPeriod  = 30 * time.Second
	clickHouseWriteTimeout  = 5 * time.Second
	healthcheckProbeTimeout = 2 * time.Second
	maxProxyHeaderCapture   = 64 << 10
	mitmLeafKeyBits         = 2048
	mitmSerialBits          = 128
	hoursPerDay             = 24
	mitmDefaultValidityDays = 30
	mitmLeafKMSProviderAWS  = "aws-kms"
)

var (
	errHealthcheckNotReady             = errors.New("healthcheck probe returned non-2xx status")
	errDecodeMITMCACert                = errors.New("decode mitm ca cert")
	errDecodeMITMCAKey                 = errors.New("decode mitm ca key")
	errOpenConfiguredFile              = errors.New("open configured file")
	errUnsupportedMITMLeafKMSProvider  = errors.New("unsupported mitm leaf kms provider")
	errMITMLeafCacheRequiresCAAndRedis = errors.New("mitm leaf cache requires ca and redis")
	errMITMLeafCacheRequiresKMS        = errors.New("mitm leaf cache requires kms provider")
)

type mitmCA struct {
	cert *x509.Certificate
	key  any
}

func main() {
	slog.SetDefault(logging.New("control"))

	err := run()
	if err != nil {
		slog.Error("control failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	controlConfig, healthcheck, err := loadControlConfig()
	if err != nil {
		return fmt.Errorf("load control config: %w", err)
	}

	if healthcheck {
		return runHealthcheck(controlConfig)
	}

	err = natsx.ValidateServers(controlConfig.NATS.Servers)
	if err != nil {
		return fmt.Errorf("validate nats servers: %w", err)
	}

	err = natsx.ValidateMaxPayload(controlConfig.NATS.MaxPayloadBytes, controlConfig.Transport.MaxFrameDataBytes, controlConfig.Request.MaxInlineRequestBodyBytes, controlConfig.Request.MaxInlineResponseBodyBytes)
	if err != nil {
		return fmt.Errorf("validate payload limits: %w", err)
	}

	_, _, err = buildMITMLeafBundleProvider(controlConfig)
	if err != nil {
		return fmt.Errorf("validate mitm leaf bundle kms config: %w", err)
	}

	natsConn, err := natsx.Connect(natsx.ConnectOptions{
		Servers:             controlConfig.NATS.Servers,
		UserCredentialsFile: controlConfig.NATS.UserCredentialsFile,
		ReconnectAttempts:   controlConfig.NATS.ReconnectAttempts,
		ReconnectWait:       time.Duration(controlConfig.NATS.ReconnectWaitMS) * time.Millisecond,
		PingInterval:        time.Duration(controlConfig.NATS.PingIntervalMS) * time.Millisecond,
		MaxPingFailures:     controlConfig.NATS.MaxPingFailures,
	})
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}

	err = natsx.ValidateConnectedMaxPayload(natsConn, controlConfig.Transport.MaxFrameDataBytes, controlConfig.Request.MaxInlineRequestBodyBytes, controlConfig.Request.MaxInlineResponseBodyBytes)
	if err != nil {
		natsConn.Close()

		return fmt.Errorf("validate live nats payload limits: %w", err)
	}

	return runControl(controlConfig, natsConn)
}

func runControl(controlConfig config.ControlConfig, natsConn *natsx.Connection) error {
	defer func() {
		if natsConn != nil {
			drainErr := natsConn.Drain()
			if drainErr != nil {
				slog.Warn("drain nats connection failed", "error", drainErr)
			}
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pepper := []byte(os.Getenv("STRAW_API_KEY_PEPPER"))

	pool, redisClient, closeStores, err := openStores(controlConfig)
	if err != nil {
		return err
	}

	defer closeStores()

	apiKeyStore, err := setupAPIKeyStore(pool, pepper)
	if err != nil {
		return err
	}

	workerCreds := control.NewPostgresWorkerCredentialStore(pool)
	workerRegistry := control.NewWorkerRegistry(workerCreds, control.DefaultWorkerTimings(), nil)
	workerRegistry.SetRuntimeStore(control.NewRedisWorkerRuntimeStore(redisClient))
	wireWorkerRegistrationReplayProtection(workerRegistry, controlConfig.Worker, redisClient)

	configStore := control.NewPostgresConfigStore(pool)

	err = setupControlConfigState(configStore, workerRegistry, workerCreds, pool)
	if err != nil {
		return err
	}

	configCache := wireConfigInvalidation(ctx, configStore, redisClient)

	chWriters := wireTelemetry(controlConfig.Database.ClickHouse, workerRegistry)
	defer chWriters.Close()

	metricsReg, metrics := wireMetrics(controlConfig, workerRegistry, chWriters)

	inflight := wireInFlightRegistry(ctx, controlConfig, redisClient)

	mux, proxyHandler, connectHandler, mitmHandler, err := buildControlMux(ctx, controlConfig, apiKeyStore, pepper, workerRegistry, workerCreds, pool, configStore, configCache, redisClient, natsConn, chWriters, metrics, inflight)
	if err != nil {
		return err
	}

	err = setupNATSSubscriptions(natsConn, workerRegistry, chWriters)
	if err != nil {
		return err
	}

	return serveControl(ctx, controlConfig, mux, proxyHandler, connectHandler, mitmHandler, redisClient, metricsReg)
}

func setupControlConfigState(configStore *control.PostgresConfigStore, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool) error {
	err := bootstrapDevProvisioning(context.Background(), control.NewPostgresTenantStore(pool), workerCreds, configStore)
	if err != nil {
		return err
	}

	err = rehydrateWorkerAdminState(context.Background(), configStore, workerRegistry)
	if err != nil {
		return fmt.Errorf("rehydrate worker admin state: %w", err)
	}

	return nil
}

func buildBodyRefStore(ctx context.Context, cfg config.BodyObjectStorageConfig) (*control.S3RequestBodyRefStore, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	client, err := objectstore.New(objectstore.Options{
		Enabled:       cfg.Enabled,
		Endpoint:      cfg.Endpoint,
		Bucket:        cfg.Bucket,
		Region:        cfg.Region,
		AccessKeyEnv:  cfg.AccessKeyEnv,
		SecretKeyEnv:  cfg.SecretKeyEnv,
		RetentionDays: cfg.BodyRetentionDays,
	})
	if err != nil {
		return nil, fmt.Errorf("build body ref store: %w", err)
	}

	err = client.ApplyLifecycleRetention(ctx)
	if err != nil {
		return nil, fmt.Errorf("apply body object lifecycle retention: %w", err)
	}

	return control.NewS3RequestBodyRefStore(client), nil
}

func setupNATSSubscriptions(natsConn *natsx.Connection, workerRegistry *control.WorkerRegistry, chWriters *clickHouseWriters) error {
	err := control.SetupWorkerDiscoverySubscriptions(natsConn, workerRegistry)
	if err != nil {
		return fmt.Errorf("setup worker discovery: %w", err)
	}

	if chWriters == nil {
		return nil
	}

	err = control.SetupLogEventSubscription(natsConn, chWriters.logEvents)
	if err != nil {
		return fmt.Errorf("setup log events: %w", err)
	}

	return nil
}

// wireWorkerRegistrationReplayProtection wires the Redis-backed registration
// nonce store into workerRegistry per the configured skew/TTL/fail policy
// (docs/planning/27-security-controls.md "Worker Credential Signing"). A
// configured-but-unreachable Redis is not special-cased here: the store's
// Consume call surfaces the error per-registration and the registry applies
// workerCfg.RegistrationFailOpenOnRedisOutage (fail-closed by default).
func wireWorkerRegistrationReplayProtection(workerRegistry *control.WorkerRegistry, workerCfg config.ControlWorkerConfig, redisClient *redis.Client) {
	workerRegistry.SetNonceStore(control.NewRedisWorkerNonceStore(redisClient), control.WorkerRegistrationPolicy{
		ClockSkew:                 time.Duration(workerCfg.RegistrationClockSkewMS) * time.Millisecond,
		NonceTTL:                  time.Duration(workerCfg.RegistrationNonceTTLMS) * time.Millisecond,
		FailOpenOnNonceStoreError: workerCfg.RegistrationFailOpenOnRedisOutage,
	})
}

// wireMetrics builds the Prometheus registry and the P0 metric series
// (docs/planning/23-observability.md) and registers the pull-based worker
// and ClickHouse-queue-depth collectors, which read live state from
// workerRegistry and chWriters at scrape time rather than being pushed.
func wireMetrics(controlConfig config.ControlConfig, workerRegistry *control.WorkerRegistry, chWriters *clickHouseWriters) (*prometheus.Registry, *control.Metrics) {
	reg := prometheus.NewRegistry()
	metrics := control.NewMetrics(reg)

	control.RegisterWorkerCollector(reg, workerRegistry)

	if controlConfig.Server.EgressMetricsEnabled {
		control.RegisterEgressMetricsCollector(reg, workerRegistry)
	}

	if chWriters.requestMetadata != nil {
		chWriters.SetMetrics(metrics)
		control.RegisterClickHouseQueueDepth(reg, chWriters)
	}

	return reg, metrics
}

// openStores opens the required Postgres pool and the Redis client and returns
// a cleanup that closes both. Postgres is the control-plane source of truth and
// must be reachable at startup (docs/planning/21); a configured-but-unreachable
// Redis only degrades Redis-backed features per their fail policies and does
// not block startup (see openRedis).
func openStores(controlConfig config.ControlConfig) (*pgxpool.Pool, *redis.Client, func(), error) {
	pool, err := openPostgres(controlConfig.Database.Postgres)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open postgres: %w", err)
	}

	redisClient, err := openRedis(controlConfig.Database.Redis)
	if err != nil {
		pool.Close()

		return nil, nil, nil, fmt.Errorf("open redis: %w", err)
	}

	cleanup := func() {
		closeErr := redisClient.Close()
		if closeErr != nil {
			slog.Warn("close redis client failed", "error", closeErr)
		}

		pool.Close()
	}

	return pool, redisClient, cleanup, nil
}

// serveControl starts the metrics/readiness server and the API server, marking
// readiness true until ctx cancellation begins drain.
func serveControl(ctx context.Context, controlConfig config.ControlConfig, mux *http.ServeMux, proxyHandler http.Handler, connectHandler http.Handler, mitmHandler http.Handler, redisClient *redis.Client, metricsReg *prometheus.Registry) error {
	ready := &atomic.Bool{}
	ready.Store(true)

	stopMetrics := serveMetricsHTTP(ctx, controlConfig, ready, metricsReg)
	defer stopMetrics()

	stopProxy := serveProxyHTTP(ctx, controlConfig, proxyHandler)
	defer stopProxy()

	stopConnect := serveConnectHTTP(ctx, controlConfig, connectHandler)
	defer stopConnect()

	stopMITM := serveMITMHTTP(ctx, controlConfig, mitmHandler, redisClient)
	defer stopMITM()

	return serveControlHTTP(ctx, controlConfig, mux, ready)
}

func buildMITMLeafBundleProviderConfig(controlConfig config.ControlConfig) (*control.MITMLeafBundleProviderConfig, error) {
	if controlConfig.Server.MITMLeafKMSProvider == "" && controlConfig.Server.MITMLeafKMSKeyID == "" {
		return nil, nil
	}

	providerConfig, err := control.NewMITMLeafBundleProviderConfig(controlConfig.Server.MITMLeafKMSProvider, controlConfig.Server.MITMLeafKMSKeyID)
	if err != nil {
		return nil, fmt.Errorf("build mitm leaf bundle provider config: %w", err)
	}

	return &providerConfig, nil
}

func buildMITMLeafBundleProvider(controlConfig config.ControlConfig) (*control.MITMLeafBundleProviderConfig, control.MITMLeafBundleKMSProvider, error) {
	providerConfig, err := buildMITMLeafBundleProviderConfig(controlConfig)
	if err != nil || providerConfig == nil {
		return providerConfig, nil, err
	}

	switch strings.ToLower(providerConfig.ProviderName) {
	case mitmLeafKMSProviderAWS:
		return providerConfig, control.NewAWSMITMLeafBundleKMSProvider(nil), nil
	default:
		return nil, nil, fmt.Errorf("%w: %s", errUnsupportedMITMLeafKMSProvider, providerConfig.ProviderName)
	}
}

func serveMITMHTTP(ctx context.Context, controlConfig config.ControlConfig, handler http.Handler, redisClient *redis.Client) func() {
	if !controlConfig.Server.MITMEnabled || handler == nil {
		return func() {}
	}

	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.MITMPort)

	leafHooks := newMITMLeafFileHooks(controlConfig, redisClient, nil)

	_, _, err := leafHooks.hooks()
	if err != nil {
		slog.Error("mitm leaf cache setup failed", "error", err)

		return func() {}
	}

	server := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	server.Handler = configureMITMServer(handler, leafHooks.Lookup, leafHooks.Preflight, controlConfig.HTTP2.Enabled)

	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("mitm server failed", "error", serveErr)
		}
	}()

	slog.Info("mitm proxy listening", "addr", addr)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			slog.Error("shutdown mitm server failed", "error", shutdownErr)
		}
	}
}

func configureMITMServer(handler http.Handler, leafLookup control.MITMLeafLookup, leafPreflight control.MITMLeafPreflight, http2Enabled bool) http.Handler {
	mitm, ok := handler.(*control.MITMHandler)
	if !ok {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "mitm handler not configured", http.StatusInternalServerError)
		})
	}

	connect := control.NewMITMConnectHandler(mitm.Authenticator(), handler, leafLookup)
	connect.SetLeafPreflight(leafPreflight)
	connect.SetHTTP2Enabled(http2Enabled)
	connect.SetHTTP2Policy(mitm.AllowsHTTP2)

	return connect
}

func buildMITMLeafLookup(controlConfig config.ControlConfig, ca *mitmCA, redisClient redis.Cmdable, provider control.MITMLeafBundleKMSProvider) (control.MITMLeafLookup, error) {
	lookup, _, err := buildMITMLeafHooks(controlConfig, ca, redisClient, provider)

	return lookup, err
}

type mitmLeafFileHooks struct {
	cfg      config.ControlConfig
	redis    redis.Cmdable
	provider control.MITMLeafBundleKMSProvider

	mu        sync.Mutex
	version   string
	lookup    control.MITMLeafLookup
	preflight control.MITMLeafPreflight
}

func newMITMLeafFileHooks(cfg config.ControlConfig, redisClient redis.Cmdable, provider control.MITMLeafBundleKMSProvider) *mitmLeafFileHooks {
	return &mitmLeafFileHooks{cfg: cfg, redis: redisClient, provider: provider}
}

func (h *mitmLeafFileHooks) Lookup(r *http.Request, identity control.Identity, sni, authority string) (*tls.Certificate, error) {
	lookup, _, err := h.hooks()
	if err != nil {
		return nil, err
	}

	return lookup(r, identity, sni, authority)
}

func (h *mitmLeafFileHooks) Preflight(r *http.Request, identity control.Identity, authority string) error {
	_, preflight, err := h.hooks()
	if err != nil {
		return err
	}

	return preflight(r, identity, authority)
}

func (h *mitmLeafFileHooks) hooks() (control.MITMLeafLookup, control.MITMLeafPreflight, error) {
	ca, err := loadMITMCACertificate(h.cfg.Server.MITMCACertFile, h.cfg.Server.MITMCAKeyFile)
	if err != nil {
		return nil, nil, err
	}

	_, version := mitmCAIdentityVersion(ca.cert)

	h.mu.Lock()
	defer h.mu.Unlock()

	if version == h.version && h.lookup != nil && h.preflight != nil {
		return h.lookup, h.preflight, nil
	}

	lookup, preflight, err := buildMITMLeafHooks(h.cfg, ca, h.redis, h.provider)
	if err != nil {
		return nil, nil, err
	}

	h.version = version
	h.lookup = lookup
	h.preflight = preflight

	return lookup, preflight, nil
}

func buildMITMLeafHooks(controlConfig config.ControlConfig, ca *mitmCA, redisClient redis.Cmdable, provider control.MITMLeafBundleKMSProvider) (control.MITMLeafLookup, control.MITMLeafPreflight, error) {
	if ca == nil || redisClient == nil {
		return nil, nil, errMITMLeafCacheRequiresCAAndRedis
	}

	providerConfig, builtProvider, err := buildMITMLeafBundleProvider(controlConfig)
	if err != nil {
		return nil, nil, err
	}

	if provider == nil {
		provider = builtProvider
	}

	if providerConfig == nil || provider == nil {
		return nil, nil, errMITMLeafCacheRequiresKMS
	}

	validity := time.Duration(controlConfig.Server.MITMCertValidityDays) * hoursPerDay * time.Hour
	caIdentity, caVersion := mitmCAIdentityVersion(ca.cert)

	cache, err := control.NewMITMLeafCache(control.MITMLeafCacheConfig{
		Redis:        redisClient,
		KMS:          provider,
		KMSKeyID:     providerConfig.KeyID,
		DeploymentID: controlConfig.DeploymentID,
		CAIdentity:   caIdentity,
		CAVersion:    caVersion,
		Validity:     validity,
		Generate: func(_ context.Context, normalizedSNI string) (*tls.Certificate, error) {
			return generateMITMLeaf(ca, normalizedSNI, validity)
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build mitm leaf cache: %w", err)
	}

	lookup := func(r *http.Request, identity control.Identity, sni, authority string) (*tls.Certificate, error) {
		return cache.Leaf(r.Context(), identity, sni, authority)
	}
	preflight := func(r *http.Request, identity control.Identity, authority string) error {
		return cache.Preflight(r.Context(), identity, authority)
	}

	return lookup, preflight, nil
}

func loadMITMCACertificate(certFile, keyFile string) (*mitmCA, error) {
	certPEM, err := readConfiguredFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("read mitm ca cert: %w", err)
	}

	keyPEM, err := readConfiguredFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read mitm ca key: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errDecodeMITMCACert
	}

	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse mitm ca cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errDecodeMITMCAKey
	}

	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse mitm ca key: %w", err)
		}
	}

	return &mitmCA{cert: ca, key: key}, nil
}

func readConfiguredFile(path string) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open configured file: %w", err)
	}

	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = syscall.Close(fd)

		return nil, errOpenConfiguredFile
	}

	defer func() {
		_ = f.Close()
	}()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read configured file: %w", err)
	}

	return b, nil
}

func generateMITMLeaf(ca *mitmCA, serverName string, validity time.Duration) (*tls.Certificate, error) {
	if serverName == "" {
		serverName = "mitm.local"
	}

	if validity <= 0 {
		validity = mitmDefaultValidityDays * hoursPerDay * time.Hour
	}

	key, err := rsa.GenerateKey(rand.Reader, mitmLeafKeyBits)
	if err != nil {
		return nil, fmt.Errorf("generate mitm leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), mitmSerialBits))
	if err != nil {
		return nil, fmt.Errorf("generate mitm leaf serial: %w", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	ip, err := netip.ParseAddr(serverName)
	if err == nil {
		tpl.IPAddresses = []net.IP{ip.AsSlice()}
	} else {
		tpl.DNSNames = []string{serverName}
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("sign mitm leaf: %w", err)
	}

	return &tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key, Leaf: tpl}, nil
}

func mitmCAIdentityVersion(ca *x509.Certificate) (string, string) {
	if ca == nil {
		return "", ""
	}

	keySum := sha256.Sum256(ca.RawSubjectPublicKeyInfo)
	certSum := sha256.Sum256(ca.Raw)

	return hex.EncodeToString(keySum[:]), hex.EncodeToString(certSum[:])
}

func serveConnectHTTP(ctx context.Context, controlConfig config.ControlConfig, handler http.Handler) func() {
	if !controlConfig.Server.ConnectEnabled || handler == nil {
		return func() {}
	}

	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.ConnectPort)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	if !controlConfig.HTTP2.Enabled {
		server.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	}

	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("connect server failed", "error", serveErr)
		}
	}()

	slog.Info("connect proxy listening", "addr", addr)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			slog.Error("shutdown connect server failed", "error", shutdownErr)
		}
	}
}

func serveProxyHTTP(ctx context.Context, controlConfig config.ControlConfig, handler http.Handler) func() {
	if !controlConfig.Server.ProxyEnabled || handler == nil {
		return func() {}
	}

	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.ProxyPort)

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		slog.Error("proxy listen failed", "addr", addr, "error", err)

		return func() {}
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			if capture, ok := c.(*proxyHeaderCaptureConn); ok {
				return control.WithProxyRawHeaderSource(ctx, capture)
			}

			return ctx
		},
	}
	if !controlConfig.HTTP2.Enabled {
		server.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	}

	go func() {
		serveErr := server.Serve(proxyHeaderCaptureListener{Listener: listener})
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("proxy server failed", "error", serveErr)
		}
	}()

	slog.Info("proxy listening", "addr", addr)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			slog.Error("shutdown proxy server failed", "error", shutdownErr)
		}
	}
}

type proxyHeaderCaptureListener struct {
	net.Listener
}

func (l proxyHeaderCaptureListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept proxy connection: %w", err)
	}

	return &proxyHeaderCaptureConn{Conn: conn}, nil
}

type proxyHeaderCaptureConn struct {
	net.Conn

	mu     sync.Mutex
	header []byte
	done   bool
}

func (c *proxyHeaderCaptureConn) ProxyRawHeaderBlock() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]byte(nil), c.header...)
}

func (c *proxyHeaderCaptureConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.capture(p[:n])
	}

	if err != nil {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}

		return n, fmt.Errorf("read proxy connection: %w", err)
	}

	return n, nil
}

func (c *proxyHeaderCaptureConn) capture(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.done {
		return
	}

	remaining := maxProxyHeaderCapture - len(c.header)
	if remaining <= 0 {
		c.done = true

		return
	}

	if len(b) > remaining {
		b = b[:remaining]
		c.done = true
	}

	c.header = append(c.header, b...)
	if end := bytes.Index(c.header, []byte("\r\n\r\n")); end >= 0 {
		c.header = c.header[:end+4]
		c.done = true
	}
}

// serveMetricsHTTP starts the liveness/readiness/metrics server on the
// metrics port (docs/planning/28) and returns a stop function that shuts it
// down.
func serveMetricsHTTP(ctx context.Context, controlConfig config.ControlConfig, ready *atomic.Bool, metricsReg *prometheus.Registry) func() {
	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.MetricsPort)
	server := &http.Server{Addr: addr, Handler: newMetricsMux(ready, metricsReg), ReadHeaderTimeout: readHeaderTimeout}

	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("metrics server failed", "error", serveErr)
		}
	}()

	slog.Info("metrics listening", "addr", addr)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			slog.Error("shutdown metrics server failed", "error", shutdownErr)
		}
	}
}

// setupAPIKeyStore builds the Postgres-backed API key store and bootstraps
// the first platform system_admin key from STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY
// if the store is empty.
func setupAPIKeyStore(pool *pgxpool.Pool, pepper []byte) (control.APIKeyStore, error) {
	apiKeyStore := control.NewPostgresAPIKeyStore(pool, pepper)

	created, err := bootstrapAPIKey(apiKeyStore, pepper)
	if err != nil {
		return nil, err
	}

	if created {
		slog.Info("bootstrapped first platform system_admin API key", "source_env", control.BootstrapSystemAdminEnvVar)
	}

	return apiKeyStore, nil
}

// wireConfigInvalidation builds the ConfigCache with a real Redis-backed
// invalidation publisher and starts the pub/sub subscriber and periodic
// Postgres poller that keep it in sync (docs/planning/25). Both background
// loops run until ctx is canceled.
func wireConfigInvalidation(ctx context.Context, configStore *control.PostgresConfigStore, redisClient *redis.Client) *control.ConfigCache {
	configCache := control.NewConfigCache(configStore, control.NewRedisInvalidationPublisher(redisClient))

	go runInvalidationSubscriber(ctx, redisClient, configCache)
	go runInvalidationPoller(ctx, configCache)

	return configCache
}

// wireInFlightRegistry builds the admin-cancel in-flight registry. When
// server.multi_control_enabled is set (docs/planning/32 "Multiple Concurrent
// Control Replicas"), it attaches the Redis-backed cross-instance coordinator
// so a cancel for a request owned by a sibling Control replica reaches that
// replica (docs/implementation-history.md#p1-23), and starts the pub/sub subscriber that applies
// sibling-authorized cancels to this replica's local contexts. Disabled (the
// default) leaves the registry pure in-process/single-Control.
func wireInFlightRegistry(ctx context.Context, controlConfig config.ControlConfig, redisClient *redis.Client) *control.InFlightRegistry {
	inflight := control.NewInFlightRegistry()

	if !controlConfig.Server.MultiControlEnabled {
		return inflight
	}

	inflight.SetCrossInstance(control.NewRedisInFlightCoordinator(redisClient, 0))

	go runRequestCancelSubscriber(ctx, redisClient, inflight)

	return inflight
}

// runRequestCancelSubscriber runs the cross-instance cancel pub/sub subscriber
// until ctx is canceled, reconnecting after transient errors (e.g. a Redis
// restart) rather than exiting the process.
func runRequestCancelSubscriber(ctx context.Context, client *redis.Client, inflight *control.InFlightRegistry) {
	subscriber := control.NewRedisRequestCancelSubscriber(client, inflight)

	for {
		if ctx.Err() != nil {
			return
		}

		err := subscriber.Run(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("request cancel subscriber failed; retrying", "error", err)

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func wireTelemetry(cfg config.ClickHouseConfig, workerRegistry *control.WorkerRegistry) *clickHouseWriters {
	chWriters := wireClickHouseWriters(cfg)
	wireLogEvents(chWriters)
	workerRegistry.SetEventRecorder(chWriters.workerEvents)

	return chWriters
}

// clickHouseWriters holds the async ClickHouse event writers. They share one
// HTTP sink and the same queue/batch/flush tuning; any
// field is nil when no ClickHouse endpoint is configured, in which case
// Control records no telemetry for that table.
type clickHouseWriters struct {
	sink            *control.HTTPClickHouseSink
	requestMetadata *control.RequestMetadataWriter
	workerEvents    *control.WorkerEventWriter
	configAudit     *control.ConfigAuditEventWriter
	logEvents       *control.LogEventWriter
	payloadCapture  *control.PayloadCaptureEventWriter
}

// SetMetrics attaches the shared Prometheus metrics recorder to every writer.
func (w *clickHouseWriters) SetMetrics(m *control.Metrics) {
	if w == nil || w.requestMetadata == nil {
		return
	}

	w.requestMetadata.SetMetrics(m)
	w.workerEvents.SetMetrics(m)
	w.configAudit.SetMetrics(m)
	w.logEvents.SetMetrics(m)
	w.payloadCapture.SetMetrics(m)
}

// QueueDepth sums the buffered event count across all writers for the
// single straw_clickhouse_write_queue_depth gauge (docs/planning/23).
func (w *clickHouseWriters) QueueDepth() int {
	if w == nil || w.requestMetadata == nil {
		return 0
	}

	return w.requestMetadata.QueueDepth() + w.workerEvents.QueueDepth() + w.configAudit.QueueDepth() + w.logEvents.QueueDepth() + w.payloadCapture.QueueDepth()
}

// Close stops every writer's background flush loop and drains its queue.
func (w *clickHouseWriters) Close() {
	if w == nil || w.requestMetadata == nil {
		return
	}

	w.requestMetadata.Close()
	w.workerEvents.Close()
	w.configAudit.Close()
	w.logEvents.Close()
	w.payloadCapture.Close()
}

// wireClickHouseWriters builds the async event writers backed by the live
// ClickHouse HTTP interface (docs/planning/22). Writers are left nil when no
// endpoint is configured, in which case Control records no telemetry. A
// ClickHouse outage never blocks the transport: each writer keeps its own
// bounded queue and drops oldest events per docs/planning/29.
func wireClickHouseWriters(cfg config.ClickHouseConfig) *clickHouseWriters {
	if cfg.Endpoint == "" {
		slog.Info("no clickhouse endpoint configured; telemetry disabled")

		return &clickHouseWriters{}
	}

	sink := control.NewHTTPClickHouseSink(
		cfg.Endpoint,
		cfg.Database,
		os.Getenv(cfg.UserEnv),
		os.Getenv(cfg.PasswordEnv),
		&http.Client{Timeout: clickHouseWriteTimeout},
	)

	slog.Info("telemetry writing to clickhouse", "endpoint", cfg.Endpoint, "database", cfg.Database)

	flushInterval := time.Duration(cfg.FlushIntervalMS) * time.Millisecond

	return &clickHouseWriters{
		sink:            sink,
		requestMetadata: control.NewRequestMetadataWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
		workerEvents:    control.NewWorkerEventWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
		configAudit:     control.NewConfigAuditEventWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
		logEvents:       control.NewLogEventWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
		payloadCapture:  control.NewPayloadCaptureEventWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
	}
}

func wireLogEvents(chWriters *clickHouseWriters) {
	if chWriters == nil || chWriters.logEvents == nil {
		return
	}

	slog.SetDefault(slog.New(logging.NewTeeHandler(logging.NewHandler(os.Stdout), chWriters.logEvents)).With("service", "control"))
}

// openPostgres connects to Postgres and applies the embedded migrations. The
// DSN (from STRAW_POSTGRES_DSN) is required; Control does not serve without its
// durable state store.
func openPostgres(pgCfg config.PostgresConfig) (*pgxpool.Pool, error) {
	cfg := postgresx.Config{
		DSNEnv:            pgCfg.DSNEnv,
		MaxOpenConns:      pgCfg.MaxOpenConns,
		MaxIdleConns:      pgCfg.MaxIdleConns,
		ConnMaxLifetimeMS: pgCfg.ConnMaxLifetimeMS,
	}

	dsn, err := postgresx.ResolveDSN(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve postgres dsn: %w", err)
	}

	pool, err := postgresx.Connect(context.Background(), cfg, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	err = postgresx.ApplyMigrations(context.Background(), pool, migrations.Postgres)
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("apply postgres migrations: %w", err)
	}

	return pool, nil
}

// openRedis resolves the Redis connection URL from STRAW_REDIS_URL (or
// redisCfg.URLEnv) and builds a client. An unresolvable URL is a
// configuration error and fails startup, the same as a missing Postgres
// DSN. A configured-but-unreachable Redis is different: per
// docs/planning/29 ("Redis unavailable: Apply configured fail policy"),
// Control must still start and serve, since every Redis-backed runtime
// component (RateLimiter, QuotaAdmission, RedisStickyStore, invalidation)
// already applies its own explicit fail policy per call.
func openRedis(redisCfg config.RedisConfig) (*redis.Client, error) {
	url, err := redisx.ResolveURL(redisCfg.URLEnv)
	if err != nil {
		return nil, fmt.Errorf("resolve redis url: %w", err)
	}

	client, err := redisx.NewClientFromURL(url, redisx.Config{
		DialTimeout:  time.Duration(redisCfg.DialTimeoutMS) * time.Millisecond,
		ReadTimeout:  time.Duration(redisCfg.ReadTimeoutMS) * time.Millisecond,
		WriteTimeout: time.Duration(redisCfg.WriteTimeoutMS) * time.Millisecond,
	})
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), redisPingTimeout)
	defer cancel()

	pingErr := client.Ping(pingCtx).Err()
	if pingErr != nil {
		slog.Warn("redis unreachable at startup; continuing with redis-backed features degraded", "error", pingErr)
	}

	return client, nil
}

// runInvalidationSubscriber runs the Redis pub/sub config-invalidation
// subscriber until ctx is canceled, reconnecting after transient errors
// (e.g. a Redis restart) rather than exiting the process — pub/sub is an
// acceleration mechanism, and runInvalidationPoller is the durable fallback.
func runInvalidationSubscriber(ctx context.Context, client *redis.Client, cache *control.ConfigCache) {
	subscriber := control.NewRedisInvalidationSubscriber(client, cache)

	for {
		if ctx.Err() != nil {
			return
		}

		err := subscriber.Run(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("config invalidation subscriber failed; retrying", "error", err)

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

// runInvalidationPoller periodically re-checks Postgres tenant versions for
// every cached tenant, recovering from a missed Redis invalidation message
// (docs/planning/25 "periodic Postgres version poll").
func runInvalidationPoller(ctx context.Context, cache *control.ConfigCache) {
	ticker := time.NewTicker(invalidationPollPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cache.PollAllTenants(ctx)
		}
	}
}

// rehydrateWorkerAdminState reloads durable worker disable decisions from
// Postgres into the runtime registry so admin actions survive a Control
// restart (docs/planning/21). Drain state is runtime-only and is not restored.
func rehydrateWorkerAdminState(ctx context.Context, configStore *control.PostgresConfigStore, registry *control.WorkerRegistry) error {
	globals, err := configStore.ListWorkerAdminStates(ctx)
	if err != nil {
		return fmt.Errorf("list worker admin states: %w", err)
	}

	for _, g := range globals {
		if g.Disabled {
			registry.SetGlobalAdmin(g.WorkerID, control.AdminDisabled)
		}
	}

	overrides, err := configStore.ListTenantWorkerOverrides(ctx)
	if err != nil {
		return fmt.Errorf("list tenant worker overrides: %w", err)
	}

	for _, o := range overrides {
		if o.Disabled {
			registry.SetTenantAdmin(o.WorkerID, o.TenantID, control.AdminDisabled)
		}
	}

	return nil
}

// buildControlMux assembles the HTTP handler with the Postgres-backed identity
// and config stores.
func buildControlMux(ctx context.Context, controlConfig config.ControlConfig, apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool, configStore *control.PostgresConfigStore, configCache *control.ConfigCache, redisClient *redis.Client, natsConn *natsx.Connection, chWriters *clickHouseWriters, metrics *control.Metrics, inflight *control.InFlightRegistry) (*http.ServeMux, http.Handler, http.Handler, http.Handler, error) {
	bodyStore, err := buildBodyRefStore(ctx, controlConfig.BodyTransport.ObjectStorage)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Typed-nil guard: a nil *S3RequestBodyRefStore stored in an interface is not
	// == nil, which would defeat the dispatcher's nil checks. Only wire the
	// stores when object storage is actually enabled.
	var (
		requestBodyStore  control.RequestBodyRefStore
		responseBodyStore control.ResponseBodyRefStore
	)

	if bodyStore != nil {
		requestBodyStore = bodyStore
		responseBodyStore = bodyStore
	}

	tenantStore := control.NewPostgresTenantStore(pool)
	adminHandlers := buildAdminHandlers(apiKeyStore, pepper, workerRegistry, workerCreds, pool, configStore, configCache, redisClient, inflight, tenantStore, chWriters.configAudit, chWriters.sink)
	startQuotaReconciliation(ctx, adminHandlers.QuotaReconciler, adminHandlers.Quotas)

	authenticator := control.NewAuthenticator(apiKeyStore, pepper).SetTenantStore(tenantStore)

	requestHandler := newControlRequestHandler(controlConfig, authenticator, chWriters)

	rateLimitAdmission := control.NewRateLimitAdmission(control.NewRateLimiter(redisClient, control.DefaultRateLimitGuardrails(), nil))
	rateLimitAdmission.SetMetrics(metrics)

	quotaAdmission := control.NewQuotaAdmission(redisClient, nil)
	quotaAdmission.SetMetrics(metrics)

	dispatcher := control.NewDefaultRequestDispatcher(control.RequestDispatcherOptions{
		ConfigCache:                configCache,
		Workers:                    workerRegistry,
		Sticky:                     control.NewRedisStickyStore(redisClient),
		NATS:                       natsConn,
		RateLimitAdmission:         rateLimitAdmission,
		QuotaAdmission:             quotaAdmission,
		BodyTransport:              controlConfig.BodyTransport,
		BodyObjectStore:            requestBodyStore,
		ResponseObjectStore:        responseBodyStore,
		MaxInlineResponseBodyBytes: controlConfig.Request.MaxInlineResponseBodyBytes,
		MaxFrameDataBytes:          controlConfig.Transport.MaxFrameDataBytes,
		MaxTimeoutMs:               controlConfig.Request.MaxTimeoutMs,
		InFlight:                   inflight,
		Metrics:                    metrics,
	})
	configureRequestHandler(requestHandler, configCache, pool, dispatcher, bodyStore, chWriters.payloadCapture)

	mux := http.NewServeMux()
	serveControlRoutes(mux, controlConfig, requestHandler, authenticator, configCache, adminHandlers)

	if chWriters.sink != nil {
		serveTelemetryRoutes(mux, &control.TelemetryHandlers{
			Authenticator: authenticator,
			Store:         control.NewHTTPClickHouseTelemetryStore(chWriters.sink),
		})
	}

	proxyHandler, connectHandler, mitmHandler := buildIngressHandlers(controlConfig, authenticator, configCache, chWriters.requestMetadata, dispatcher)

	return mux, proxyHandler, connectHandler, mitmHandler, nil
}

func newControlRequestHandler(controlConfig config.ControlConfig, authenticator *control.Authenticator, chWriters *clickHouseWriters) *control.RequestHandler {
	if chWriters.requestMetadata != nil {
		return control.NewRequestHandler(
			controlConfig.Request.MaxInlineRequestBodyBytes,
			controlConfig.Request.MaxInlineResponseBodyBytes,
			controlConfig.Request.MaxTimeoutMs,
			authenticator,
			chWriters.requestMetadata,
		)
	}

	return control.NewRequestHandler(
		controlConfig.Request.MaxInlineRequestBodyBytes,
		controlConfig.Request.MaxInlineResponseBodyBytes,
		controlConfig.Request.MaxTimeoutMs,
		authenticator,
	)
}

func serveControlRoutes(mux *http.ServeMux, controlConfig config.ControlConfig, requestHandler *control.RequestHandler, authenticator *control.Authenticator, configCache *control.ConfigCache, adminHandlers *control.AdminHandlers) {
	serveAdminUIRoutes(mux)
	mux.Handle("POST /api/v1/requests", requestHandler)
	mux.HandleFunc("POST /api/v1/requests:stream", requestHandler.ServeStreamHTTP)
	serveMITMCARoutes(mux, controlConfig, authenticator, configCache, adminHandlers.Audit)
	serveAdminRoutes(mux, adminHandlers)
}

func configureRequestHandler(h *control.RequestHandler, configCache *control.ConfigCache, pool *pgxpool.Pool, dispatcher control.RequestDispatcher, bodyStore *control.S3RequestBodyRefStore, captureEvents *control.PayloadCaptureEventWriter) {
	h.SetConfigCache(configCache)
	h.SetPayloadCapturePolicyStore(control.NewPostgresPayloadCapturePolicyStore(pool))

	if bodyStore != nil && captureEvents != nil {
		h.SetPayloadCaptureStore(control.NewPayloadCaptureStore(bodyStore, captureEvents))
	}

	h.SetDispatcher(dispatcher)
}

func serveTelemetryRoutes(mux *http.ServeMux, h *control.TelemetryHandlers) {
	mux.HandleFunc("GET /api/v1/telemetry/requests", h.Requests)
	mux.HandleFunc("GET /api/v1/telemetry/requests/{request_id}", h.RequestDetail)
	mux.HandleFunc("GET /api/v1/telemetry/workers", h.Workers)
	mux.HandleFunc("GET /api/v1/telemetry/audit", h.Audit)
}

func serveMITMCARoutes(mux *http.ServeMux, controlConfig config.ControlConfig, authenticator *control.Authenticator, configCache *control.ConfigCache, audit control.AuditStore) {
	if controlConfig.Server.MITMCACertFile == "" {
		return
	}

	h := &control.MITMCAHandler{
		Authenticator: authenticator,
		ConfigCache:   configCache,
		CertFile:      controlConfig.Server.MITMCACertFile,
		KeyFile:       controlConfig.Server.MITMCAKeyFile,
		Audit:         audit,
	}
	mux.Handle("GET /api/v1/mitm/ca.pem", h)
	mux.Handle("PUT /api/v1/mitm/ca", h)
}

func buildIngressHandlers(controlConfig config.ControlConfig, authenticator *control.Authenticator, configCache *control.ConfigCache, metadata control.RequestMetadataRecorder, dispatcher control.RequestDispatcher) (http.Handler, http.Handler, http.Handler) {
	return buildProxyHandler(controlConfig, authenticator, configCache, metadata, dispatcher),
		buildConnectHandler(controlConfig, authenticator, dispatcher),
		buildMITMHandler(controlConfig, authenticator, configCache, metadata, dispatcher)
}

func buildProxyHandler(controlConfig config.ControlConfig, authenticator *control.Authenticator, configCache *control.ConfigCache, metadata control.RequestMetadataRecorder, dispatcher control.RequestDispatcher) http.Handler {
	if !controlConfig.Server.ProxyEnabled {
		return nil
	}

	h := control.NewProxyHandler(
		controlConfig.Request.MaxInlineRequestBodyBytes,
		controlConfig.Request.MaxInlineResponseBodyBytes,
		controlConfig.Request.MaxTimeoutMs,
		authenticator,
		metadata,
	)
	h.SetDispatcher(dispatcher)
	h.SetConfigCache(configCache)

	return h
}

func buildConnectHandler(controlConfig config.ControlConfig, authenticator *control.Authenticator, dispatcher control.RequestDispatcher) http.Handler {
	if !controlConfig.Server.ConnectEnabled {
		return nil
	}

	h := control.NewConnectHandler(authenticator)
	h.SetDispatcher(dispatcher)

	return h
}

func buildMITMHandler(controlConfig config.ControlConfig, authenticator *control.Authenticator, configCache *control.ConfigCache, metadata control.RequestMetadataRecorder, dispatcher control.RequestDispatcher) http.Handler {
	if !controlConfig.Server.MITMEnabled {
		return nil
	}

	h := control.NewMITMHandler(
		controlConfig.Request.MaxInlineRequestBodyBytes,
		controlConfig.Request.MaxInlineResponseBodyBytes,
		controlConfig.Request.MaxTimeoutMs,
		authenticator,
		metadata,
	)
	h.SetDispatcher(dispatcher)
	h.SetConfigCache(configCache)

	return h
}

// buildAdminHandlers constructs the AdminHandlers with the Postgres-backed
// stores and the Redis-backed runtime admission components
// (docs/implementation-history.md#p0-21). These are the admin-surface instances; the request path
// consumes its own rate limiter/quota/sticky instances wired into the
// dispatcher (docs/implementation-history.md#p0-24).
func buildAdminHandlers(apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool, configStore *control.PostgresConfigStore, configCache *control.ConfigCache, redisClient *redis.Client, inflight *control.InFlightRegistry, tenantStore control.TenantStore, configAuditEvents *control.ConfigAuditEventWriter, clickHouseSink *control.HTTPClickHouseSink) *control.AdminHandlers {
	rateLimiter := control.NewRateLimiter(redisClient, control.DefaultRateLimitGuardrails(), nil)

	// A plain interface-typed nil check on configAuditEvents would miss a
	// typed nil *ConfigAuditEventWriter (no clickhouse endpoint configured),
	// so only pass a non-nil pointer through as the ConfigAuditRecorder.
	var auditEvents control.ConfigAuditRecorder
	if configAuditEvents != nil {
		auditEvents = configAuditEvents
	}

	var quotaReconciler *control.QuotaReconciler

	var quotaUsage control.QuotaUsageSnapshotStore
	if clickHouseSink != nil {
		quotaUsage = control.NewPostgresQuotaUsageStore(pool)
		quotaReconciler = control.NewQuotaReconciler(control.NewHTTPClickHouseQuotaUsageSource(clickHouseSink), redisClient, nil)
		quotaReconciler.SetSnapshotStore(quotaUsage)
	}

	return &control.AdminHandlers{
		Authenticator:  control.NewAuthenticator(apiKeyStore, pepper).SetTenantStore(tenantStore),
		APIKeys:        apiKeyStore,
		WorkerCreds:    workerCreds,
		Tenants:        tenantStore,
		Quotas:         control.NewPostgresQuotaStore(pool),
		RateLimits:     control.NewPostgresRateLimitConfigStore(pool),
		PayloadCapture: control.NewPostgresPayloadCapturePolicyStore(pool),
		Audit:          control.NewAuditStoreWithEvents(control.NewPostgresAuditStore(pool), auditEvents),
		ConfigCache:    configCache,
		Workers:        workerRegistry,
		ConfigWrites:   configStore,
		WorkerAdmin:    configStore,
		InFlight:       inflight,
		Pepper:         pepper,

		RoutingRules:        configStore,
		ExecutorPools:       configStore,
		DenyRules:           configStore,
		InjectionPolicies:   configStore,
		FingerprintProfiles: configStore,

		RateLimiter:        rateLimiter,
		RateLimitAdmission: control.NewRateLimitAdmission(rateLimiter),
		QuotaAdmission:     control.NewQuotaAdmission(redisClient, nil),
		StickySessions:     control.NewRedisStickyStore(redisClient),
		QuotaReconciler:    quotaReconciler,
		QuotaUsage:         quotaUsage,
	}
}

func startQuotaReconciliation(ctx context.Context, reconciler *control.QuotaReconciler, quotas control.QuotaConfigSource) {
	if reconciler == nil || quotas == nil {
		return
	}

	go reconciler.Run(ctx, quotas)
}

// serveAdminRoutes registers all admin HTTP routes on the given mux.
func serveAdminRoutes(mux *http.ServeMux, h *control.AdminHandlers) {
	serveIdentityRoutes(mux, h)
	serveConfigResourceRoutes(mux, h)
	serveWorkerAdminRoutes(mux, h)
}

// serveIdentityRoutes registers tenant, API key, and worker credential
// lifecycle routes.
func serveIdentityRoutes(mux *http.ServeMux, h *control.AdminHandlers) {
	mux.HandleFunc("POST /api/v1/config/tenants", h.CreateTenant)
	mux.HandleFunc("GET /api/v1/config/tenants", h.ListTenants)
	mux.HandleFunc("GET /api/v1/config/tenants/{id}", h.GetTenant)
	mux.HandleFunc("PUT /api/v1/config/tenants/{id}", h.UpdateTenant)
	mux.HandleFunc("DELETE /api/v1/config/tenants/{id}", h.SoftDeleteTenant)
	mux.HandleFunc("POST /api/v1/config/platform-api-keys", h.CreatePlatformAPIKey)
	mux.HandleFunc("GET /api/v1/config/platform-api-keys", h.ListPlatformAPIKeys)
	mux.HandleFunc("POST /api/v1/config/platform-api-keys/{id}/revoke", h.RevokePlatformAPIKey)
	mux.HandleFunc("POST /api/v1/config/api-keys", h.CreateTenantAPIKey)
	mux.HandleFunc("POST /api/v1/config/tenants/{id}/api-keys", h.CreateTenantKeyForTenant)
	mux.HandleFunc("GET /api/v1/config/api-keys", h.ListTenantAPIKeys)
	mux.HandleFunc("POST /api/v1/config/api-keys/{id}/revoke", h.RevokeTenantAPIKey)
	mux.HandleFunc("POST /api/v1/config/worker-credentials", h.CreateWorkerCredential)
	mux.HandleFunc("GET /api/v1/config/worker-credentials", h.ListWorkerCredentials)
	mux.HandleFunc("POST /api/v1/config/worker-credentials/{id}/revoke", h.RevokeWorkerCredential)
}

// serveConfigResourceRoutes registers quota, rate-limit, routing, deny,
// injection, and fingerprint config routes.
func serveConfigResourceRoutes(mux *http.ServeMux, h *control.AdminHandlers) {
	mux.HandleFunc("GET /api/v1/config/quotas", h.GetQuotas)
	mux.HandleFunc("PUT /api/v1/config/tenants/{id}/quotas", h.PutTenantQuotas)
	mux.HandleFunc("GET /api/v1/config/rate-limits", h.GetRateLimits)
	mux.HandleFunc("PUT /api/v1/config/rate-limits", h.PutRateLimits)
	mux.HandleFunc("GET /api/v1/config/payload-capture", h.GetPayloadCapturePolicy)
	mux.HandleFunc("PUT /api/v1/config/payload-capture", h.PutPayloadCapturePolicy)
	mux.HandleFunc("GET /api/v1/config/routing-rules", h.ListRoutingRules)
	mux.HandleFunc("POST /api/v1/config/routing-rules", h.CreateRoutingRule)
	mux.HandleFunc("PUT /api/v1/config/routing-rules/{id}", h.UpdateRoutingRule)
	mux.HandleFunc("DELETE /api/v1/config/routing-rules/{id}", h.DeleteRoutingRule)
	mux.HandleFunc("GET /api/v1/config/executor-pools", h.ListExecutorPools)
	mux.HandleFunc("POST /api/v1/config/executor-pools", h.CreateExecutorPool)
	mux.HandleFunc("PUT /api/v1/config/executor-pools/{id}", h.UpdateExecutorPool)
	mux.HandleFunc("DELETE /api/v1/config/executor-pools/{id}", h.DeleteExecutorPool)
	mux.HandleFunc("GET /api/v1/config/deny-rules", h.ListDenyRules)
	mux.HandleFunc("POST /api/v1/config/deny-rules", h.CreateDenyRule)
	mux.HandleFunc("PUT /api/v1/config/deny-rules/{id}", h.UpdateDenyRule)
	mux.HandleFunc("DELETE /api/v1/config/deny-rules/{id}", h.DeleteDenyRule)
	mux.HandleFunc("GET /api/v1/config/injection-policies", h.ListInjectionPolicies)
	mux.HandleFunc("POST /api/v1/config/injection-policies", h.CreateInjectionPolicy)
	mux.HandleFunc("PUT /api/v1/config/injection-policies/{id}", h.UpdateInjectionPolicy)
	mux.HandleFunc("DELETE /api/v1/config/injection-policies/{id}", h.DeleteInjectionPolicy)
	mux.HandleFunc("GET /api/v1/config/fingerprint-profiles", h.ListFingerprintProfiles)
	mux.HandleFunc("GET /api/v1/config/changes", h.ListChanges)
	mux.HandleFunc("POST /api/v1/config/rollback", h.RollbackConfig)
}

// serveWorkerAdminRoutes registers runtime worker admin and request
// cancellation routes.
func serveWorkerAdminRoutes(mux *http.ServeMux, h *control.AdminHandlers) {
	mux.HandleFunc("GET /api/v1/admin/workers", h.ListWorkers)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/disable", h.DisableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/enable", h.EnableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/drain", h.DrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/undrain", h.UndrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-disable", h.TenantDisableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-enable", h.TenantEnableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-drain", h.TenantDrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-undrain", h.TenantUndrainWorker)
	mux.HandleFunc("POST /api/v1/admin/requests/{request_id}/cancel", h.CancelRequest)
}

func serveControlHTTP(ctx context.Context, controlConfig config.ControlConfig, mux *http.ServeMux, ready *atomic.Bool) error {
	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.APIPort)
	slog.Info("listening", "addr", addr)

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}
	if !controlConfig.HTTP2.Enabled {
		server.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	}

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		// Drain begins: mark readiness false so /readyz returns 503 and load
		// balancers stop sending new requests before the API server stops
		// (docs/planning/29 graceful shutdown).
		ready.Store(false)

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			return fmt.Errorf("shutdown control http server: %w", shutdownErr)
		}
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen and serve control http server: %w", err)
		}
	}

	return nil
}

func bootstrapAPIKey(store control.APIKeyStore, pepper []byte) (bool, error) {
	_, created, err := control.BootstrapFromEnv(context.Background(), store, os.Getenv(control.BootstrapSystemAdminEnvVar), pepper)
	if err != nil {
		return false, fmt.Errorf("bootstrap api key: %w", err)
	}

	return created, nil
}

// bootstrapDevProvisioning seeds the dev vertical slice so the docker-compose
// egress worker can both register and serve a live dispatch round-trip out of
// the box (see deploy/docker/README.md). When STRAW_BOOTSTRAP_DEV_TENANT_ID and
// STRAW_BOOTSTRAP_DEV_POOL_ID are set it seeds a dev tenant + routing rule and
// scopes the worker credential to that (tenant, pool); otherwise it falls back
// to the placeholder "dev" scope and only seeds the credential. Each step is a
// no-op once its resource exists. Dev-only: production provisions all of this
// through the admin API.
func bootstrapDevProvisioning(ctx context.Context, tenants control.TenantStore, creds control.WorkerCredentialStore, configStore *control.PostgresConfigStore) error {
	devTenantID := os.Getenv(control.DevTenantIDEnvVar)
	devPoolID := os.Getenv(control.DevPoolIDEnvVar)

	var scope []string

	var pools []control.AllowedPool

	if devTenantID != "" && devPoolID != "" {
		tenantCreated, err := control.BootstrapDevTenant(ctx, tenants, devTenantID)
		if err != nil {
			return fmt.Errorf("bootstrap dev tenant: %w", err)
		}

		ruleCreated, err := control.BootstrapDevRoutingRule(ctx, configStore, devTenantID, devPoolID)
		if err != nil {
			return fmt.Errorf("bootstrap dev routing rule: %w", err)
		}

		if tenantCreated || ruleCreated {
			slog.Info("bootstrapped dev routing path", "tenant_id", devTenantID, "pool_id", devPoolID)
		}

		scope = []string{devTenantID}
		pools = []control.AllowedPool{{TenantID: devTenantID, PoolID: devPoolID}}
	}

	created, err := control.BootstrapWorkerCredentialFromEnv(
		ctx,
		creds,
		os.Getenv(control.DevWorkerIDEnvVar),
		os.Getenv(control.DevWorkerPublicEd25519EnvVar),
		scope,
		pools,
	)
	if err != nil {
		return fmt.Errorf("bootstrap worker credential: %w", err)
	}

	if created {
		slog.Info("bootstrapped dev worker credential", "source_env", control.DevWorkerIDEnvVar)
	}

	return nil
}

func loadControlConfig() (config.ControlConfig, bool, error) {
	configPath := flag.String("config", "", "path to the control config file")
	healthcheck := flag.Bool("healthcheck", false, "probe the local /readyz endpoint and exit (for container healthchecks)")

	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")

		os.Exit(exitUsage)
	}

	controlConfig, err := config.LoadControl(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return config.ControlConfig{}, false, fmt.Errorf("load control config: %w", err)
	}

	return controlConfig, *healthcheck, nil
}

// runHealthcheck probes the local /readyz endpoint on the metrics port and
// returns nil only on a 2xx. Container healthchecks invoke the control binary
// with -healthcheck so the distroless image needs no extra probe tooling.
func runHealthcheck(controlConfig config.ControlConfig) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/readyz", controlConfig.Server.MetricsPort)

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build healthcheck request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck probe %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %s -> %d", errHealthcheckNotReady, url, resp.StatusCode)
	}

	return nil
}
