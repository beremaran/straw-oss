package egress

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/bogdanfinn/tls-client/profiles"
)

func TestProfileRegistryExecutableFingerprintIsExact(t *testing.T) {
	t.Parallel()

	if len(executableFingerprintProfiles) != 1 {
		t.Fatalf("executable fingerprint profiles = %d, want exactly chrome_120", len(executableFingerprintProfiles))
	}

	got, ok := executableFingerprintProfiles[chrome120FingerprintProfile]
	if !ok {
		t.Fatal("executable fingerprint profiles missing chrome_120")
	}
	assertClientProfileEqual(t, got, profiles.Chrome_120)

	names := make([]string, 0, len(executableFingerprintProfiles))
	for name := range executableFingerprintProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	if advertised := SupportedFingerprintProfiles(); !slices.Equal(advertised, names) {
		t.Fatalf("advertised fingerprint profiles = %v, executable registry = %v", advertised, names)
	}

	for _, name := range []string{
		"baseline",
		"default",
		"firefox_120",
		"firefox_121",
		"firefox_123",
		"safari_17",
		"safari_ios_17_0",
	} {
		if _, exists := executableFingerprintProfiles[name]; exists {
			t.Errorf("unplanned profile %q is executable", name)
		}
	}
}

func TestChrome120GoldenFixtureIsImmutable(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/chrome_120_v1_15_1.json")
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

func assertClientProfileEqual(t *testing.T, got, want profiles.ClientProfile) {
	t.Helper()

	gotSpec, err := got.GetClientHelloSpec()
	if err != nil {
		t.Fatalf("get registry ClientHello spec: %v", err)
	}
	wantSpec, err := want.GetClientHelloSpec()
	if err != nil {
		t.Fatalf("get profiles.Chrome_120 ClientHello spec: %v", err)
	}
	gotHelloID := got.GetClientHelloId()
	wantHelloID := want.GetClientHelloId()

	gotContract := []any{
		got.GetClientHelloStr(),
		gotHelloID.Client,
		gotHelloID.RandomExtensionOrder,
		gotHelloID.Version,
		gotHelloID.Seed,
		gotSpec,
		got.GetSettings(),
		got.GetSettingsOrder(),
		got.GetConnectionFlow(),
		got.GetPseudoHeaderOrder(),
		got.GetHeaderPriority(),
		got.GetPriorities(),
		got.GetStreamID(),
		got.GetAllowHTTP(),
		got.GetHttp3Settings(),
		got.GetHttp3SettingsOrder(),
		got.GetHttp3PriorityParam(),
		got.GetHttp3PseudoHeaderOrder(),
		got.GetHttp3SendGreaseFrames(),
	}
	wantContract := []any{
		want.GetClientHelloStr(),
		wantHelloID.Client,
		wantHelloID.RandomExtensionOrder,
		wantHelloID.Version,
		wantHelloID.Seed,
		wantSpec,
		want.GetSettings(),
		want.GetSettingsOrder(),
		want.GetConnectionFlow(),
		want.GetPseudoHeaderOrder(),
		want.GetHeaderPriority(),
		want.GetPriorities(),
		want.GetStreamID(),
		want.GetAllowHTTP(),
		want.GetHttp3Settings(),
		want.GetHttp3SettingsOrder(),
		want.GetHttp3PriorityParam(),
		want.GetHttp3PseudoHeaderOrder(),
		want.GetHttp3SendGreaseFrames(),
	}
	if !reflect.DeepEqual(gotContract, wantContract) {
		t.Fatal("chrome_120 registry entry does not equal profiles.Chrome_120")
	}
}

const chrome120GoldenContract = `{
  "profile": "chrome_120",
  "contract_revision": "chrome_120_v1_15_1",
  "tls_client_version": "v1.15.1",
  "preset": "profiles.Chrome_120",
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
