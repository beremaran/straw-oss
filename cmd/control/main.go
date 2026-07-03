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
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/postgresx"
	"github.com/beremaran/straw/v2/internal/redisx"
	"github.com/beremaran/straw/v2/migrations"
)

const (
	exitUsage              = 2
	readHeaderTimeout      = 5 * time.Second
	controlShutdownTimeout = 5 * time.Second
	redisPingTimeout       = 2 * time.Second
	invalidationPollPeriod = 30 * time.Second
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("control: %v", err)
	}
}

func run() error {
	controlConfig, err := loadControlConfig()
	if err != nil {
		return fmt.Errorf("load control config: %w", err)
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

	// Postgres is the control-plane source of truth for identity state and is
	// required at startup (docs/planning/21-state-and-storage.md).
	pool, err := openPostgres(controlConfig.Database.Postgres)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}

	defer pool.Close()

	// Redis is ephemeral runtime state only (docs/planning/21). A Redis
	// outage degrades rate limits, quotas, sticky sessions, and invalidation
	// acceleration per their configured fail policies; it must not block
	// Control startup the way a missing Postgres does.
	redisClient, err := openRedis(controlConfig.Database.Redis)
	if err != nil {
		return fmt.Errorf("open redis: %w", err)
	}

	defer func() {
		closeErr := redisClient.Close()
		if closeErr != nil {
			log.Printf("control: close redis client: %v", closeErr)
		}
	}()

	apiKeyStore, err := setupAPIKeyStore(pool, pepper)
	if err != nil {
		return err
	}

	workerCreds := control.NewPostgresWorkerCredentialStore(pool)
	workerRegistry := control.NewWorkerRegistry(workerCreds, control.DefaultWorkerTimings(), nil)

	configStore := control.NewPostgresConfigStore(pool)

	err = rehydrateWorkerAdminState(context.Background(), configStore, workerRegistry)
	if err != nil {
		return fmt.Errorf("rehydrate worker admin state: %w", err)
	}

	configCache := wireConfigInvalidation(ctx, configStore, redisClient)

	mux := buildControlMux(controlConfig, apiKeyStore, pepper, workerRegistry, workerCreds, pool, configStore, configCache, redisClient, natsConn)

	err = control.SetupWorkerDiscoverySubscriptions(natsConn, workerRegistry)
	if err != nil {
		return fmt.Errorf("setup worker discovery: %w", err)
	}

	return serveControlHTTP(ctx, controlConfig, mux)
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
func buildControlMux(controlConfig config.ControlConfig, apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool, configStore *control.PostgresConfigStore, configCache *control.ConfigCache, redisClient *redis.Client, natsConn *natsx.Connection) *http.ServeMux {
	adminHandlers := buildAdminHandlers(apiKeyStore, pepper, workerRegistry, workerCreds, pool, configStore, configCache, redisClient)
	requestHandler := control.NewRequestHandler(
		controlConfig.Request.MaxInlineRequestBodyBytes,
		controlConfig.Request.MaxInlineResponseBodyBytes,
		controlConfig.Request.MaxTimeoutMs,
		control.NewAuthenticator(apiKeyStore, pepper),
	)
	requestHandler.SetDispatcher(control.NewDefaultRequestDispatcher(control.RequestDispatcherOptions{
		ConfigCache:                configCache,
		Workers:                    workerRegistry,
		Sticky:                     control.NewRedisStickyStore(redisClient),
		NATS:                       natsConn,
		RateLimitAdmission:         control.NewRateLimitAdmission(control.NewRateLimiter(redisClient, control.DefaultRateLimitGuardrails(), nil)),
		QuotaAdmission:             control.NewQuotaAdmission(redisClient, nil),
		MaxInlineResponseBodyBytes: controlConfig.Request.MaxInlineResponseBodyBytes,
		MaxFrameDataBytes:          controlConfig.Transport.MaxFrameDataBytes,
		MaxTimeoutMs:               controlConfig.Request.MaxTimeoutMs,
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
func buildAdminHandlers(apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool, configStore *control.PostgresConfigStore, configCache *control.ConfigCache, redisClient *redis.Client) *control.AdminHandlers {
	rateLimiter := control.NewRateLimiter(redisClient, control.DefaultRateLimitGuardrails(), nil)

	return &control.AdminHandlers{
		Authenticator: control.NewAuthenticator(apiKeyStore, pepper),
		APIKeys:       apiKeyStore,
		WorkerCreds:   workerCreds,
		Tenants:       control.NewPostgresTenantStore(pool),
		Quotas:        control.NewPostgresQuotaStore(pool),
		RateLimits:    control.NewPostgresRateLimitConfigStore(pool),
		Audit:         control.NewPostgresAuditStore(pool),
		ConfigCache:   configCache,
		Workers:       workerRegistry,
		ConfigWrites:  configStore,
		WorkerAdmin:   configStore,
		Pepper:        pepper,

		RoutingRules:        configStore,
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
	mux.HandleFunc("POST /tenants", h.CreateTenant)
	mux.HandleFunc("POST /platform-api-keys", h.CreatePlatformAPIKey)
	mux.HandleFunc("GET /platform-api-keys", h.ListPlatformAPIKeys)
	mux.HandleFunc("POST /platform-api-keys/{id}/revoke", h.RevokePlatformAPIKey)
	mux.HandleFunc("POST /api-keys", h.CreateTenantAPIKey)
	mux.HandleFunc("GET /api-keys", h.ListTenantAPIKeys)
	mux.HandleFunc("POST /api-keys/{id}/revoke", h.RevokeTenantAPIKey)
	mux.HandleFunc("POST /worker-credentials", h.CreateWorkerCredential)
	mux.HandleFunc("GET /worker-credentials", h.ListWorkerCredentials)
	mux.HandleFunc("POST /worker-credentials/{id}/revoke", h.RevokeWorkerCredential)
	mux.HandleFunc("GET /quotas", h.GetQuotas)
	mux.HandleFunc("PUT /tenants/{id}/quotas", h.PutTenantQuotas)
	mux.HandleFunc("GET /rate-limits", h.GetRateLimits)
	mux.HandleFunc("PUT /rate-limits", h.PutRateLimits)
	mux.HandleFunc("GET /api/v1/config/routing-rules", h.ListRoutingRules)
	mux.HandleFunc("POST /api/v1/config/routing-rules", h.CreateRoutingRule)
	mux.HandleFunc("PUT /api/v1/config/routing-rules/{id}", h.UpdateRoutingRule)
	mux.HandleFunc("DELETE /api/v1/config/routing-rules/{id}", h.DeleteRoutingRule)
	mux.HandleFunc("GET /api/v1/config/deny-rules", h.ListDenyRules)
	mux.HandleFunc("POST /api/v1/config/deny-rules", h.CreateDenyRule)
	mux.HandleFunc("PUT /api/v1/config/deny-rules/{id}", h.UpdateDenyRule)
	mux.HandleFunc("DELETE /api/v1/config/deny-rules/{id}", h.DeleteDenyRule)
	mux.HandleFunc("GET /api/v1/config/injection-policies", h.ListInjectionPolicies)
	mux.HandleFunc("POST /api/v1/config/injection-policies", h.CreateInjectionPolicy)
	mux.HandleFunc("PUT /api/v1/config/injection-policies/{id}", h.UpdateInjectionPolicy)
	mux.HandleFunc("DELETE /api/v1/config/injection-policies/{id}", h.DeleteInjectionPolicy)
	mux.HandleFunc("GET /api/v1/config/fingerprint-profiles", h.ListFingerprintProfiles)
	mux.HandleFunc("GET /api/v1/admin/workers", h.ListWorkers)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/disable", h.DisableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/enable", h.EnableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/drain", h.DrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/undrain", h.UndrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-disable", h.TenantDisableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-enable", h.TenantEnableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-drain", h.TenantDrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-undrain", h.TenantUndrainWorker)
}

func serveControlHTTP(ctx context.Context, controlConfig config.ControlConfig, mux *http.ServeMux) error {
	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.APIPort)
	log.Printf("control: listening on %s", addr)

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}
	serveErr := make(chan error, 1)

	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
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

func loadControlConfig() (config.ControlConfig, error) {
	configPath := flag.String("config", "", "path to the control config file")

	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")

		os.Exit(exitUsage)
	}

	controlConfig, err := config.LoadControl(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return config.ControlConfig{}, fmt.Errorf("load control config: %w", err)
	}

	return controlConfig, nil
}
