package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const configTestUnknownField = "unknown field"

func TestLoadControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "valid",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090
					}
				},
				"egress": {
					"worker_id": "egress-local-001"
				}
			}`,
		},
		{
			name: "missing control section",
			config: `{
				"config_version": "v1",
				"egress": {
					"worker_id": "egress-local-001"
				}
			}`,
			wantErr: "missing control section",
		},
		{
			name: "invalid api port",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 0,
						"metrics_port": 9090
					}
				}
			}`,
			wantErr: "server.api_port must be between 1 and 65535",
		},
		{
			name: "valid proxy listener",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"proxy_enabled": true,
						"proxy_port": 8081
					}
				}
			}`,
		},
		{
			name: "valid mitm listener",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_enabled": true,
						"mitm_port": 8083,
						"mitm_ca_cert_file": "/tmp/ca.pem",
						"mitm_ca_key_file": "/tmp/ca-key.pem",
						"mitm_cert_validity_days": 45
					}
				}
			}`,
		},
		{
			name: "invalid enabled mitm port",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_enabled": true,
						"mitm_port": 8082,
						"mitm_ca_cert_file": "/tmp/ca.pem",
						"mitm_ca_key_file": "/tmp/ca-key.pem"
					}
				}
			}`,
			wantErr: "server.mitm_port must be 8083 when mitm_enabled is true",
		},
		{
			name: "enabled mitm requires ca files",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_enabled": true
					}
				}
			}`,
			wantErr: "server.mitm_ca_cert_file and server.mitm_ca_key_file are required",
		},
		{
			name: "invalid enabled proxy port",
			config: `{
					"config_version": "v1",
					"control": {
						"server": {
							"host": "127.0.0.1",
							"api_port": 8080,
							"metrics_port": 9090,
							"proxy_enabled": true,
							"proxy_port": 8082
						}
					}
				}`,
			wantErr: "server.proxy_port must be 8081 when proxy_enabled is true",
		},
		{
			name: configTestUnknownField,
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"extra": true
					}
				}
			}`,
			wantErr: configTestUnknownField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfig(t, tt.config)
			_, err := LoadControl(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("LoadControl() error = %v", err)
				}

				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadControl() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadControlDefaultsProxyPort(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"control": {
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"proxy_enabled": true
			}
		}
	}`)

	cfg, err := LoadControl(path)
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}
	if cfg.Server.ProxyPort != 8081 {
		t.Fatalf("proxy_port = %d, want 8081", cfg.Server.ProxyPort)
	}
}

func TestLoadControlDefaultsConnectPort(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"control": {
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"connect_enabled": true
			}
		}
	}`)

	cfg, err := LoadControl(path)
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}
	if cfg.Server.ConnectPort != 8082 {
		t.Fatalf("connect_port = %d, want 8082", cfg.Server.ConnectPort)
	}
}

func TestLoadControlDefaultsMITMPort(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"control": {
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"mitm_enabled": true,
				"mitm_ca_cert_file": "/tmp/ca.pem",
				"mitm_ca_key_file": "/tmp/ca-key.pem"
			}
		}
	}`)

	cfg, err := LoadControl(path)
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}
	if cfg.Server.MITMPort != 8083 {
		t.Fatalf("mitm_port = %d, want 8083", cfg.Server.MITMPort)
	}
	if cfg.Server.MITMCertValidityDays != 30 {
		t.Fatalf("mitm_cert_validity_days = %d, want 30", cfg.Server.MITMCertValidityDays)
	}
}

func TestLoadControlMITMValidityDaysEnv(t *testing.T) {
	t.Setenv("STRAW_MITM_CERT_VALIDITY_DAYS", "14")

	path := writeConfig(t, `{
		"config_version": "v1",
		"control": {
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"mitm_enabled": true,
				"mitm_ca_cert_file": "/tmp/ca.pem",
				"mitm_ca_key_file": "/tmp/ca-key.pem"
			}
		}
	}`)

	cfg, err := LoadControl(path)
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}
	if cfg.Server.MITMCertValidityDays != 14 {
		t.Fatalf("mitm_cert_validity_days = %d, want 14", cfg.Server.MITMCertValidityDays)
	}
}

func TestLoadControlMITMValidityDaysRejectsInvalidEnv(t *testing.T) {
	t.Setenv("STRAW_MITM_CERT_VALIDITY_DAYS", "nope")

	path := writeConfig(t, `{
		"config_version": "v1",
		"control": {
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"mitm_enabled": true,
				"mitm_ca_cert_file": "/tmp/ca.pem",
				"mitm_ca_key_file": "/tmp/ca-key.pem"
			}
		}
	}`)

	_, err := LoadControl(path)
	if err == nil || !strings.Contains(err.Error(), "server.mitm_cert_validity_days must be positive") {
		t.Fatalf("LoadControl() error = %v, want validity error", err)
	}
}

func TestLoadControlRedisDefaults(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"control": {
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090
			}
		}
	}`)

	cfg, err := LoadControl(path)
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}

	redisCfg := cfg.Database.Redis
	if redisCfg.URLEnv != "STRAW_REDIS_URL" {
		t.Fatalf("Database.Redis.URLEnv = %q, want STRAW_REDIS_URL", redisCfg.URLEnv)
	}

	if redisCfg.DialTimeoutMS != 2000 || redisCfg.ReadTimeoutMS != 500 || redisCfg.WriteTimeoutMS != 500 {
		t.Fatalf("Database.Redis timeouts = %+v, want {2000 500 500}", redisCfg)
	}
}

func TestLoadEgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "valid",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090
					}
				},
				"egress": {
					"worker_id": "egress-local-001",
					"credential_id": "wcred_test",
					"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64"
				}
			}`,
		},
		{
			name: "missing worker id",
			config: `{
				"config_version": "v1",
				"egress": {}
			}`,
			wantErr: "worker_id is required",
		},
		{
			name: "missing credential id",
			config: `{
				"config_version": "v1",
				"egress": {
					"worker_id": "egress-local-001"
				}
			}`,
			wantErr: "credential_id is required",
		},
		{
			name: "missing private key env",
			config: `{
				"config_version": "v1",
				"egress": {
					"worker_id": "egress-local-001",
					"credential_id": "wcred_test"
				}
			}`,
			wantErr: "private_key_ed25519_env is required",
		},
		{
			name: configTestUnknownField,
			config: `{
				"config_version": "v1",
				"egress": {
					"worker_id": "egress-local-001",
					"credential_id": "wcred_test",
					"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
					"extra": true
				}
			}`,
			wantErr: configTestUnknownField,
		},
		{
			name: "allowed pool missing pool id",
			config: `{
				"config_version": "v1",
				"egress": {
					"worker_id": "egress-local-001",
					"credential_id": "wcred_test",
					"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
					"allowed_pools": [{"tenant_id": "ten_x"}]
				}
			}`,
			wantErr: "allowed_pools entries require both tenant_id and pool_id",
		},
		{
			name: "invalid health port",
			config: `{
				"config_version": "v1",
				"egress": {
					"worker_id": "egress-local-001",
					"credential_id": "wcred_test",
					"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
					"health_port": 70000
				}
			}`,
			wantErr: "health_port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfig(t, tt.config)
			cfg, err := LoadEgress(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("LoadEgress() error = %v", err)
				}

				if cfg.HealthPort != defaultEgressHealthPort {
					t.Fatalf("HealthPort = %d, want default %d", cfg.HealthPort, defaultEgressHealthPort)
				}

				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadEgress() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadEgressParsesAllowedPools(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential_id": "wcred_test",
			"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
			"allowed_pools": [{"tenant_id": "ten_x", "pool_id": "pool_y"}]
		}
	}`)

	cfg, err := LoadEgress(path)
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}

	if len(cfg.AllowedPools) != 1 || cfg.AllowedPools[0].TenantID != "ten_x" || cfg.AllowedPools[0].PoolID != "pool_y" {
		t.Fatalf("AllowedPools = %+v, want [{ten_x pool_y}]", cfg.AllowedPools)
	}
}

func TestLoadEgressUpstreamConnectionPoolDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential_id": "wcred_test",
			"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
			"upstream_connection_pool": {"enabled": true}
		}
	}`)

	cfg, err := LoadEgress(path)
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}

	pool := cfg.UpstreamConnectionPool
	if !pool.Enabled || pool.MaxIdleConnsPerTenantHost != 2 || pool.IdleTimeoutMS != 30_000 || pool.MaxLifetimeMS != 300_000 {
		t.Fatalf("UpstreamConnectionPool = %+v, want enabled defaults", pool)
	}

	path = writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential_id": "wcred_test",
			"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
			"upstream_connection_pool": {"enabled": true, "idle_timeout_ms": -1}
		}
	}`)

	_, err = LoadEgress(path)
	if err == nil || !strings.Contains(err.Error(), "upstream_connection_pool values must be positive when enabled") {
		t.Fatalf("LoadEgress() error = %v, want upstream pool validation error", err)
	}
}

func TestLoadEgressParsesCapabilities(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential_id": "wcred_test",
			"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
			"capabilities": {
				"tags": ["datacenter", "local"],
				"countries": ["AU"],
				"regions": ["wa"],
				"ip_types": ["datacenter"],
				"supported_ingress_modes": ["rest"],
				"max_concurrency": 100
			}
		}
	}`)

	cfg, err := LoadEgress(path)
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}

	caps := cfg.Capabilities
	if !slices.Equal(caps.Tags, []string{"datacenter", "local"}) ||
		!slices.Equal(caps.Countries, []string{"AU"}) ||
		!slices.Equal(caps.Regions, []string{"wa"}) ||
		!slices.Equal(caps.IPTypes, []string{"datacenter"}) ||
		!slices.Equal(caps.SupportedIngressModes, []string{defaultIngressMode}) ||
		caps.MaxConcurrency != 100 {
		t.Fatalf("Capabilities = %+v, want the configured values", caps)
	}
}

func TestLoadEgressCapabilitiesDefaults(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential_id": "wcred_test",
			"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64"
		}
	}`)

	cfg, err := LoadEgress(path)
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}

	caps := cfg.Capabilities
	if len(caps.Tags) != 0 || len(caps.Countries) != 0 || len(caps.Regions) != 0 || len(caps.IPTypes) != 0 {
		t.Fatalf("Capabilities lists = %+v, want empty defaults", caps)
	}

	if !slices.Equal(caps.SupportedIngressModes, []string{defaultIngressMode}) {
		t.Fatalf("SupportedIngressModes = %v, want default [rest]", caps.SupportedIngressModes)
	}

	if caps.MaxConcurrency != 0 {
		t.Fatalf("MaxConcurrency = %d, want 0 (unset)", caps.MaxConcurrency)
	}
}

func TestLoadEgressCredentialKeysAreFlat(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential_id": "wcred_test",
			"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64"
		}
	}`)

	cfg, err := LoadEgress(path)
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}
	if cfg.WorkerID != "egress-local-001" {
		t.Fatalf("WorkerID = %q, want egress-local-001", cfg.WorkerID)
	}
	if cfg.CredentialID != "wcred_test" {
		t.Fatalf("CredentialID = %q, want wcred_test", cfg.CredentialID)
	}
	if cfg.PrivateKeyEd25519Env != "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64" {
		t.Fatalf("PrivateKeyEd25519Env = %q, want STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64", cfg.PrivateKeyEd25519Env)
	}

	path = writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential": {
				"credential_id_env": "STRAW_WORKER_CREDENTIAL_ID",
				"private_key_env": "STRAW_WORKER_PRIVATE_KEY"
			}
		}
	}`)

	_, err = LoadEgress(path)
	if err == nil || !strings.Contains(err.Error(), "credential_id is required") {
		t.Fatalf("LoadEgress() error = %v, want substring %q", err, "credential_id is required")
	}
}

func TestLoadEgressHTTP2Defaults(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"egress": {
			"worker_id": "egress-local-001",
			"credential_id": "wcred_test",
			"private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
			"http2": {
				"enabled": true
			}
		}
	}`)

	cfg, err := LoadEgress(path)
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}

	if !cfg.HTTP2.Enabled {
		t.Fatalf("HTTP2.Enabled = false, want true")
	}
	if cfg.HTTP2.FallbackCacheTTLMS != 300_000 {
		t.Fatalf("HTTP2.FallbackCacheTTLMS = %d, want 300000", cfg.HTTP2.FallbackCacheTTLMS)
	}
}

func TestLoadControlHTTP2(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `{
		"config_version": "v1",
		"control": {
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090
			},
			"http2": {
				"enabled": true
			}
		}
	}`)

	cfg, err := LoadControl(path)
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}

	if !cfg.HTTP2.Enabled {
		t.Fatalf("HTTP2.Enabled = false, want true")
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}
