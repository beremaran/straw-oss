package config

import (
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/beremaran/straw-oss/internal/proxytemplate"
)

// Canonical upstream-proxy field bounds. These constants intentionally live
// in config as well as the protocol bindings so neither package depends on the
// other's runtime internals.
const (
	MaxUpstreamProxyIDBytes                 = 128
	MaxUpstreamProxyRegionBytes             = 128
	MaxUpstreamProxyEndpointBytes           = 1024
	MaxUpstreamProxyUsernameTemplateBytes   = 4096
	MaxEnvVarNameBytes                      = 128
	MaxUpstreamProxyRenderedCredentialBytes = 4096
	MaxProxyAuthorizationBytes              = 16 * 1024
	upstreamProxyAuthNone                   = "none"
	upstreamProxyAuthBasic                  = "basic"
)

func (e EgressConfig) validateUpstreamProxies() (map[string]EgressUpstreamProxyConfig, error) {
	profiles := make(map[string]EgressUpstreamProxyConfig, len(e.UpstreamProxies))
	for _, profile := range e.UpstreamProxies {
		if !validSnapshotID(profile.ID) {
			return nil, fmt.Errorf("%w: profile id is invalid", errInvalidUpstreamProxy)
		}

		if _, duplicate := profiles[profile.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate profile %q", errInvalidUpstreamProxy, profile.ID)
		}

		err := validateUpstreamProxyEndpoint(profile)
		if err != nil {
			return nil, err
		}

		err = validateUpstreamProxyAuth(profile)
		if err != nil {
			return nil, err
		}

		err = validateUpstreamProxyDefaults(profile, e.Capabilities)
		if err != nil {
			return nil, err
		}

		profiles[profile.ID] = profile
	}

	return profiles, nil
}

func validateUpstreamProxyEndpoint(profile EgressUpstreamProxyConfig) error {
	if profile.Endpoint == "" || len(profile.Endpoint) > MaxUpstreamProxyEndpointBytes {
		return fmt.Errorf("%w: profile %q endpoint is invalid", errInvalidUpstreamProxy, profile.ID)
	}

	endpoint, err := url.Parse(profile.Endpoint)
	if err != nil || !validUpstreamProxyEndpointAddress(endpoint) {
		return fmt.Errorf("%w: profile %q endpoint must use http or https with an explicit hostname and port", errInvalidUpstreamProxy, profile.ID)
	}

	if !validUpstreamProxyEndpointPort(endpoint.Port()) {
		return fmt.Errorf("%w: profile %q endpoint port is invalid", errInvalidUpstreamProxy, profile.ID)
	}

	if hasUnsupportedUpstreamProxyURLComponents(profile.Endpoint, endpoint) {
		return fmt.Errorf("%w: profile %q endpoint contains unsupported URL components", errInvalidUpstreamProxy, profile.ID)
	}

	return nil
}

func validUpstreamProxyEndpointAddress(endpoint *url.URL) bool {
	return (endpoint.Scheme == "http" || endpoint.Scheme == "https") && endpoint.Hostname() != "" && endpoint.Port() != ""
}

func validUpstreamProxyEndpointPort(value string) bool {
	port, err := strconv.ParseUint(value, 10, 16)

	return err == nil && port != 0
}

func hasUnsupportedUpstreamProxyURLComponents(raw string, endpoint *url.URL) bool {
	return endpoint.User != nil || endpoint.Path != "" && endpoint.Path != "/" || endpoint.RawPath != "" ||
		endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || strings.Contains(raw, "#")
}

func validateUpstreamProxyAuth(profile EgressUpstreamProxyConfig) error {
	auth := profile.Auth
	switch auth.Type {
	case upstreamProxyAuthNone:
		if auth.UsernameEnv != "" || auth.PasswordEnv != "" || auth.UsernameTemplate != "" {
			return fmt.Errorf("%w: profile %q auth type none cannot configure credentials", errInvalidUpstreamProxy, profile.ID)
		}

		return nil
	case upstreamProxyAuthBasic:
		return validateBasicUpstreamProxyAuth(profile)
	default:
		return fmt.Errorf("%w: profile %q auth type must be none or basic", errInvalidUpstreamProxy, profile.ID)
	}
}

func validateBasicUpstreamProxyAuth(profile EgressUpstreamProxyConfig) error {
	auth := profile.Auth
	if !validEnvName(auth.UsernameEnv) {
		return fmt.Errorf("%w: profile %q username_env %q is invalid", errInvalidUpstreamProxy, profile.ID, auth.UsernameEnv)
	}

	username, exists := os.LookupEnv(auth.UsernameEnv)
	if !exists || username == "" {
		return fmt.Errorf("%w: profile %q username environment variable %q is unset or empty", errInvalidUpstreamProxy, profile.ID, auth.UsernameEnv)
	}

	if auth.PasswordEnv != "" {
		if !validEnvName(auth.PasswordEnv) {
			return fmt.Errorf("%w: profile %q password_env %q is invalid", errInvalidUpstreamProxy, profile.ID, auth.PasswordEnv)
		}

		if _, exists := os.LookupEnv(auth.PasswordEnv); !exists {
			return fmt.Errorf("%w: profile %q password environment variable %q is unset", errInvalidUpstreamProxy, profile.ID, auth.PasswordEnv)
		}
	}

	if auth.UsernameTemplate == "" || len(auth.UsernameTemplate) > MaxUpstreamProxyUsernameTemplateBytes {
		return fmt.Errorf("%w: profile %q username_template is invalid", errInvalidUpstreamProxy, profile.ID)
	}

	_, err := proxytemplate.Parse(profile.ID, auth.UsernameTemplate)
	if err != nil {
		return fmt.Errorf("%w: profile %q username_template is invalid: %w", errInvalidUpstreamProxy, profile.ID, err)
	}

	return nil
}

func validateUpstreamProxyDefaults(profile EgressUpstreamProxyConfig, capabilities EgressCapabilities) error {
	defaults := profile.Defaults
	if defaults.Country != "" && (len(defaults.Country) != 2 || !asciiUpper(defaults.Country)) {
		return fmt.Errorf("%w: profile %q default country is invalid", errInvalidUpstreamProxy, profile.ID)
	}

	if !validOptionalUpstreamProxyText(defaults.Region) || !validOptionalUpstreamProxyText(defaults.IPType) {
		return fmt.Errorf("%w: profile %q default region or ip_type is invalid", errInvalidUpstreamProxy, profile.ID)
	}

	for _, constraint := range []struct {
		name         string
		value        string
		capabilities []string
	}{
		{name: "country", value: defaults.Country, capabilities: capabilities.Countries},
		{name: "region", value: defaults.Region, capabilities: capabilities.Regions},
		{name: "ip_type", value: defaults.IPType, capabilities: capabilities.IPTypes},
	} {
		if constraint.value != "" && len(constraint.capabilities) != 0 && !slices.Contains(constraint.capabilities, constraint.value) {
			return fmt.Errorf("%w: profile %q default %s contradicts worker capabilities", errInvalidUpstreamProxy, profile.ID, constraint.name)
		}
	}

	return nil
}

func validEnvName(name string) bool {
	if name == "" || len(name) > MaxEnvVarNameBytes || !asciiLetter(name[0]) && name[0] != '_' {
		return false
	}

	for _, c := range []byte(name[1:]) {
		if !asciiLetter(c) && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}

	return true
}

func validOptionalUpstreamProxyText(value string) bool {
	if value == "" {
		return true
	}

	if len(value) > MaxUpstreamProxyRegionBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}

	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}

	return true
}

func asciiUpper(value string) bool {
	for _, c := range []byte(value) {
		if c < 'A' || c > 'Z' {
			return false
		}
	}

	return true
}

func asciiLetter(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}
