package main

import (
	"slices"
	"testing"

	"github.com/beremaran/straw/v2/internal/config"
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
