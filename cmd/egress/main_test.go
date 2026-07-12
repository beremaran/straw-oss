package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	internalegress "github.com/beremaran/straw-oss/internal/egress"
	"github.com/beremaran/straw-oss/internal/natsx"
	sdkegress "github.com/beremaran/straw-oss/sdk/egress"
)

func TestBuildCapabilitiesUsesDeploymentPool(t *testing.T) {
	t.Parallel()

	caps := buildCapabilities(config.DefaultEgress())
	if len(caps.AllowedPools) != 1 || caps.AllowedPools[0].GetTenantId() != config.DefaultDeploymentID || caps.AllowedPools[0].GetPoolId() != config.DefaultPoolID {
		t.Fatalf("AllowedPools = %+v, want deployment default", caps.AllowedPools)
	}
	if caps.MaxConcurrency != defaultConcurrency {
		t.Fatalf("MaxConcurrency = %d, want %d", caps.MaxConcurrency, defaultConcurrency)
	}
}

func TestRunWorkerUsesSDKRuntime(t *testing.T) {
	original := runSDKWorker
	t.Cleanup(func() { runSDKWorker = original })

	called := false
	runSDKWorker = func(_ context.Context, _ *natsx.Connection, id sdkegress.Identity, _ sdkegress.Capabilities, executor *internalegress.Executor, heartbeat time.Duration, _ *atomic.Bool) error {
		called = true
		if id.WorkerID != "worker-sdk" || id.ExecutorType != "egress" || len(id.PrivateKey) == 0 {
			t.Fatalf("identity = %+v", id)
		}
		if executor == nil || heartbeat != 250*time.Millisecond {
			t.Fatalf("executor/heartbeat = %v/%s", executor, heartbeat)
		}

		return nil
	}

	err := runWorker(context.Background(), nil, config.EgressConfig{WorkerID: "worker-sdk", HeartbeatIntervalMs: 250, HealthPort: 8090})
	if err != nil || !called {
		t.Fatalf("runWorker() = %v, called=%v", err, called)
	}
}
