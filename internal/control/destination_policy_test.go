package control

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/beremaran/straw-oss/internal/config"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	testInjectionHeaderName        = "X-Foo"
	testDisabledFingerprintProfile = "disabled_profile"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}

	return u
}

func fingerprintSnapshot() []config.FingerprintProfile {
	return []config.FingerprintProfile{
		{Name: defaultFingerprintProfileName, ScopeType: fingerprintProfileScopeGlobal},
		{Name: fingerprintProfileChrome120, ScopeType: fingerprintProfileScopeGlobal, Enabled: true, SupportedByWorker: true},
		{Name: testDisabledFingerprintProfile, ScopeType: fingerprintProfileScopeGlobal, Enabled: false, SupportedByWorker: true},
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestResolveDestinationPolicy_AllowsOrdinaryPublicHost(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{ConfigVersion: 3, FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "https://example.com/path"),
		MaxInjectedHeaderBytes: 1024,
	}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if result.Policy.ResolutionMode != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL {
		t.Fatalf("resolution mode = %v, want DIRECT_LOCAL", result.Policy.ResolutionMode)
	}

	if result.FingerprintProfile != baselineFingerprintProfileName {
		t.Fatalf("fingerprint = %q, want baseline", result.FingerprintProfile)
	}

	if result.Policy.PolicyVersion != "3" {
		t.Fatalf("policy version = %q, want 3", result.Policy.PolicyVersion)
	}
}

func TestResolveDestinationPolicy_DefaultAliasUsesBaselineWithDisabledDurableRow(t *testing.T) {
	t.Parallel()

	result, verr := ResolveDestinationPolicy(DestinationPolicyRequest{
		Snapshot:                    config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:                   mustURL(t, "https://example.com/path"),
		RequestedFingerprintProfile: defaultFingerprintProfileName,
		MaxInjectedHeaderBytes:      1024,
	})
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}
	if result.FingerprintProfile != baselineFingerprintProfileName {
		t.Fatalf("fingerprint = %q, want baseline", result.FingerprintProfile)
	}
}

func TestResolveDestinationPolicy_HostDenyNormalization(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeHost, Action: denyRuleActionDeny, Enabled: true, NormalizedHost: "blocked.example.com"},
		},
	}

	// The stored rule is already normalized (lowercase, no trailing dot); the
	// incoming request host must be normalized the same way before matching.
	req := DestinationPolicyRequest{
		Snapshot:               snap,
		TargetURL:              mustURL(t, "https://Blocked.Example.Com./path"),
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied", verr)
	}
}

func TestResolveDestinationPolicy_IDNAHostDenyNormalization(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeHost, Action: denyRuleActionDeny, Enabled: true, NormalizedHost: "xn--pple-43d.com"},
		},
	}

	req := DestinationPolicyRequest{
		Snapshot:               snap,
		TargetURL:              mustURL(t, "https://аpple.com/path"),
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied", verr)
	}
}

func TestValidateRequestRejectsInvalidIDNAHost(t *testing.T) {
	_, err := ValidateRequest([]byte(`{"method":"GET","url":"https://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.com/","timeout_ms":5000}`), 1<<20, 5000)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != errorCodeInvalidRequest {
		t.Fatalf("ValidateRequest() error = %#v, want invalid_request", err)
	}
}

func TestResolveDestinationPolicy_HostAllowOverride(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeHost, Action: denyRuleActionDeny, Enabled: true, NormalizedHost: "shared.example.com"},
			{RuleType: denyRuleTypeHost, Action: denyRuleActionAllowOverride, Enabled: true, NormalizedHost: "shared.example.com"},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://shared.example.com/"), MaxInjectedHeaderBytes: 1024}

	_, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}
}

func TestResolveDestinationPolicy_MetadataIPDefaultDenied(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "http://169.254.169.254/latest/meta-data"),
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied", verr)
	}
}

func TestResolveDestinationPolicy_PrivateRangeDefaultDenied(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "http://10.0.0.5/"),
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied", verr)
	}
}

func TestResolveDestinationPolicy_CIDRAllowOverridesPrivateDefault(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeCIDR, Action: denyRuleActionAllowOverride, Enabled: true, NormalizedCIDR: "10.0.0.0/8"},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "http://10.0.0.5/"), MaxInjectedHeaderBytes: 1024}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if len(result.Policy.AllowedCidrs) != 1 || result.Policy.AllowedCidrs[0] != "10.0.0.0/8" {
		t.Fatalf("allowed cidrs = %v", result.Policy.AllowedCidrs)
	}
}

func TestResolveDestinationPolicy_IPv4MappedIPv6Denied(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "http://[::ffff:169.254.169.254]/"),
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied", verr)
	}
}

func TestResolveDestinationPolicy_CNameRuleCompiledNotEvaluated(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeCNAMESuffix, Action: denyRuleActionDeny, Enabled: true, NormalizedName: testCNAMESuffix},
		},
	}

	// Control cannot resolve DNS/CNAME chains, so a request whose *host* is
	// not itself denied must pass Control-side evaluation; the cname rule is
	// only compiled into the bundle for Egress to enforce post-resolution.
	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://public.example.com/"), MaxInjectedHeaderBytes: 1024}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if len(result.Policy.DeniedCnameSuffixes) != 1 || result.Policy.DeniedCnameSuffixes[0] != testCNAMESuffix {
		t.Fatalf("denied cname suffixes = %v", result.Policy.DeniedCnameSuffixes)
	}
}

func TestResolveDestinationPolicy_HostSuffixDeniesSubdomain(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeHostSuffix, Action: denyRuleActionDeny, Enabled: true, NormalizedHost: "blocked.example.net"},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://api.blocked.example.net/"), MaxInjectedHeaderBytes: 1024}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied for a subdomain of a denied host_suffix", verr)
	}
}

func TestResolveDestinationPolicy_HostSuffixAllowOverrideCancelsBundleEntry(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeHostSuffix, Action: denyRuleActionDeny, Enabled: true, NormalizedHost: "shared.example.net"},
			{RuleType: denyRuleTypeHostSuffix, Action: denyRuleActionAllowOverride, Enabled: true, NormalizedHost: "shared.example.net"},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://api.shared.example.net/"), MaxInjectedHeaderBytes: 1024}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if len(result.Policy.DeniedHostSuffixes) != 0 {
		t.Fatalf("denied host suffixes = %v, want none: allow_override must cancel the overridden entry so Egress doesn't re-deny the host Control just approved", result.Policy.DeniedHostSuffixes)
	}
}

func TestResolveDestinationPolicy_CnameSuffixAllowOverrideCancelsBundleEntry(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeCNAMESuffix, Action: denyRuleActionDeny, Enabled: true, NormalizedName: testCNAMESuffix},
			{RuleType: denyRuleTypeCNAMESuffix, Action: denyRuleActionAllowOverride, Enabled: true, NormalizedName: testCNAMESuffix},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://public.example.com/"), MaxInjectedHeaderBytes: 1024}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if len(result.Policy.DeniedCnameSuffixes) != 0 {
		t.Fatalf("denied cname suffixes = %v, want none after allow_override cancellation", result.Policy.DeniedCnameSuffixes)
	}
}

func TestResolveDestinationPolicy_MetadataIPTypeCompilesToDeniedCidrs(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypeMetadataIP, Action: denyRuleActionDeny, Enabled: true, NormalizedCIDR: "169.254.169.254/32"},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "http://169.254.169.254/"), MaxInjectedHeaderBytes: 1024}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied", verr)
	}
}

func TestResolveDestinationPolicy_PrivateRangeAllowOverrideCompilesToAllowedCidrs(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		DenyRules: []config.DenyRule{
			{RuleType: denyRuleTypePrivateRange, Action: denyRuleActionAllowOverride, Enabled: true, NormalizedCIDR: testPrivateRange},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "http://172.16.5.5/"), MaxInjectedHeaderBytes: 1024}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if len(result.Policy.AllowedCidrs) != 1 || result.Policy.AllowedCidrs[0] != testPrivateRange {
		t.Fatalf("allowed cidrs = %v, want [%s]", result.Policy.AllowedCidrs, testPrivateRange)
	}
}

func TestResolveDestinationPolicy_FingerprintMismatch(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:                    config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:                   mustURL(t, "https://example.com/"),
		RequestedFingerprintProfile: "safari_99",
		MaxInjectedHeaderBytes:      1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeUnsupportedFingerprint {
		t.Fatalf("verr = %+v, want unsupported_fingerprint", verr)
	}
}

func TestFingerprintProfileDestinationPolicyAcceptsEnabledNamedProfile(t *testing.T) {
	result, verr := ResolveDestinationPolicy(DestinationPolicyRequest{
		Snapshot:                    config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:                   mustURL(t, "https://example.com/"),
		RequestedFingerprintProfile: fingerprintProfileChrome120,
		MaxInjectedHeaderBytes:      1024,
	})
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}
	if result.FingerprintProfile != fingerprintProfileChrome120 {
		t.Fatalf("fingerprint = %q, want chrome_120", result.FingerprintProfile)
	}
}

func TestResolveDestinationPolicy_FingerprintDisabledProfileRejected(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:                    config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:                   mustURL(t, "https://example.com/"),
		RequestedFingerprintProfile: testDisabledFingerprintProfile,
		MaxInjectedHeaderBytes:      1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeUnsupportedFingerprint {
		t.Fatalf("verr = %+v, want unsupported_fingerprint", verr)
	}
}

func TestResolveDestinationPolicyEnabledNamedProfileDoesNotDependOnWorkerAvailability(t *testing.T) {
	t.Parallel()

	req := DestinationPolicyRequest{
		Snapshot: config.Snapshot{FingerprintProfiles: []config.FingerprintProfile{
			{Name: defaultFingerprintProfileName, ScopeType: fingerprintProfileScopeGlobal, Enabled: true, SupportedByWorker: true},
			{Name: fingerprintProfileChrome120, ScopeType: fingerprintProfileScopeGlobal, Enabled: true, SupportedByWorker: false},
		}},
		TargetURL:                   mustURL(t, "https://example.com/path"),
		RequestedFingerprintProfile: fingerprintProfileChrome120,
		MaxInjectedHeaderBytes:      1024,
	}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("ResolveDestinationPolicy() error = %+v, want enabled named profile to survive until ordinary routing/capability filtering", verr)
	}
	if result.FingerprintProfile != fingerprintProfileChrome120 {
		t.Fatalf("fingerprint = %q, want %q", result.FingerprintProfile, fingerprintProfileChrome120)
	}
}

func TestResolveDestinationPolicy_InjectionOrderedAndSafe(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		InjectionPolicies: []config.InjectionPolicy{
			{ID: "b_second", Enabled: true, Operations: []config.InjectionOperation{
				{Op: injectionOpAppend, HeaderName: "X-Trace", ValueBase64: b64("v2")},
			}},
			{ID: "a_first", Enabled: true, Operations: []config.InjectionOperation{
				{Op: injectionOpSet, HeaderName: "User-Agent", ValueBase64: b64("straw/1.0")},
			}},
			{ID: "c_disabled", Enabled: false, Operations: []config.InjectionOperation{
				{Op: injectionOpSet, HeaderName: "X-Ignored", ValueBase64: b64("nope")},
			}},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://example.com/"), MaxInjectedHeaderBytes: 1024}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if len(result.InjectionOperations) != 2 {
		t.Fatalf("ops = %+v, want 2 (policy a_first before b_second, c_disabled excluded)", result.InjectionOperations)
	}

	if result.InjectionOperations[0].HeaderName != "User-Agent" || result.InjectionOperations[1].HeaderName != "X-Trace" {
		t.Fatalf("ops out of order: %+v", result.InjectionOperations)
	}
}

func TestResolveDestinationPolicy_InjectionDeniedHeaderRejected(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		InjectionPolicies: []config.InjectionPolicy{
			{ID: "p1", Enabled: true, Operations: []config.InjectionOperation{
				{Op: injectionOpSet, HeaderName: headerCanonicalContentLength, ValueBase64: b64("0")},
			}},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://example.com/"), MaxInjectedHeaderBytes: 1024}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeHeaderInjectionFailed {
		t.Fatalf("verr = %+v, want header_injection_failed", verr)
	}
}

func TestResolveDestinationPolicy_InjectionDuplicateSetRejected(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		InjectionPolicies: []config.InjectionPolicy{
			{ID: "p1", Enabled: true, Operations: []config.InjectionOperation{
				{Op: injectionOpSet, HeaderName: testInjectionHeaderName, ValueBase64: b64("a")},
			}},
			{ID: "p2", Enabled: true, Operations: []config.InjectionOperation{
				{Op: injectionOpSet, HeaderName: "x-foo", ValueBase64: b64("b")},
			}},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://example.com/"), MaxInjectedHeaderBytes: 1024}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeHeaderInjectionFailed {
		t.Fatalf("verr = %+v, want header_injection_failed", verr)
	}
}

func TestResolveDestinationPolicy_InjectionCRLFRejected(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		InjectionPolicies: []config.InjectionPolicy{
			{ID: "p1", Enabled: true, Operations: []config.InjectionOperation{
				{Op: injectionOpSet, HeaderName: testInjectionHeaderName, ValueBase64: b64("bad\r\nvalue")},
			}},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://example.com/"), MaxInjectedHeaderBytes: 1024}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeHeaderInjectionFailed {
		t.Fatalf("verr = %+v, want header_injection_failed", verr)
	}
}

func TestResolveDestinationPolicy_InjectionSizeBound(t *testing.T) {
	snap := config.Snapshot{
		FingerprintProfiles: fingerprintSnapshot(),
		InjectionPolicies: []config.InjectionPolicy{
			{ID: "p1", Enabled: true, Operations: []config.InjectionOperation{
				{Op: injectionOpSet, HeaderName: testInjectionHeaderName, ValueBase64: b64("this value is much too long for the tiny bound")},
			}},
		},
	}

	req := DestinationPolicyRequest{Snapshot: snap, TargetURL: mustURL(t, "https://example.com/"), MaxInjectedHeaderBytes: 8}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeHeaderInjectionFailed {
		t.Fatalf("verr = %+v, want header_injection_failed", verr)
	}
}

func TestResolveDestinationPolicy_UpstreamProxyUntrustedRejected(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "https://example.com/"),
		UpstreamProxyEnabled:   true,
		UpstreamProxyTrusted:   false,
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("verr = %+v, want destination_denied", verr)
	}
}

func TestResolveDestinationPolicy_UpstreamProxyTrustedResolvesRemoteMode(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "https://example.com/"),
		UpstreamProxyEnabled:   true,
		UpstreamProxyTrusted:   true,
		MaxInjectedHeaderBytes: 1024,
	}

	result, verr := ResolveDestinationPolicy(req)
	if verr != nil {
		t.Fatalf("unexpected error: %+v", verr)
	}

	if result.Policy.ResolutionMode != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE {
		t.Fatalf("resolution mode = %v, want UPSTREAM_PROXY_REMOTE", result.Policy.ResolutionMode)
	}
}

func TestResolveDestinationPolicy_UserinfoRejected(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "https://user:pass@example.com/"),
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeInvalidRequest {
		t.Fatalf("verr = %+v, want invalid_request", verr)
	}
}

func TestResolveDestinationPolicy_NonASCIIHostRejected(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              &url.URL{Scheme: urlSchemeHTTPS, Host: "xn--\xc3\xa9.example.com"},
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil || verr.Code != errorCodeInvalidRequest {
		t.Fatalf("verr = %+v, want invalid_request", verr)
	}
}

// PublicSafeErrorDetails: errors returned by the resolver must never embed
// the raw target URL/host in their message or details.
func TestResolveDestinationPolicy_PublicSafeErrorDetails(t *testing.T) {
	req := DestinationPolicyRequest{
		Snapshot:               config.Snapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              mustURL(t, "http://169.254.169.254/secret-path?token=abc"),
		MaxInjectedHeaderBytes: 1024,
	}

	_, verr := ResolveDestinationPolicy(req)
	if verr == nil {
		t.Fatalf("expected denial")
	}

	if strings.Contains(verr.Message, "169.254.169.254") || strings.Contains(verr.Message, "secret-path") {
		t.Fatalf("message leaks raw target: %q", verr.Message)
	}

	for k, v := range verr.Details {
		if strings.Contains(v, "169.254.169.254") || strings.Contains(v, "secret-path") {
			t.Fatalf("details[%q] leaks raw target: %q", k, v)
		}
	}
}
