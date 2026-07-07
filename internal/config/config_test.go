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
					"deployment_id": "dep_test",
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
					"deployment_id": "dep_test",
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
					"deployment_id": "dep_test",
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_enabled": true,
						"mitm_port": 8083,
						"mitm_ca_cert_file": "/tmp/ca.pem",
						"mitm_ca_key_file": "/tmp/ca-key.pem",
						"mitm_cert_validity_days": 45,
						"mitm_leaf_kms_provider": "aws-kms",
						"mitm_leaf_kms_key_id": "arn:aws:kms:us-west-2:123:key/abc"
					}
				}
			}`,
		},
		{
			name: "mitm leaf kms provider requires key id",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_leaf_kms_provider": "aws-kms"
					}
				}
			}`,
			wantErr: "server.mitm_leaf_kms_provider and server.mitm_leaf_kms_key_id must be supplied together",
		},
		{
			name: "mitm leaf kms key id requires provider",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_leaf_kms_key_id": "key"
					}
				}
			}`,
			wantErr: "server.mitm_leaf_kms_provider and server.mitm_leaf_kms_key_id must be supplied together",
		},
		{
			name: "mitm leaf kms rejects plaintext provider",
			config: `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_leaf_kms_provider": "plaintext",
						"mitm_leaf_kms_key_id": "key"
					}
				}
			}`,
			wantErr: "server.mitm_leaf_kms_provider must not be plaintext or static-key",
		},
		{
			name: "invalid enabled mitm port",
			config: `{
				"config_version": "v1",
				"control": {
					"deployment_id": "dep_test",
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_enabled": true,
						"mitm_port": 8082,
						"mitm_ca_cert_file": "/tmp/ca.pem",
						"mitm_ca_key_file": "/tmp/ca-key.pem",
						"mitm_leaf_kms_provider": "aws-kms",
						"mitm_leaf_kms_key_id": "arn:test"
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
					"deployment_id": "dep_test",
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090,
						"mitm_enabled": true,
						"mitm_leaf_kms_provider": "aws-kms",
						"mitm_leaf_kms_key_id": "arn:test"
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
			"deployment_id": "dep_test",
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"mitm_enabled": true,
				"mitm_ca_cert_file": "/tmp/ca.pem",
				"mitm_ca_key_file": "/tmp/ca-key.pem",
				"mitm_leaf_kms_provider": "aws-kms",
				"mitm_leaf_kms_key_id": "arn:test"
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
			"deployment_id": "dep_test",
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"mitm_enabled": true,
				"mitm_ca_cert_file": "/tmp/ca.pem",
				"mitm_ca_key_file": "/tmp/ca-key.pem",
				"mitm_leaf_kms_provider": "aws-kms",
				"mitm_leaf_kms_key_id": "arn:test"
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
			"deployment_id": "dep_test",
			"server": {
				"host": "127.0.0.1",
				"api_port": 8080,
				"metrics_port": 9090,
				"mitm_enabled": true,
				"mitm_ca_cert_file": "/tmp/ca.pem",
				"mitm_ca_key_file": "/tmp/ca-key.pem",
				"mitm_leaf_kms_provider": "aws-kms",
				"mitm_leaf_kms_key_id": "arn:test"
			}
		}
	}`)

	_, err := LoadControl(path)
	if err == nil || !strings.Contains(err.Error(), "server.mitm_cert_validity_days must be positive") {
		t.Fatalf("LoadControl() error = %v, want validity error", err)
	}
}

func TestLoadControlMITMRequiresDeploymentID(t *testing.T) {
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
				"mitm_ca_key_file": "/tmp/ca-key.pem",
				"mitm_leaf_kms_provider": "aws-kms",
				"mitm_leaf_kms_key_id": "arn:test"
			}
		}
	}`)

	_, err := LoadControl(path)
	if err == nil || !strings.Contains(err.Error(), "control.deployment_id is required") {
		t.Fatalf("LoadControl() error = %v, want deployment id error", err)
	}
}

func TestLoadControlDeploymentIDEnv(t *testing.T) {
	t.Setenv("STRAW_CONTROL_DEPLOYMENT_ID", "dep_env")

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
	if cfg.DeploymentID != "dep_env" {
		t.Fatalf("DeploymentID = %q, want dep_env", cfg.DeploymentID)
	}
}

func TestLoadControlMITMLeafKMSEnv(t *testing.T) {
	t.Setenv("STRAW_MITM_LEAF_KMS_PROVIDER", "aws-kms")
	t.Setenv("STRAW_MITM_LEAF_KMS_KEY_ID", "arn:test")

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
	if cfg.Server.MITMLeafKMSProvider != "aws-kms" || cfg.Server.MITMLeafKMSKeyID != "arn:test" {
		t.Fatalf("MITM leaf KMS config = %q/%q", cfg.Server.MITMLeafKMSProvider, cfg.Server.MITMLeafKMSKeyID)
	}
}

func TestLoadControlBodyTransportDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
		check   func(t *testing.T, cfg ControlConfig)
	}{
		{
			name: "defaults",
			body: `{}`,
			check: func(t *testing.T, cfg ControlConfig) {
				t.Helper()

				if cfg.BodyTransport.LargeBodyThresholdBytes != DefaultLargeBodyThresholdBytes {
					t.Fatalf("large_body_threshold_bytes = %d, want %d", cfg.BodyTransport.LargeBodyThresholdBytes, DefaultLargeBodyThresholdBytes)
				}
				if cfg.BodyTransport.ResponseBodyMode != BodyResponseModeStreamThroughControlTeeObjectStorage {
					t.Fatalf("response_body_mode = %q, want resolved mode", cfg.BodyTransport.ResponseBodyMode)
				}
				if cfg.BodyTransport.ObjectStorage.BodyRetentionDays != DefaultBodyRetentionDays {
					t.Fatalf("body_retention_days = %d, want %d", cfg.BodyTransport.ObjectStorage.BodyRetentionDays, DefaultBodyRetentionDays)
				}
			},
		},
		{
			name: "valid object storage and direct stream",
			body: `{
				"large_body_threshold_bytes": 2097152,
				"object_storage": {
					"enabled": true,
					"endpoint": "https://s3.example",
					"bucket": "straw-bodies",
					"region": "us-west-2",
					"access_key_env": "STRAW_S3_ACCESS_KEY",
					"secret_key_env": "STRAW_S3_SECRET_KEY",
					"body_retention_days": 3
				},
				"direct_stream": {
					"enabled": true,
					"endpoint": "http://stream.test"
				}
			}`,
			check: func(t *testing.T, cfg ControlConfig) {
				t.Helper()

				if cfg.BodyTransport.LargeBodyThresholdBytes != 2_097_152 {
					t.Fatalf("large_body_threshold_bytes = %d, want 2097152", cfg.BodyTransport.LargeBodyThresholdBytes)
				}
				if !cfg.BodyTransport.ObjectStorage.Enabled || !cfg.BodyTransport.DirectStream.Enabled {
					t.Fatalf("enabled transports = object:%v direct:%v, want both true", cfg.BodyTransport.ObjectStorage.Enabled, cfg.BodyTransport.DirectStream.Enabled)
				}
				if cfg.BodyTransport.DirectStream.StreamTimeoutMS != DefaultDirectStreamTimeoutMS {
					t.Fatalf("stream_timeout_ms = %d, want default", cfg.BodyTransport.DirectStream.StreamTimeoutMS)
				}
			},
		},
		{
			name: "unsupported response mode",
			body: `{
				"response_body_mode": "executor_writes_object_read_after_completion"
			}`,
			wantErr: "body_transport.response_body_mode is unsupported",
		},
		{
			name: "retention too long",
			body: `{
				"object_storage": {
					"body_retention_days": 4
				}
			}`,
			wantErr: "body_transport.object_storage.body_retention_days must be between 1 and 3",
		},
		{
			name: "direct stream timeout invalid",
			body: `{
				"direct_stream": {
					"enabled": true,
					"stream_timeout_ms": -1
				}
			}`,
			wantErr: "body_transport.direct_stream.stream_timeout_ms must be positive",
		},
		{
			name: "object storage enabled but incomplete",
			body: `{
				"object_storage": {
					"enabled": true,
					"endpoint": "https://s3.example",
					"bucket": "straw-bodies"
				}
			}`,
			wantErr: "body_transport.object_storage requires endpoint, bucket, region",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfig(t, `{
				"config_version": "v1",
				"control": {
					"server": {
						"host": "127.0.0.1",
						"api_port": 8080,
						"metrics_port": 9090
					},
					"body_transport": `+tt.body+`
				}
			}`)

			cfg, err := LoadControl(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadControl() error = %v, want substring %q", err, tt.wantErr)
				}

				return
			}
			if err != nil {
				t.Fatalf("LoadControl() error = %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
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
