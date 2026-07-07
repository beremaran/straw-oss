package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
	internalegress "github.com/beremaran/straw/v2/internal/egress"
	"github.com/beremaran/straw/v2/internal/natsx"
	sdkegress "github.com/beremaran/straw/v2/sdk/egress"
)

func TestBuildCapabilitiesFromConfig(t *testing.T) {
	t.Parallel()

	cfg := config.EgressConfig{
		AllowedPools: []config.EgressPoolRef{{TenantID: "ten_x", PoolID: "pool_y"}},
		Capabilities: config.EgressCapabilities{
			Tags:                  []string{"datacenter", "local"},
			Countries:             []string{"AU"},
			Regions:               []string{"wa"},
			IPTypes:               []string{"datacenter"},
			SupportedIngressModes: []string{"rest"},
			MaxConcurrency:        100,
		},
	}

	caps := buildCapabilities(cfg)

	if !slices.Equal(caps.Tags, cfg.Capabilities.Tags) ||
		!slices.Equal(caps.Countries, cfg.Capabilities.Countries) ||
		!slices.Equal(caps.Regions, cfg.Capabilities.Regions) ||
		!slices.Equal(caps.IPTypes, cfg.Capabilities.IPTypes) ||
		!slices.Equal(caps.SupportedIngressModes, cfg.Capabilities.SupportedIngressModes) {
		t.Fatalf("capabilities = %+v, want the configured claims", caps)
	}

	if caps.MaxConcurrency != 100 {
		t.Fatalf("MaxConcurrency = %d, want 100", caps.MaxConcurrency)
	}

	if len(caps.AllowedPools) != 1 || caps.AllowedPools[0].GetTenantId() != "ten_x" || caps.AllowedPools[0].GetPoolId() != "pool_y" {
		t.Fatalf("AllowedPools = %+v, want [{ten_x pool_y}]", caps.AllowedPools)
	}
}

func TestBuildCapabilitiesDefaultsMaxConcurrency(t *testing.T) {
	t.Parallel()

	caps := buildCapabilities(config.EgressConfig{})
	if caps.MaxConcurrency != defaultConcurrency {
		t.Fatalf("MaxConcurrency = %d, want default %d", caps.MaxConcurrency, defaultConcurrency)
	}
}

func TestRunWorkerUsesSDKRuntime(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	t.Setenv("TEST_EGRESS_PRIVATE_KEY", base64.StdEncoding.EncodeToString(seed))

	orig := runSDKWorker
	t.Cleanup(func() { runSDKWorker = orig })

	called := false
	runSDKWorker = func(_ context.Context, _ *natsx.Connection, id sdkegress.Identity, caps sdkegress.Capabilities, executor *internalegress.Executor, heartbeatInterval time.Duration, ready *atomic.Bool) error {
		called = true

		if id.WorkerID != "worker_sdk" || id.CredentialID != "cred_sdk" || id.ExecutorType != "egress" {
			t.Fatalf("identity = %+v, want configured worker identity", id)
		}

		if caps.MaxConcurrency != 7 {
			t.Fatalf("MaxConcurrency = %d, want 7", caps.MaxConcurrency)
		}

		if executor == nil {
			t.Fatal("executor is nil")
		}

		if heartbeatInterval != 250*time.Millisecond {
			t.Fatalf("heartbeatInterval = %s, want 250ms", heartbeatInterval)
		}

		if ready == nil {
			t.Fatal("ready flag is nil")
		}

		return nil
	}

	err := runWorker(context.Background(), nil, config.EgressConfig{
		WorkerID:             "worker_sdk",
		CredentialID:         "cred_sdk",
		PrivateKeyEd25519Env: "TEST_EGRESS_PRIVATE_KEY",
		HeartbeatIntervalMs:  250,
		Capabilities: config.EgressCapabilities{
			MaxConcurrency: 7,
		},
	})
	if err != nil {
		t.Fatalf("runWorker() error = %v", err)
	}

	if !called {
		t.Fatal("runSDKWorker was not called")
	}
}
