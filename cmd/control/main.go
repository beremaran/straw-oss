// Package main runs the Straw control service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
	"github.com/beremaran/straw/v2/internal/natsx"
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
)

var errHealthcheckNotReady = errors.New("healthcheck probe returned non-2xx status")

func main() {
	err := run()
	if err != nil {
		log.Fatalf("control: %v", err)
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
				log.Printf("control: drain nats connection: %v", drainErr)
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
	wireWorkerRegistrationReplayProtection(workerRegistry, controlConfig.Worker, redisClient)

	err = bootstrapWorkerCredential(workerCreds)
	if err != nil {
		return err
	}

	configStore := control.NewPostgresConfigStore(pool)

	err = rehydrateWorkerAdminState(context.Background(), configStore, workerRegistry)
	if err != nil {
		return fmt.Errorf("rehydrate worker admin state: %w", err)
	}

	configCache := wireConfigInvalidation(ctx, configStore, redisClient)

	chWriters := wireClickHouseWriters(controlConfig.Database.ClickHouse)
	defer chWriters.Close()

	workerRegistry.SetEventRecorder(chWriters.workerEvents)

	metricsReg, metrics := wireMetrics(workerRegistry, chWriters)

	mux := buildControlMux(controlConfig, apiKeyStore, pepper, workerRegistry, workerCreds, pool, configStore, configCache, redisClient, natsConn, chWriters, metrics)

	err = control.SetupWorkerDiscoverySubscriptions(natsConn, workerRegistry)
	if err != nil {
		return fmt.Errorf("setup worker discovery: %w", err)
	}

	return serveControl(ctx, controlConfig, mux, metricsReg)
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
func wireMetrics(workerRegistry *control.WorkerRegistry, chWriters *clickHouseWriters) (*prometheus.Registry, *control.Metrics) {
	reg := prometheus.NewRegistry()
	metrics := control.NewMetrics(reg)

	control.RegisterWorkerCollector(reg, workerRegistry)

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
			log.Printf("control: close redis client: %v", closeErr)
		}

		pool.Close()
	}

	return pool, redisClient, cleanup, nil
}

// serveControl starts the metrics/readiness server and the API server, marking
// readiness true until ctx cancellation begins drain.
func serveControl(ctx context.Context, controlConfig config.ControlConfig, mux *http.ServeMux, metricsReg *prometheus.Registry) error {
	ready := &atomic.Bool{}
	ready.Store(true)

	stopMetrics := serveMetricsHTTP(ctx, controlConfig, ready, metricsReg)
	defer stopMetrics()

	return serveControlHTTP(ctx, controlConfig, mux, ready)
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
			log.Printf("control: metrics server: %v", serveErr)
		}
	}()

	log.Printf("control: metrics listening on %s", addr)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			log.Printf("control: shutdown metrics server: %v", shutdownErr)
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
		log.Printf("control: bootstrapped first platform system_admin API key from %s", control.BootstrapSystemAdminEnvVar)
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

// clickHouseWriters holds the three async ClickHouse event writers
// (docs/tasks/p0/32): request_events, worker_events, and config_audit_events.
// All three share one HTTP sink and the same queue/batch/flush tuning; any
// field is nil when no ClickHouse endpoint is configured, in which case
// Control records no telemetry for that table.
type clickHouseWriters struct {
	requestMetadata *control.RequestMetadataWriter
	workerEvents    *control.WorkerEventWriter
	configAudit     *control.ConfigAuditEventWriter
}

// SetMetrics attaches the shared Prometheus metrics recorder to every writer.
func (w *clickHouseWriters) SetMetrics(m *control.Metrics) {
	w.requestMetadata.SetMetrics(m)
	w.workerEvents.SetMetrics(m)
	w.configAudit.SetMetrics(m)
}

// QueueDepth sums the buffered event count across all three writers for the
// single straw_clickhouse_write_queue_depth gauge (docs/planning/23).
func (w *clickHouseWriters) QueueDepth() int {
	return w.requestMetadata.QueueDepth() + w.workerEvents.QueueDepth() + w.configAudit.QueueDepth()
}

// Close stops every writer's background flush loop and drains its queue.
func (w *clickHouseWriters) Close() {
	w.requestMetadata.Close()
	w.workerEvents.Close()
	w.configAudit.Close()
}

// wireClickHouseWriters builds the async event writers backed by the live
// ClickHouse HTTP interface (docs/planning/22). Writers are left nil when no
// endpoint is configured, in which case Control records no telemetry. A
// ClickHouse outage never blocks the transport: each writer keeps its own
// bounded queue and drops oldest events per docs/planning/29.
func wireClickHouseWriters(cfg config.ClickHouseConfig) *clickHouseWriters {
	if cfg.Endpoint == "" {
		log.Printf("control: no clickhouse endpoint configured; telemetry disabled")

		return &clickHouseWriters{}
	}

	sink := control.NewHTTPClickHouseSink(
		cfg.Endpoint,
		cfg.Database,
		os.Getenv(cfg.UserEnv),
		os.Getenv(cfg.PasswordEnv),
		&http.Client{Timeout: clickHouseWriteTimeout},
	)

	log.Printf("control: telemetry writing to clickhouse at %s (db=%s)", cfg.Endpoint, cfg.Database)

	flushInterval := time.Duration(cfg.FlushIntervalMS) * time.Millisecond

	return &clickHouseWriters{
		requestMetadata: control.NewRequestMetadataWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
		workerEvents:    control.NewWorkerEventWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
		configAudit:     control.NewConfigAuditEventWriter(sink, cfg.MaxQueueEntries, cfg.BatchSize, flushInterval),
	}
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
		log.Printf("control: redis unreachable at startup: %v (continuing with Redis-backed features degraded)", pingErr)
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
			log.Printf("control: config invalidation subscriber: %v (retrying)", err)

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
func buildControlMux(controlConfig config.ControlConfig, apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool, configStore *control.PostgresConfigStore, configCache *control.ConfigCache, redisClient *redis.Client, natsConn *natsx.Connection, chWriters *clickHouseWriters, metrics *control.Metrics) *http.ServeMux {
	inflight := control.NewInFlightRegistry()
	tenantStore := control.NewPostgresTenantStore(pool)
	adminHandlers := buildAdminHandlers(apiKeyStore, pepper, workerRegistry, workerCreds, pool, configStore, configCache, redisClient, inflight, tenantStore, chWriters.configAudit)

	authenticator := control.NewAuthenticator(apiKeyStore, pepper).SetTenantStore(tenantStore)

	var requestHandler *control.RequestHandler
	if chWriters.requestMetadata != nil {
		requestHandler = control.NewRequestHandler(
			controlConfig.Request.MaxInlineRequestBodyBytes,
			controlConfig.Request.MaxInlineResponseBodyBytes,
			controlConfig.Request.MaxTimeoutMs,
			authenticator,
			chWriters.requestMetadata,
		)
	} else {
		requestHandler = control.NewRequestHandler(
			controlConfig.Request.MaxInlineRequestBodyBytes,
			controlConfig.Request.MaxInlineResponseBodyBytes,
			controlConfig.Request.MaxTimeoutMs,
			authenticator,
		)
	}

	rateLimitAdmission := control.NewRateLimitAdmission(control.NewRateLimiter(redisClient, control.DefaultRateLimitGuardrails(), nil))
	rateLimitAdmission.SetMetrics(metrics)

	quotaAdmission := control.NewQuotaAdmission(redisClient, nil)
	quotaAdmission.SetMetrics(metrics)

	requestHandler.SetDispatcher(control.NewDefaultRequestDispatcher(control.RequestDispatcherOptions{
		ConfigCache:                configCache,
		Workers:                    workerRegistry,
		Sticky:                     control.NewRedisStickyStore(redisClient),
		NATS:                       natsConn,
		RateLimitAdmission:         rateLimitAdmission,
		QuotaAdmission:             quotaAdmission,
		MaxInlineResponseBodyBytes: controlConfig.Request.MaxInlineResponseBodyBytes,
		MaxFrameDataBytes:          controlConfig.Transport.MaxFrameDataBytes,
		MaxTimeoutMs:               controlConfig.Request.MaxTimeoutMs,
		InFlight:                   inflight,
		Metrics:                    metrics,
	}))

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/requests", requestHandler)
	serveAdminRoutes(mux, adminHandlers)

	return mux
}

// buildAdminHandlers constructs the AdminHandlers with the Postgres-backed
// stores and the Redis-backed runtime admission components
// (docs/tasks/p0/21). The rate limiter, quota admission, and sticky store are
// constructed against the live Redis client but not yet consumed on the
// request path; that wiring is docs/tasks/p0/24.
func buildAdminHandlers(apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool, configStore *control.PostgresConfigStore, configCache *control.ConfigCache, redisClient *redis.Client, inflight *control.InFlightRegistry, tenantStore control.TenantStore, configAuditEvents *control.ConfigAuditEventWriter) *control.AdminHandlers {
	rateLimiter := control.NewRateLimiter(redisClient, control.DefaultRateLimitGuardrails(), nil)

	// A plain interface-typed nil check on configAuditEvents would miss a
	// typed nil *ConfigAuditEventWriter (no clickhouse endpoint configured),
	// so only pass a non-nil pointer through as the ConfigAuditRecorder.
	var auditEvents control.ConfigAuditRecorder
	if configAuditEvents != nil {
		auditEvents = configAuditEvents
	}

	return &control.AdminHandlers{
		Authenticator: control.NewAuthenticator(apiKeyStore, pepper).SetTenantStore(tenantStore),
		APIKeys:       apiKeyStore,
		WorkerCreds:   workerCreds,
		Tenants:       tenantStore,
		Quotas:        control.NewPostgresQuotaStore(pool),
		RateLimits:    control.NewPostgresRateLimitConfigStore(pool),
		Audit:         control.NewAuditStoreWithEvents(control.NewPostgresAuditStore(pool), auditEvents),
		ConfigCache:   configCache,
		Workers:       workerRegistry,
		ConfigWrites:  configStore,
		WorkerAdmin:   configStore,
		InFlight:      inflight,
		Pepper:        pepper,

		RoutingRules:        configStore,
		ExecutorPools:       configStore,
		DenyRules:           configStore,
		InjectionPolicies:   configStore,
		FingerprintProfiles: configStore,

		RateLimiter:        rateLimiter,
		RateLimitAdmission: control.NewRateLimitAdmission(rateLimiter),
		QuotaAdmission:     control.NewQuotaAdmission(redisClient, nil),
		StickySessions:     control.NewRedisStickyStore(redisClient),
	}
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
	mux.HandleFunc("POST /tenants", h.CreateTenant)
	mux.HandleFunc("GET /api/v1/config/tenants", h.ListTenants)
	mux.HandleFunc("GET /api/v1/config/tenants/{id}", h.GetTenant)
	mux.HandleFunc("PUT /api/v1/config/tenants/{id}", h.UpdateTenant)
	mux.HandleFunc("DELETE /api/v1/config/tenants/{id}", h.SoftDeleteTenant)
	mux.HandleFunc("POST /platform-api-keys", h.CreatePlatformAPIKey)
	mux.HandleFunc("GET /platform-api-keys", h.ListPlatformAPIKeys)
	mux.HandleFunc("POST /platform-api-keys/{id}/revoke", h.RevokePlatformAPIKey)
	mux.HandleFunc("POST /api-keys", h.CreateTenantAPIKey)
	mux.HandleFunc("GET /api-keys", h.ListTenantAPIKeys)
	mux.HandleFunc("POST /api-keys/{id}/revoke", h.RevokeTenantAPIKey)
	mux.HandleFunc("POST /worker-credentials", h.CreateWorkerCredential)
	mux.HandleFunc("GET /worker-credentials", h.ListWorkerCredentials)
	mux.HandleFunc("POST /worker-credentials/{id}/revoke", h.RevokeWorkerCredential)
}

// serveConfigResourceRoutes registers quota, rate-limit, routing, deny,
// injection, and fingerprint config routes.
func serveConfigResourceRoutes(mux *http.ServeMux, h *control.AdminHandlers) {
	mux.HandleFunc("GET /quotas", h.GetQuotas)
	mux.HandleFunc("PUT /tenants/{id}/quotas", h.PutTenantQuotas)
	mux.HandleFunc("GET /rate-limits", h.GetRateLimits)
	mux.HandleFunc("PUT /rate-limits", h.PutRateLimits)
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
	log.Printf("control: listening on %s", addr)

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}
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

// bootstrapWorkerCredential seeds a dev worker credential from
// STRAW_BOOTSTRAP_WORKER_CREDENTIAL_ID/STRAW_BOOTSTRAP_WORKER_PUBLIC_KEY_ED25519_BASE64
// so the docker-compose egress worker can register out of the box (see
// deploy/docker/README.md). A no-op when either variable is unset.
func bootstrapWorkerCredential(store control.WorkerCredentialStore) error {
	created, err := control.BootstrapWorkerCredentialFromEnv(
		context.Background(),
		store,
		os.Getenv(control.DevWorkerIDEnvVar),
		os.Getenv(control.DevWorkerPublicEd25519EnvVar),
	)
	if err != nil {
		return fmt.Errorf("bootstrap worker credential: %w", err)
	}

	if created {
		log.Printf("control: bootstrapped dev worker credential from %s", control.DevWorkerIDEnvVar)
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
