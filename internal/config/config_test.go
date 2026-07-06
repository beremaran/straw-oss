package config

import (
	"os"
	"path/filepath"
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
