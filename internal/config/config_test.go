package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
			name: "unknown field",
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
			wantErr: "unknown field",
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
					"worker_id": "egress-local-001"
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
			name: "unknown field",
			config: `{
				"config_version": "v1",
				"egress": {
					"worker_id": "egress-local-001",
					"extra": true
				}
			}`,
			wantErr: "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeConfig(t, tt.config)
			_, err := LoadEgress(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("LoadEgress() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadEgress() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
