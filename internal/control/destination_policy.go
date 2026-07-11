package control

import (
	"encoding/base64"
	"net/netip"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
)

const (
	denyRuleTypeHost              = "host"
	denyRuleTypeHostSuffix        = "host_suffix"
	denyRuleTypeCIDR              = "cidr"
	denyRuleTypeCNAMESuffix       = "cname_suffix"
	denyRuleTypeMetadataIP        = "metadata_ip"
	denyRuleTypePrivateRange      = "private_range"
	denyRuleActionDeny            = "deny"
	denyRuleActionAllowOverride   = "allow_override"
	injectionOpSet                = "set"
	injectionOpAppend             = "append"
	injectionOpRemove             = "remove"
	deniedInjectionHeaderPrefix   = "x-straw-"
	fingerprintProfileChrome120   = "chrome_120"
	fingerprintProfileScopeGlobal = "global"
)

var (
	denyRuleCIDRTypes            = map[string]bool{denyRuleTypeCIDR: true, denyRuleTypeMetadataIP: true, denyRuleTypePrivateRange: true}
	denyRuleHostTypes            = map[string]bool{denyRuleTypeHost: true, denyRuleTypeHostSuffix: true}
	alwaysDeniedInjectionHeaders = map[string]bool{
		denyRuleTypeHost:             true,
		headerNameContentLength:      true,
		headerNameTransferEncoding:   true,
		headerNameConnection:         true,
		headerNameProxyAuthorization: true,
	}
)

func denyRuleValue(rule config.DenyRule) string {
	switch {
	case denyRuleCIDRTypes[rule.RuleType]:
		return rule.NormalizedCIDR
	case rule.RuleType == denyRuleTypeCNAMESuffix:
		return rule.NormalizedName
	default:
		return rule.NormalizedHost
	}
}

// This file implements the P0 Control-side destination-policy resolver
// (docs/implementation-history.md#p0-22, docs/planning/27-security-controls.md,
// docs/planning/16-egress-execution.md). It evaluates the immutable tenant
// snapshot captured at request start and produces the strawpb.DestinationPolicy
// bundle, resolved header-injection operations, and resolved fingerprint
// profile name that task 24 attaches to RequestStart. It does not perform any
// DNS resolution or dial: Egress remains solely responsible for final
// resolved-IP validation immediately before connect (docs/planning/16 "Dial
// target invariant").
//
// internal/egress/executor.go (task 26) enforces DeniedHostSuffixes and
// DeniedCnameSuffixes downstream; this resolver only compiles them.

const (
	defaultFingerprintProfileName  = "default"
	baselineFingerprintProfileName = "baseline"
)

// defaultDeniedPrefixes and metadataIPs mirror
// internal/egress/executor.go's default-deny set exactly
// (docs/planning/27 "Default Denied CIDR Set") so Control's fail-fast
// pre-dispatch check agrees with what Egress will actually enforce.
// RFC1918/ULA private ranges, loopback, link-local, and multicast are
// covered directly via netip.Addr predicates (IsPrivate, IsLoopback,
// IsLinkLocalUnicast, IsMulticast) rather than a duplicated prefix list.
var defaultDeniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("::ffff:0:0/96"),
}

var metadataIPs = []netip.Addr{
	netip.MustParseAddr("169.254.169.254"),
	netip.MustParseAddr("169.254.169.253"),
	netip.MustParseAddr("169.254.170.2"),
	netip.MustParseAddr("100.100.100.200"),
	netip.MustParseAddr("100.100.100.201"),
}

// DestinationPolicyRequest is the input to ResolveDestinationPolicy. Snapshot
// must be the immutable tenant config snapshot captured at request start
// (docs/planning/10 "evaluated from an immutable snapshot captured at request
// start"). TargetURL must already be validated by ValidateRequest (request.go):
// scheme http/https, no fragment, no userinfo, non-empty host.
type DestinationPolicyRequest struct {
	Snapshot                    config.Snapshot
	TargetURL                   *url.URL
	RequestedFingerprintProfile string // empty means baseline transport

	// UpstreamProxyEnabled/UpstreamProxyTrusted describe deployment config;
	// P0 has no upstream-proxy config surface anywhere in this repo, so P0
	// callers always pass UpstreamProxyEnabled=false and always get
	// DESTINATION_RESOLUTION_DIRECT_LOCAL.
	UpstreamProxyEnabled bool
	UpstreamProxyTrusted bool

	// MaxInjectedHeaderBytes bounds the aggregate injected header name+value
	// bytes (docs/planning/27 "Maximum injected header bytes is bounded by
	// control.transport.max_frame_data_bytes"); callers pass
	// ControlConfig.Transport.MaxFrameDataBytes.
	MaxInjectedHeaderBytes uint64
}

// DestinationPolicyResult is the resolved bundle RequestStart carries to
// Egress (docs/planning/13-protobuf-contract.md RequestStart fields
// destination_policy, injection_operations, fingerprint_instruction).
type DestinationPolicyResult struct {
	Policy              *strawpb.DestinationPolicy
	InjectionOperations []*strawpb.InjectionOperation
	FingerprintProfile  string
}

// ResolveDestinationPolicy validates the target host against deny rules,
// resolves ordered header-injection operations, validates the fingerprint
// profile, and selects a destination-resolution mode. It returns a
// *ValidationError using the canonical registry codes (invalid_request,
// destination_denied, header_injection_failed, unsupported_fingerprint) on
// rejection; error messages/details never include the raw target URL/host,
// worker IDs, session IDs, or NATS subjects.
func ResolveDestinationPolicy(req DestinationPolicyRequest) (*DestinationPolicyResult, *ValidationError) {
	if req.TargetURL == nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "target url is required"}
	}

	if req.TargetURL.User != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "url userinfo is rejected"}
	}

	host, addr, isLiteralIP, verr := normalizeTargetHost(req.TargetURL)
	if verr != nil {
		return nil, verr
	}

	deniedCidrs, allowedCidrs, deniedHostSuffixes, deniedCNAMESuffixes := compileDenyRules(req.Snapshot.DenyRules)

	verr = evaluateHostDeny(host, req.Snapshot.DenyRules)
	if verr != nil {
		return nil, verr
	}

	if isLiteralIP {
		verr = evaluateLiteralIPDeny(addr, deniedCidrs, allowedCidrs)
		if verr != nil {
			return nil, verr
		}
	}

	resolutionMode, verr := resolveDestinationMode(req.UpstreamProxyEnabled, req.UpstreamProxyTrusted)
	if verr != nil {
		return nil, verr
	}

	fingerprint, verr := resolveFingerprintProfile(req.Snapshot.FingerprintProfiles, req.RequestedFingerprintProfile)
	if verr != nil {
		return nil, verr
	}

	ops, verr := resolveInjectionOperations(req.Snapshot.InjectionPolicies, req.MaxInjectedHeaderBytes)
	if verr != nil {
		return nil, verr
	}

	policy := &strawpb.DestinationPolicy{
		AllowLoopback:         allowedPrefixesContain(allowedCidrs, func(addr netip.Addr) bool { return addr.IsLoopback() }),
		AllowPrivateRanges:    allowedPrefixesContain(allowedCidrs, func(addr netip.Addr) bool { return addr.IsPrivate() }),
		AllowLinkLocal:        allowedPrefixesContain(allowedCidrs, func(addr netip.Addr) bool { return addr.IsLinkLocalUnicast() }),
		AllowMulticast:        allowedPrefixesContain(allowedCidrs, func(addr netip.Addr) bool { return addr.IsMulticast() }),
		AllowMetadataIps:      allowedPrefixesContain(allowedCidrs, isMetadataAddr),
		DeniedCidrs:           deniedCidrs,
		AllowedCidrs:          allowedCidrs,
		DeniedHostSuffixes:    deniedHostSuffixes,
		DeniedCnameSuffixes:   deniedCNAMESuffixes,
		SniHostMismatchPolicy: strawpb.SniHostMismatchPolicy_SNI_HOST_MISMATCH_STRICT,
		RedirectPolicy:        strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		PolicyVersion:         strconv.FormatUint(req.Snapshot.ConfigVersion, 10),
		ResolutionMode:        resolutionMode,
	}

	return &DestinationPolicyResult{Policy: policy, InjectionOperations: ops, FingerprintProfile: fingerprint}, nil
}

func allowedPrefixesContain(prefixes []string, match func(netip.Addr) bool) bool {
	for _, raw := range prefixes {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			continue
		}

		if match(prefix.Addr()) {
			return true
		}
	}

	return false
}

func isMetadataAddr(addr netip.Addr) bool {
	return slices.Contains(metadataIPs, addr)
}

// normalizeTargetHost lowercases, IDNA-normalizes, and trims the trailing dot
// from the URL host. It reports whether the normalized host is an IP literal.
func normalizeTargetHost(u *url.URL) (string, netip.Addr, bool, *ValidationError) {
	hostname, err := normalizeHostname(u.Hostname())
	if err != nil {
		return "", netip.Addr{}, false, &ValidationError{Code: errorCodeInvalidRequest, Message: "url host is not a valid hostname"}
	}

	parsed, err := netip.ParseAddr(hostname)
	isLiteralIP := err == nil

	return hostname, parsed, isLiteralIP, nil
}

// compileDenyRules partitions the tenant's deny rules into the bundle fields
// Egress consumes. cidr/metadata_ip/private_range all compile into the same
// denied/allowed CIDR sets (docs/implementation-history.md#p0-43: they differ from "cidr" only in
// admin-facing label, not enforcement). host/host_suffix and cname_suffix
// compile into suffix lists; an allow_override rule for the same normalized
// value cancels the matching deny entry out of the compiled list, since
// Egress's suffix lists (unlike allowed_cidrs) have no separate override
// concept (docs/planning/16 "Egress performs final resolved-IP validation" —
// cname can only be evaluated post-resolution, so cname_suffix rules are
// never evaluated Control-side the way host/host_suffix are in
// evaluateHostDeny).
func compileDenyRules(rules []config.DenyRule) ([]string, []string, []string, []string) {
	var deniedCidrs, allowedCidrs []string

	var deniedHostVals, allowedHostVals, deniedCnameVals, allowedCnameVals []string

	for _, r := range rules {
		if !r.Enabled {
			continue
		}

		switch {
		case denyRuleCIDRTypes[r.RuleType]:
			deniedCidrs, allowedCidrs = compileCIDRDenyRule(r, deniedCidrs, allowedCidrs)
		case denyRuleHostTypes[r.RuleType]:
			if r.Action == denyRuleActionAllowOverride {
				allowedHostVals = append(allowedHostVals, r.NormalizedHost)
			} else {
				deniedHostVals = append(deniedHostVals, r.NormalizedHost)
			}
		case r.RuleType == denyRuleTypeCNAMESuffix:
			if r.Action == denyRuleActionAllowOverride {
				allowedCnameVals = append(allowedCnameVals, r.NormalizedName)
			} else {
				deniedCnameVals = append(deniedCnameVals, r.NormalizedName)
			}
		}
	}

	deniedHostSuffixes := subtractOverrides(deniedHostVals, allowedHostVals)
	deniedCNAMESuffixes := subtractOverrides(deniedCnameVals, allowedCnameVals)

	sort.Strings(deniedCidrs)
	sort.Strings(allowedCidrs)
	sort.Strings(deniedHostSuffixes)
	sort.Strings(deniedCNAMESuffixes)

	return deniedCidrs, allowedCidrs, deniedHostSuffixes, deniedCNAMESuffixes
}

// subtractOverrides removes any denied value also present in allowed,
// implementing allow_override precedence for the flat suffix lists Egress
// enforces (no per-request evaluation available for host_suffix/cname_suffix
// there, unlike allowed_cidrs).
func subtractOverrides(denied, allowed []string) []string {
	if len(allowed) == 0 {
		return denied
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, v := range allowed {
		allowedSet[v] = true
	}

	out := make([]string, 0, len(denied))

	for _, v := range denied {
		if !allowedSet[v] {
			out = append(out, v)
		}
	}

	return out
}

func compileCIDRDenyRule(r config.DenyRule, deniedCidrs, allowedCidrs []string) ([]string, []string) {
	value := denyRuleValue(r)
	if value == "" {
		return deniedCidrs, allowedCidrs
	}

	if r.Action == denyRuleActionAllowOverride {
		return deniedCidrs, append(allowedCidrs, value)
	}

	return append(deniedCidrs, value), allowedCidrs
}

// evaluateHostDeny rejects the request when host matches an enabled
// host/host_suffix deny rule that has no matching enabled allow_override rule
// (docs/planning/27 "Destination Deny Normalization"). host matches exactly;
// host_suffix additionally matches subdomains, mirroring Egress's
// hostMatchesSuffix (internal/egress/executor.go).
func evaluateHostDeny(host string, rules []config.DenyRule) *ValidationError {
	denied := false
	allowed := false

	for _, r := range rules {
		if !r.Enabled || !denyRuleHostTypes[r.RuleType] || !hostMatchesDenyRule(host, r) {
			continue
		}

		if r.Action == denyRuleActionAllowOverride {
			allowed = true
		} else {
			denied = true
		}
	}

	if denied && !allowed {
		return destinationDeniedError()
	}

	return nil
}

func hostMatchesDenyRule(host string, r config.DenyRule) bool {
	if r.RuleType == denyRuleTypeHostSuffix {
		return hostMatchesSuffix(host, r.NormalizedHost)
	}

	return host == r.NormalizedHost
}

// hostMatchesSuffix mirrors internal/egress/executor.go's hostMatchesSuffix
// exactly, so host_suffix rules pre-checked here agree with Egress's
// downstream DeniedHostSuffixes enforcement.
func hostMatchesSuffix(host, suffix string) bool {
	if suffix == "" {
		return false
	}

	if host == suffix {
		return true
	}

	return strings.HasSuffix(host, "."+suffix)
}

// evaluateLiteralIPDeny is Control's fail-fast pre-dispatch check for
// IP-literal targets: private/link-local/multicast/metadata/default-denied
// ranges are denied unless the tenant has an explicit allow-type deny rule
// (allowedCidrs) covering the address, which is a true override
// (docs/planning/27 "denied by default unless a tenant admin explicitly
// allows them"). This is a pre-dispatch optimization only — Egress still
// performs the authoritative resolved-IP check after DNS resolution
// (docs/planning/16 "Dial target invariant"). internal/egress/executor.go's
// validateResolvedIP (task 26) checks AllowedCidrs first, before denied_cidrs
// or any default-deny predicate, matching this override precedence exactly.
func evaluateLiteralIPDeny(addr netip.Addr, deniedCidrs, allowedCidrs []string) *ValidationError {
	if addr.Is4In6() {
		return destinationDeniedError()
	}

	if prefixesContainAddr(parseCIDRs(allowedCidrs), addr) {
		return nil
	}

	if prefixesContainAddr(parseCIDRs(deniedCidrs), addr) {
		return destinationDeniedError()
	}

	if containsAddr(metadataIPs, addr) {
		return destinationDeniedError()
	}

	if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() || addr.IsPrivate() {
		return destinationDeniedError()
	}

	if prefixesContainAddr(defaultDeniedPrefixes, addr) {
		return destinationDeniedError()
	}

	return nil
}

func parseCIDRs(raw []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(raw))

	for _, s := range raw {
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			continue
		}

		prefixes = append(prefixes, prefix)
	}

	return prefixes
}

func prefixesContainAddr(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}

	return false
}

func containsAddr(addrs []netip.Addr, addr netip.Addr) bool {
	return slices.Contains(addrs, addr)
}

func destinationDeniedError() *ValidationError {
	return &ValidationError{Code: errorCodeDestinationDenied, Message: ErrorRegistry[DestinationDenied].Message}
}

// resolveDestinationMode selects DIRECT_LOCAL or UPSTREAM_PROXY_REMOTE per
// deployment config (docs/planning/27 "SSRF Enforcement by Resolution Mode").
// Untrusted upstream-proxy remote resolution is rejected before dispatch.
func resolveDestinationMode(upstreamProxyEnabled, upstreamProxyTrusted bool) (strawpb.DestinationResolutionMode, *ValidationError) {
	if !upstreamProxyEnabled {
		return strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL, nil
	}

	if !upstreamProxyTrusted {
		return 0, destinationDeniedError()
	}

	return strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE, nil
}

// resolveFingerprintProfile validates the requested (or tenant-default)
// fingerprint profile against the snapshot's enabled catalog entries. Worker
// capability is deliberately not consulted here: routing must first compute
// the ordinary tenant/route/session candidate set and then apply the exact
// capability filter so route, sticky, and capacity errors retain precedence.
func resolveFingerprintProfile(profiles []config.FingerprintProfile, requested string) (string, *ValidationError) {
	name := requested
	if name == "" {
		name = defaultFingerprintProfileName
	}

	if name == defaultFingerprintProfileName {
		return baselineFingerprintProfileName, nil
	}

	for _, p := range profiles {
		if p.Name == name && p.Enabled {
			return name, nil
		}
	}

	return "", &ValidationError{Code: errorCodeUnsupportedFingerprint, Message: ErrorRegistry[UnsupportedFingerprint].Message}
}

// resolveInjectionOperations concatenates every enabled injection policy's
// operations, ordered by policy ID for determinism, then re-validates the
// combined ordered list against the Section 15 safety table plus the
// resolve-time rules that policy-write-time validation
// (config_admin_handlers.go validateInjectionOperation) does not already
// cover: duplicate `set` rejection across policies, CR/LF in decoded values,
// and the aggregate size bound. It does not re-check the tenant_admin-only
// restriction on Authorization/Cookie: that is an actor-authorization rule
// enforced at write time (config_admin_handlers.go), and the resolver has no
// actor context for a stored snapshot.
func resolveInjectionOperations(policies []config.InjectionPolicy, maxBytes uint64) ([]*strawpb.InjectionOperation, *ValidationError) {
	sorted := make([]config.InjectionPolicy, len(policies))
	copy(sorted, policies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	seenSet := make(map[string]bool)

	var (
		out        []*strawpb.InjectionOperation
		totalBytes uint64
	)

	for _, pol := range sorted {
		if !pol.Enabled {
			continue
		}

		for _, op := range pol.Operations {
			resolved, verr := resolveInjectionOperation(op, seenSet)
			if verr != nil {
				return nil, verr
			}

			totalBytes += uint64(len(op.HeaderName) + len(resolved.Value))
			if maxBytes > 0 && totalBytes > maxBytes {
				return nil, &ValidationError{Code: errorCodeHeaderInjectionFailed, Message: "injected header bytes exceed configured maximum"}
			}

			out = append(out, resolved)
		}
	}

	return out, nil
}

func resolveInjectionOperation(op config.InjectionOperation, seenSet map[string]bool) (*strawpb.InjectionOperation, *ValidationError) {
	if op.Op != injectionOpSet && op.Op != injectionOpAppend && op.Op != injectionOpRemove {
		return nil, &ValidationError{Code: errorCodeHeaderInjectionFailed, Message: "resolved injection operation has an invalid op"}
	}

	lower := strings.ToLower(op.HeaderName)
	if alwaysDeniedInjectionHeaders[lower] || strings.HasPrefix(lower, deniedInjectionHeaderPrefix) {
		return nil, &ValidationError{Code: errorCodeHeaderInjectionFailed, Message: "resolved injection operation targets a denied header"}
	}

	if op.Op == injectionOpSet {
		if seenSet[lower] {
			return nil, &ValidationError{Code: errorCodeHeaderInjectionFailed, Message: "duplicate set operation for the same header"}
		}

		seenSet[lower] = true
	}

	value, err := base64.StdEncoding.DecodeString(op.ValueBase64)
	if err != nil {
		return nil, &ValidationError{Code: errorCodeHeaderInjectionFailed, Message: "resolved injection value is not valid base64"}
	}

	if strings.ContainsAny(string(value), "\r\n") {
		return nil, &ValidationError{Code: errorCodeHeaderInjectionFailed, Message: "resolved injection value contains CR or LF"}
	}

	return &strawpb.InjectionOperation{Op: op.Op, HeaderName: op.HeaderName, Value: value}, nil
}
