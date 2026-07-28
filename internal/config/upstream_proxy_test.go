package config

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const upstreamProxyTestProfileID = "provider-profile"

func TestUpstreamProxyLimitsMatchProtocol(t *testing.T) {
	t.Parallel()

	if MaxUpstreamProxyIDBytes != strawpb.MaxUpstreamProxyIDBytes ||
		MaxUpstreamProxyRegionBytes != strawpb.MaxUpstreamProxyRegionBytes ||
		MaxUpstreamProxyEndpointBytes != strawpb.MaxUpstreamProxyEndpointBytes ||
		MaxUpstreamProxyUsernameTemplateBytes != strawpb.MaxUpstreamProxyUsernameTemplateBytes ||
		MaxEnvVarNameBytes != strawpb.MaxEnvVarNameBytes ||
		MaxUpstreamProxyRenderedCredentialBytes != strawpb.MaxUpstreamProxyRenderedCredentialBytes ||
		MaxProxyAuthorizationBytes != strawpb.MaxProxyAuthorizationBytes {
		t.Fatal("upstream proxy limits differ from straw-protos-go")
	}
}

func TestLoadEgressUpstreamProxy(t *testing.T) {
	t.Setenv("STRAW_TEST_PROXY_USERNAME", "account")
	t.Setenv("STRAW_TEST_PROXY_PASSWORD", "")

	cfg, err := loadEgressValue(t, validEgressUpstreamProxyConfig())
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}

	pool := cfg.Capabilities.AllowedPools[0]
	if pool.UpstreamProxyID != upstreamProxyTestProfileID {
		t.Fatalf("allowed pool = %+v, want upstream proxy binding", pool)
	}
	if got := cfg.UpstreamProxies[0].Auth.UsernameTemplate; got != "{{.Username}}" {
		t.Fatalf("username template = %q, want default", got)
	}
}

func TestLoadEgressUpstreamProxyNoneAuth(t *testing.T) {
	cfg := validEgressUpstreamProxyConfig()
	cfg.UpstreamProxies[0].Auth = EgressUpstreamProxyAuthConfig{Type: upstreamProxyAuthNone}

	_, err := loadEgressValue(t, cfg)
	if err != nil {
		t.Fatalf("LoadEgress() error = %v", err)
	}
}

func TestLoadEgressRejectsInvalidUpstreamProxyProfiles(t *testing.T) {
	t.Setenv("STRAW_TEST_PROXY_USERNAME", "account")
	t.Setenv("STRAW_TEST_PROXY_PASSWORD", "password")
	t.Setenv("STRAW_TEST_EMPTY_USERNAME", "")
	t.Setenv("STRAW_TEST_MISSING_USERNAME", "restored")
	t.Setenv("STRAW_TEST_MISSING_PASSWORD", "restored")
	err := os.Unsetenv("STRAW_TEST_MISSING_USERNAME")
	if err != nil {
		t.Fatal(err)
	}

	err = os.Unsetenv("STRAW_TEST_MISSING_PASSWORD")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*EgressConfig){
		"duplicate profile": func(c *EgressConfig) {
			c.UpstreamProxies = append(c.UpstreamProxies, c.UpstreamProxies[0])
		},
		"empty profile id": func(c *EgressConfig) { c.UpstreamProxies[0].ID = "" },
		"invalid profile id": func(c *EgressConfig) {
			c.UpstreamProxies[0].ID = "bad profile"
		},
		"oversized profile id": func(c *EgressConfig) {
			c.UpstreamProxies[0].ID = strings.Repeat("x", MaxUpstreamProxyIDBytes+1)
		},
		"unknown profile": func(c *EgressConfig) {
			c.Capabilities.AllowedPools[0].UpstreamProxyID = "unknown"
		},
		"unused profile": func(c *EgressConfig) {
			c.Capabilities.AllowedPools[0].UpstreamProxyID = ""
		},
		"duplicate pool": func(c *EgressConfig) {
			c.Capabilities.AllowedPools = append(c.Capabilities.AllowedPools, c.Capabilities.AllowedPools[0])
		},
		"unsupported endpoint scheme": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "socks5://proxy.example:1080"
		},
		"endpoint without port": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://proxy.example"
		},
		"endpoint without hostname": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://:8080"
		},
		"endpoint with zero port": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://proxy.example:0"
		},
		"endpoint with userinfo": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://user:secret@proxy.example:8080"
		},
		"endpoint with path": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://proxy.example:8080/connect"
		},
		"endpoint with query": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://proxy.example:8080?x=1"
		},
		"endpoint with fragment": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://proxy.example:8080#fragment"
		},
		"oversized endpoint": func(c *EgressConfig) {
			c.UpstreamProxies[0].Endpoint = "http://" + strings.Repeat("x", MaxUpstreamProxyEndpointBytes) + ":80"
		},
		"unsupported auth": func(c *EgressConfig) { c.UpstreamProxies[0].Auth.Type = "bearer" },
		"none with credentials": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.Type = upstreamProxyAuthNone
		},
		"missing username env name": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.UsernameEnv = ""
		},
		"invalid username env name": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.UsernameEnv = "9INVALID"
		},
		"unset username env": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.UsernameEnv = "STRAW_TEST_MISSING_USERNAME"
		},
		"empty username env": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.UsernameEnv = "STRAW_TEST_EMPTY_USERNAME"
		},
		"invalid password env name": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.PasswordEnv = "BAD-NAME"
		},
		"unset password env": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.PasswordEnv = "STRAW_TEST_MISSING_PASSWORD"
		},
		"invalid username template": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.UsernameTemplate = "{{"
		},
		"unsupported template function": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.UsernameTemplate = "{{printf \"%s\" .Username}}"
		},
		"oversized username template": func(c *EgressConfig) {
			c.UpstreamProxies[0].Auth.UsernameTemplate = strings.Repeat("x", MaxUpstreamProxyUsernameTemplateBytes+1)
		},
		"invalid default country": func(c *EgressConfig) {
			c.UpstreamProxies[0].Defaults.Country = "au"
		},
		"default region whitespace": func(c *EgressConfig) {
			c.UpstreamProxies[0].Defaults.Region = " nsw"
		},
		"default region control": func(c *EgressConfig) {
			c.UpstreamProxies[0].Defaults.Region = "nsw\n"
		},
		"oversized default ip type": func(c *EgressConfig) {
			c.UpstreamProxies[0].Defaults.IPType = strings.Repeat("x", MaxUpstreamProxyRegionBytes+1)
		},
		"country capability contradiction": func(c *EgressConfig) {
			c.UpstreamProxies[0].Defaults.Country = "NZ"
		},
		"region capability contradiction": func(c *EgressConfig) {
			c.UpstreamProxies[0].Defaults.Region = "vic"
		},
		"ip type capability contradiction": func(c *EgressConfig) {
			c.UpstreamProxies[0].Defaults.IPType = "datacenter"
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validEgressUpstreamProxyConfig()
			mutate(&cfg)

			_, err := loadEgressValue(t, cfg)
			if err == nil {
				t.Fatal("LoadEgress() unexpectedly succeeded")
			}
		})
	}
}

func TestLoadEgressUpstreamProxyStrictJSON(t *testing.T) {
	raw := `{"config_version":"v1","egress":{"capabilities":{"allowed_pools":[{"pool_id":"proxy","upstream_proxy_id":"profile"}]},"upstream_proxies":[{"id":"profile","endpoint":"http://proxy.example:8080","auth":{"type":"none","token_env":"TOKEN"}}]}}`
	_, err := LoadEgress(writeConfig(t, raw))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadEgress() error = %v, want unknown field", err)
	}
}

func TestLoadEgressRejectsInvalidUTF8(t *testing.T) {
	raw := []byte(`{"config_version":"v1","egress":{"worker_id":"`)
	raw = append(raw, 0xff)
	raw = append(raw, []byte(`"}}`)...)

	_, err := LoadEgress(writeConfig(t, string(raw)))
	if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("LoadEgress() error = %v, want invalid UTF-8", err)
	}
}

func TestEgressUpstreamProxyDefaultsRejectInvalidUTF8(t *testing.T) {
	cfg := validEgressUpstreamProxyConfig()
	cfg.UpstreamProxies[0].Auth = EgressUpstreamProxyAuthConfig{Type: upstreamProxyAuthNone}
	cfg.UpstreamProxies[0].Defaults.Region = string([]byte{0xff})
	cfg.applyDefaults()

	err := cfg.validate()
	if !errors.Is(err, errInvalidUpstreamProxy) {
		t.Fatalf("validate() error = %v, want invalid upstream proxy", err)
	}
}

func validEgressUpstreamProxyConfig() EgressConfig {
	return EgressConfig{
		Capabilities: EgressCapabilities{
			AllowedPools: []EgressPoolRef{{PoolID: "proxy-pool", UpstreamProxyID: upstreamProxyTestProfileID}},
			Countries:    []string{"AU"},
			Regions:      []string{"nsw"},
			IPTypes:      []string{"residential"},
		},
		UpstreamProxies: []EgressUpstreamProxyConfig{{
			ID:       upstreamProxyTestProfileID,
			Endpoint: "https://proxy.example:8443/",
			Auth: EgressUpstreamProxyAuthConfig{
				Type:        upstreamProxyAuthBasic,
				UsernameEnv: "STRAW_TEST_PROXY_USERNAME",
				PasswordEnv: "STRAW_TEST_PROXY_PASSWORD",
			},
			Defaults: EgressUpstreamProxyDefaults{Country: "AU", Region: "nsw", IPType: "residential"},
		}},
	}
}

func loadEgressValue(t *testing.T, cfg EgressConfig) (EgressConfig, error) {
	t.Helper()

	raw, err := json.Marshal(File{ConfigVersion: Version, Egress: &cfg})
	if err != nil {
		t.Fatal(err)
	}

	return LoadEgress(writeConfig(t, string(raw)))
}
