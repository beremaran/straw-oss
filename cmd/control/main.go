package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
	"github.com/beremaran/straw/v2/internal/natsx"
)

func main() {
	configPath := flag.String("config", "", "path to the control config file")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")
		os.Exit(2)
	}

	controlConfig, err := config.LoadControl(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := natsx.ValidateServers(controlConfig.NATS.Servers); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := natsx.ValidateMaxPayload(controlConfig.NATS.MaxPayloadBytes, controlConfig.Transport.MaxFrameDataBytes, controlConfig.Request.MaxInlineRequestBodyBytes, controlConfig.Request.MaxInlineResponseBodyBytes); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// P0 stores are process-local; a Postgres-backed implementation is
	// future work once a database driver dependency is introduced for
	// Control (see docs/agents/handoffs/07-auth-rbac-api-keys.md).
	apiKeyStore := control.NewInMemoryAPIKeyStore()
	pepper := []byte(os.Getenv("STRAW_API_KEY_PEPPER"))

	if _, created, err := control.BootstrapFromEnv(context.Background(), apiKeyStore, os.Getenv(control.BootstrapSystemAdminEnvVar), pepper); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	} else if created {
		log.Printf("control: bootstrapped first platform system_admin API key from %s", control.BootstrapSystemAdminEnvVar)
	}

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

	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.APIPort)
	log.Printf("control: listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("control: %v", err)
	}
}
