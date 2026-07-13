package egress

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"testing"

	utls "github.com/bogdanfinn/utls"

	"github.com/beremaran/straw-oss/internal/egress/profilecatalog"
	"github.com/beremaran/straw-oss/internal/fingerprint"
)

func TestProfileRegistryExecutableFingerprintIsExact(t *testing.T) {
	t.Parallel()

	if len(executableFingerprintProfiles) != 79 {
		t.Fatalf("executable fingerprint profiles = %d, want 79", len(executableFingerprintProfiles))
	}

	got, ok := executableFingerprintProfiles[chrome120FingerprintProfile]
	if !ok {
		t.Fatal("executable fingerprint profiles missing chrome_120")
	}
	assertClientProfileEqual(t, got, profilecatalog.Chrome_120)
	assertClientHelloEqual(t, got.GetClientHelloId(), utls.HelloChrome_120)

	names := make([]string, 0, len(executableFingerprintProfiles))
	for name := range executableFingerprintProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	if advertised := SupportedFingerprintProfiles(); !slices.Equal(advertised, names) {
		t.Fatalf("advertised fingerprint profiles = %v, executable registry = %v", advertised, names)
	}

	if !slices.Equal(names, fingerprint.Names()) {
		t.Fatalf("executable names = %v, contract names = %v", names, fingerprint.Names())
	}

	for _, name := range []string{"baseline", "default", "firefox_121", "safari_17"} {
		if _, exists := executableFingerprintProfiles[name]; exists {
			t.Errorf("non-catalogue profile %q is executable", name)
		}
	}

	for name, profile := range executableFingerprintProfiles {
		id := profile.GetClientHelloId()
		_, err := id.ToSpec()
		if err != nil {
			_, fallbackErr := utls.UTLSIdToSpec(id)
			if fallbackErr != nil {
				t.Errorf("profile %q ClientHello spec: %v (fallback: %v)", name, err, fallbackErr)
			}
		}
	}
}

func TestProfileSessionCachesMatchPSKProfilesAndAreIsolated(t *testing.T) {
	t.Parallel()

	caches := newProfileSessionCaches()
	for name, profile := range executableFingerprintProfiles {
		_, cached := caches[name]
		if cached != supportsProfileSessionResumption(profile) {
			t.Errorf("profile %q cache present = %t, PSK capable = %t", name, cached, supportsProfileSessionResumption(profile))
		}
	}

	first := caches["chrome_144_PSK"]
	second := caches["firefox_147_PSK"]
	if first == nil || second == nil {
		t.Fatal("representative PSK profiles are missing session caches")
	}
	if first == second {
		t.Fatal("PSK profiles unexpectedly share a session cache")
	}
}

func assertClientProfileEqual(t *testing.T, got, want profilecatalog.ClientProfile) {
	t.Helper()

	gotID := got.GetClientHelloId()
	wantID := want.GetClientHelloId()
	gotContract := []any{
		gotID.Str(), gotID.Client, gotID.Version, gotID.RandomExtensionOrder,
		got.GetSettings(), got.GetSettingsOrder(),
		got.GetConnectionFlow(), got.GetPseudoHeaderOrder(), got.GetHeaderPriority(),
		got.GetPriorities(), got.GetStreamID(), got.GetAllowHTTP(),
	}
	wantContract := []any{
		wantID.Str(), wantID.Client, wantID.Version, wantID.RandomExtensionOrder,
		want.GetSettings(), want.GetSettingsOrder(),
		want.GetConnectionFlow(), want.GetPseudoHeaderOrder(), want.GetHeaderPriority(),
		want.GetPriorities(), want.GetStreamID(), want.GetAllowHTTP(),
	}
	if !reflect.DeepEqual(gotContract, wantContract) {
		t.Fatal("registry entry does not preserve the pinned profile contract")
	}
}

func TestChrome120GoldenFixtureIsImmutable(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/chrome_120_profile_catalog_v1_15_1.json")
	if err != nil {
		t.Fatalf("read chrome_120 fixture: %v", err)
	}

	var got any
	err = json.Unmarshal(raw, &got)
	if err != nil {
		t.Fatalf("decode chrome_120 fixture: %v", err)
	}
	var want any
	err = json.Unmarshal([]byte(chrome120GoldenContract), &want)
	if err != nil {
		t.Fatalf("decode expected chrome_120 contract: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chrome_120 fixture drifted from its immutable v1.15.1 contract\ngot:  %s\nwant: %s", raw, chrome120GoldenContract)
	}
}

func assertClientHelloEqual(t *testing.T, got, want utls.ClientHelloID) {
	t.Helper()

	gotContract := []any{
		got.Str(),
		got.Client,
		got.RandomExtensionOrder,
		got.Version,
		got.Seed,
	}
	wantContract := []any{
		want.Str(),
		want.Client,
		want.RandomExtensionOrder,
		want.Version,
		want.Seed,
	}
	if !reflect.DeepEqual(gotContract, wantContract) {
		t.Fatal("chrome_120 registry entry does not equal utls.HelloChrome_120")
	}
}

const chrome120GoldenContract = `{
  "profile": "chrome_120",
  "contract_revision": "tls-client-v1.15.1-http1-http2",
  "catalogue_source": "github.com/bogdanfinn/tls-client@v1.15.1/profiles",
  "utls_version": "v1.7.7-barnius",
  "preset": "profilecatalog.Chrome_120",
  "transport": {
    "protocols": ["h2", "http/1.1"],
    "http3": "disabled",
    "redirects": "disabled",
    "connection_reuse": "request_scoped"
  },
  "tls": {
    "cipher_suites": [
      "GREASE",
      "TLS_AES_128_GCM_SHA256",
      "TLS_AES_256_GCM_SHA384",
      "TLS_CHACHA20_POLY1305_SHA256",
      "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
      "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
      "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
      "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
      "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
      "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
      "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
      "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
      "TLS_RSA_WITH_AES_128_GCM_SHA256",
      "TLS_RSA_WITH_AES_256_GCM_SHA384",
      "TLS_RSA_WITH_AES_128_CBC_SHA",
      "TLS_RSA_WITH_AES_256_CBC_SHA"
    ],
    "extensions": [
      "GREASE",
      "GREASE",
      "application_layer_protocol_negotiation",
      "application_settings",
      "compress_certificate",
      "ec_point_formats",
      "encrypted_client_hello",
      "extended_master_secret",
      "key_share",
      "psk_key_exchange_modes",
      "renegotiation_info",
      "server_name",
      "session_ticket",
      "signature_algorithms",
      "signed_certificate_timestamp",
      "status_request",
      "supported_groups",
      "supported_versions"
    ],
    "extension_order": "randomized_except_grease_and_padding",
    "grease": "normalized",
    "supported_versions": ["GREASE", "TLS1.3", "TLS1.2"],
    "supported_groups": ["GREASE", "X25519", "P-256", "P-384"],
    "signature_algorithms": [
      "ECDSA_SECP256R1_SHA256",
      "RSA_PSS_RSAE_SHA256",
      "RSA_PKCS1_SHA256",
      "ECDSA_SECP384R1_SHA384",
      "RSA_PSS_RSAE_SHA384",
      "RSA_PKCS1_SHA384",
      "RSA_PSS_RSAE_SHA512",
      "RSA_PKCS1_SHA512"
    ],
    "key_shares": [
      {"group": "GREASE", "key_exchange_length": 1},
      {"group": "X25519", "key_exchange_length": 32}
    ],
    "alpn": ["h2", "http/1.1"]
  },
  "http2": {
    "settings": [
      {"id": "HEADER_TABLE_SIZE", "value": 65536},
      {"id": "ENABLE_PUSH", "value": 0},
      {"id": "INITIAL_WINDOW_SIZE", "value": 6291456},
      {"id": "MAX_HEADER_LIST_SIZE", "value": 262144}
    ],
    "connection_window_update": 15663105,
    "pseudo_header_order": [":method", ":authority", ":scheme", ":path"],
    "priority_behavior": "none",
    "application_header_order": ["user-agent", "accept"],
    "application_header_order_rule": "preserve_request_order"
  },
  "incidental_fields": [
    "connection_ids",
    "tcp.source_port",
    "timestamps",
    "tls.ephemeral_key_bytes",
    "tls.grease_numeric_values",
    "tls.padding_length",
    "tls.record_boundaries"
  ]
}`
