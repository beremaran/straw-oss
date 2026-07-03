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

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/postgresx"
	"github.com/beremaran/straw/v2/migrations"
)

const (
	exitUsage              = 2
	readHeaderTimeout      = 5 * time.Second
	controlShutdownTimeout = 5 * time.Second
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

	pepper := []byte(os.Getenv("STRAW_API_KEY_PEPPER"))

	// Postgres is the control-plane source of truth for identity state and is
	// required at startup (docs/planning/21-state-and-storage.md).
	pool, err := openPostgres(controlConfig.Database.Postgres)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}

	defer pool.Close()

	apiKeyStore := control.NewPostgresAPIKeyStore(pool, pepper)

	created, err := bootstrapAPIKey(apiKeyStore, pepper)
	if err != nil {
		return err
	}

	if created {
		log.Printf("control: bootstrapped first platform system_admin API key from %s", control.BootstrapSystemAdminEnvVar)
	}

	workerCreds := control.NewPostgresWorkerCredentialStore(pool)
	workerRegistry := control.NewWorkerRegistry(workerCreds, control.DefaultWorkerTimings(), nil)

	mux := buildControlMux(controlConfig, apiKeyStore, pepper, workerRegistry, workerCreds, pool)

	err = control.SetupWorkerDiscoverySubscriptions(natsConn, workerRegistry)
	if err != nil {
		return fmt.Errorf("setup worker discovery: %w", err)
	}

	return serveControlHTTP(controlConfig, mux)
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

// buildControlMux assembles the HTTP handler with the Postgres-backed identity
// stores.
func buildControlMux(controlConfig config.ControlConfig, apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool) *http.ServeMux {
	adminHandlers := buildAdminHandlers(apiKeyStore, pepper, workerRegistry, workerCreds, pool)
	requestHandler := control.NewRequestHandler(
		controlConfig.Request.MaxInlineRequestBodyBytes,
		controlConfig.Request.MaxInlineResponseBodyBytes,
		controlConfig.Request.MaxTimeoutMs,
		control.NewAuthenticator(apiKeyStore, pepper),
	)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/requests", requestHandler)
	serveAdminRoutes(mux, adminHandlers)

	return mux
}

// buildAdminHandlers constructs the AdminHandlers with the Postgres-backed
// identity stores this task owns (tenants, API keys, worker credentials, audit).
// Quota, rate-limit, config-cache, and snapshot stores stay in-memory here until
// their owning tasks back them with Postgres/Redis (docs/tasks/p0/19, 20, 21).
func buildAdminHandlers(apiKeyStore control.APIKeyStore, pepper []byte, workerRegistry *control.WorkerRegistry, workerCreds control.WorkerCredentialStore, pool *pgxpool.Pool) *control.AdminHandlers {
	snapshotStore := control.NewInMemorySnapshotStore()

	return &control.AdminHandlers{
		Authenticator: control.NewAuthenticator(apiKeyStore, pepper),
		APIKeys:       apiKeyStore,
		WorkerCreds:   workerCreds,
		Tenants:       control.NewPostgresTenantStore(pool),
		Quotas:        control.NewInMemoryQuotaStore(),
		RateLimits:    control.NewInMemoryRateLimitConfigStore(),
		Audit:         control.NewPostgresAuditStore(pool),
		ConfigCache:   control.NewConfigCache(snapshotStore, nil),
		Workers:       workerRegistry,
		Pepper:        pepper,
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

func serveControlHTTP(controlConfig config.ControlConfig, mux *http.ServeMux) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.APIPort)
	log.Printf("control: listening on %s", addr)

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}
	serveErr := make(chan error, 1)

	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), controlShutdownTimeout)
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
