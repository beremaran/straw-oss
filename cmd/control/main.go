// Package main runs the Straw control service.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const (
	exitUsage         = 2
	readHeaderTimeout = 5 * time.Second
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

	return runControl(controlConfig)
}

func runControl(controlConfig config.ControlConfig) error {
	apiKeyStore := control.NewInMemoryAPIKeyStore()
	pepper := []byte(os.Getenv("STRAW_API_KEY_PEPPER"))

	created, err := bootstrapAPIKey(apiKeyStore, pepper)
	if err != nil {
		return err
	}

	if created {
		log.Printf("control: bootstrapped first platform system_admin API key from %s", control.BootstrapSystemAdminEnvVar)
	}

	mux := buildControlMux(controlConfig, apiKeyStore, pepper)

	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.APIPort)
	log.Printf("control: listening on %s", addr)

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}

	err = server.ListenAndServe()
	if err != nil {
		return fmt.Errorf("listen and serve control http server: %w", err)
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

func buildControlMux(controlConfig config.ControlConfig, apiKeyStore control.APIKeyStore, pepper []byte) *http.ServeMux {
	authenticator := control.NewAuthenticator(apiKeyStore, pepper)
	snapshotStore := control.NewInMemorySnapshotStore()
	configCache := control.NewConfigCache(snapshotStore, nil)
	workerCreds := control.NewInMemoryWorkerCredentialStore()
	workerRegistry := control.NewWorkerRegistry(workerCreds, control.DefaultWorkerTimings(), nil)

	adminHandlers := &control.AdminHandlers{
		Authenticator: authenticator,
		APIKeys:       apiKeyStore,
		WorkerCreds:   workerCreds,
		Tenants:       control.NewInMemoryTenantStore(),
		Quotas:        control.NewInMemoryQuotaStore(),
		Audit:         control.NewInMemoryAuditStore(),
		ConfigCache:   configCache,
		Workers:       workerRegistry,
		Pepper:        pepper,
	}

	requestHandler := control.NewRequestHandler(
		controlConfig.Request.MaxInlineRequestBodyBytes,
		controlConfig.Request.MaxInlineResponseBodyBytes,
		controlConfig.Request.MaxTimeoutMs,
		authenticator,
	)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/requests", requestHandler)
	mux.HandleFunc("POST /tenants", adminHandlers.CreateTenant)
	mux.HandleFunc("POST /platform-api-keys", adminHandlers.CreatePlatformAPIKey)
	mux.HandleFunc("GET /platform-api-keys", adminHandlers.ListPlatformAPIKeys)
	mux.HandleFunc("POST /platform-api-keys/{id}/revoke", adminHandlers.RevokePlatformAPIKey)
	mux.HandleFunc("POST /api-keys", adminHandlers.CreateTenantAPIKey)
	mux.HandleFunc("GET /api-keys", adminHandlers.ListTenantAPIKeys)
	mux.HandleFunc("POST /api-keys/{id}/revoke", adminHandlers.RevokeTenantAPIKey)
	mux.HandleFunc("POST /worker-credentials", adminHandlers.CreateWorkerCredential)
	mux.HandleFunc("GET /worker-credentials", adminHandlers.ListWorkerCredentials)
	mux.HandleFunc("POST /worker-credentials/{id}/revoke", adminHandlers.RevokeWorkerCredential)
	mux.HandleFunc("GET /quotas", adminHandlers.GetQuotas)
	mux.HandleFunc("PUT /tenants/{id}/quotas", adminHandlers.PutTenantQuotas)
	mux.HandleFunc("GET /api/v1/admin/workers", adminHandlers.ListWorkers)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/disable", adminHandlers.DisableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/enable", adminHandlers.EnableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/drain", adminHandlers.DrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/undrain", adminHandlers.UndrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-disable", adminHandlers.TenantDisableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-enable", adminHandlers.TenantEnableWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-drain", adminHandlers.TenantDrainWorker)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/tenant-undrain", adminHandlers.TenantUndrainWorker)

	return mux
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
