package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMinimalControl(t *testing.T) {
	t.Parallel()

	cfg, err := LoadControl(writeConfig(t, `{"config_version":"v1","control":{}}`))
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}
	if cfg.Server.APIPort != 8080 || cfg.Server.MetricsPort != 9090 || cfg.NATS.Servers[0] != defaultNATSServer {
		t.Fatalf("defaults = %+v", cfg)
	}
}

func TestLoadMinimalEgress(t *testing.T) {
	t.Parallel()

	cfg, err := LoadEgress(writeConfig(t, `{"config_version":"v1","egress":{}}`))
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}
	if cfg.WorkerID != "egress-1" || cfg.HealthPort != defaultEgressHealthPort || cfg.NATS.Servers[0] != defaultNATSServer {
		t.Fatalf("defaults = %+v", cfg)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	t.Parallel()

	_, err := LoadControl(writeConfig(t, `{"config_version":"v1","control":{"enterprise_mode":true}}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadControl() error = %v, want unknown field", err)
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	_, err := LoadEgress(writeConfig(t, `{"config_version":"v1","egress":{}} {}`))
	if !errors.Is(err, errUnexpectedTrailingJSON) {
		t.Fatalf("LoadEgress() error = %v, want trailing JSON", err)
	}
}

func TestLoadRedisRuntimeState(t *testing.T) {
	t.Parallel()
	cfg, err := LoadControl(writeConfig(t, `{"config_version":"v1","control":{"runtime_state":{"backend":"redis","redis_url_env":"REDIS_URL","key_prefix":"test"}}}`))
	if err != nil {
		t.Fatalf("LoadControl() error = %v", err)
	}
	if cfg.RuntimeState.Backend != "redis" || cfg.RuntimeState.RedisURLEnv != "REDIS_URL" || cfg.RuntimeState.WorkerTTLMS != 30000 {
		t.Fatalf("runtime state defaults = %+v", cfg.RuntimeState)
	}
}

func TestLoadRejectsInvalidRuntimeState(t *testing.T) {
	t.Parallel()
	_, err := LoadControl(writeConfig(t, `{"config_version":"v1","control":{"runtime_state":{"backend":"postgres"}}}`))
	if !errors.Is(err, errInvalidRuntimeState) {
		t.Fatalf("LoadControl() error = %v", err)
	}
}

func TestLoadObjectStorageProfiles(t *testing.T) {
	t.Parallel()
	cfg, err := LoadControl(writeConfig(t, `{"config_version":"v1","control":{"object_storage":{"enabled":true,"backend":"s3","endpoint":"https://s3.example","bucket":"receipts","server_side_encryption":"AES256"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ObjectStorage.MaxObjectBytes != 1<<30 || cfg.ObjectStorage.SigningKeyEnv != "STRAW_RECEIPT_SIGNING_KEY" {
		t.Fatalf("object storage defaults = %+v", cfg.ObjectStorage)
	}
	_, err = LoadControl(writeConfig(t, `{"config_version":"v1","control":{"object_storage":{"enabled":true,"backend":"s3"}}}`))
	if !errors.Is(err, errInvalidObjectStorage) {
		t.Fatalf("invalid S3 config = %v", err)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return path
}
