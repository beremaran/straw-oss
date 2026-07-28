package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

// ErrInvalidSnapshot indicates that runtime configuration cannot be activated.
var ErrInvalidSnapshot = errors.New("invalid runtime configuration")

const (
	denyRuleTypeHost         = "host"
	denyRuleTypeHostSuffix   = "host_suffix"
	denyRuleTypeCIDR         = "cidr"
	denyRuleTypeIP           = "ip"
	denyRuleTypeCNAMESuffix  = "cname_suffix"
	denyRuleTypeMetadataIP   = "metadata_ip"
	denyRuleTypePrivateRange = "private_range"
	denyRuleActionDeny       = "deny"
	denyRuleActionAllow      = "allow_override"
	injectionOpSet           = "set"
	injectionOpAppend        = "append"
	injectionOpRemove        = "remove"
	snapshotIngressConnect   = "connect"
	snapshotIngressREST      = "rest"
	snapshotIngressHTTPProxy = "http_proxy"
	metadataIP169254169254   = "169.254.169.254"
	maxSnapshotStringBytes   = 1024
	maxSnapshotListBytes     = 128
	maxSnapshotTagBytes      = 64
)

var errSnapshotFieldInvalid = errors.New("invalid runtime snapshot field")

func snapshotError(message string) error {
	return fmt.Errorf("%w: %s", errSnapshotFieldInvalid, message)
}

func wrapSnapshotError(err error) error {
	return fmt.Errorf("%w: %w", errSnapshotFieldInvalid, err)
}

var snapshotIDAllowed = func() [256]bool {
	var allowed [256]bool

	for c := byte('a'); c <= 'z'; c++ {
		allowed[c] = true
	}

	for c := byte('A'); c <= 'Z'; c++ {
		allowed[c] = true
	}

	for c := byte('0'); c <= '9'; c++ {
		allowed[c] = true
	}

	for _, c := range []byte{'-', '_', '.', ':'} {
		allowed[c] = true
	}

	return allowed
}()

var snapshotHTTPTokenAllowed = func() [256]bool {
	var allowed [256]bool

	for c := byte('0'); c <= '9'; c++ {
		allowed[c] = true
	}

	for c := byte('A'); c <= 'Z'; c++ {
		allowed[c] = true
	}

	for c := byte('a'); c <= 'z'; c++ {
		allowed[c] = true
	}

	for _, c := range []byte("!#$%&'*+-.^_`|~") {
		allowed[c] = true
	}

	return allowed
}()

var snapshotIDNA = idna.New(idna.MapForLookup(), idna.VerifyDNSLength(true))

// PrepareSnapshot returns the canonical, immutable input used for validation,
// persistence, publication, and activation. Normalized destination fields are
// derived from RawPattern; caller-supplied normalized values are never trusted.
func PrepareSnapshot(s Snapshot) (Snapshot, error) {
	out := s.Clone()

	err := normalizeDestinationRules(out.DenyRules)
	if err != nil {
		return Snapshot{}, err
	}

	err = validateSnapshot(out)
	if err != nil {
		return Snapshot{}, err
	}

	return out, nil
}

// ValidateSnapshot rejects a runtime configuration before it can become active.
func ValidateSnapshot(s Snapshot) error {
	_, err := PrepareSnapshot(s)

	return err
}

func validateSnapshot(s Snapshot) error {
	if s.ConfigVersion == 0 {
		return fmt.Errorf("%w: config_version must be positive", ErrInvalidSnapshot)
	}

	if s.DefaultTimeoutMs == 0 || s.MaxTimeoutMs == 0 || s.DefaultTimeoutMs > s.MaxTimeoutMs {
		return fmt.Errorf("%w: timeouts must be positive and default_timeout_ms must not exceed max_timeout_ms", ErrInvalidSnapshot)
	}

	poolIDs, err := validatePools(s.ExecutorPools)
	if err != nil {
		return err
	}

	err = validateRoutes(s.RoutingRules, poolIDs)
	if err != nil {
		return err
	}

	err = validateDestinationRules(s.DenyRules)
	if err != nil {
		return err
	}

	err = validateInjectionPolicies(s.InjectionPolicies)
	if err != nil {
		return err
	}

	err = validateFingerprintProfiles(s.FingerprintProfiles)
	if err != nil {
		return err
	}

	return validateWorkerSettings(s.WorkerSettings)
}

func validatePools(pools []ExecutorPool) (map[string]struct{}, error) {
	ids := make(map[string]struct{}, len(pools))

	for _, pool := range pools {
		err := validatePool(pool, ids)
		if err != nil {
			return nil, err
		}

		ids[pool.ID] = struct{}{}
	}

	return ids, nil
}

func validatePool(pool ExecutorPool, ids map[string]struct{}) error {
	if pool.ID == "" || pool.ExecutorType == "" {
		return fmt.Errorf("%w: pool id and executor_type are required", ErrInvalidSnapshot)
	}

	if _, exists := ids[pool.ID]; exists {
		return fmt.Errorf("%w: duplicate pool %q", ErrInvalidSnapshot, pool.ID)
	}

	if !validSnapshotID(pool.ID) || !validSnapshotString(pool.ExecutorType, maxSnapshotStringBytes) {
		return fmt.Errorf("%w: pool %q has an invalid id or executor_type", ErrInvalidSnapshot, pool.ID)
	}

	err := validatePoolUpstreamProxy(pool)
	if err != nil {
		return err
	}

	err = validateStringList("pool tags", pool.Tags, maxSnapshotTagBytes)
	if err != nil {
		return err
	}

	return validatePoolRestrictions(pool)
}

func validatePoolUpstreamProxy(pool ExecutorPool) error {
	if pool.UpstreamProxy == nil {
		return nil
	}

	if !validSnapshotID(pool.UpstreamProxy.ID) {
		return fmt.Errorf("%w: pool %q has an invalid upstream proxy id", ErrInvalidSnapshot, pool.ID)
	}

	if !pool.UpstreamProxy.TrustedRemoteResolution {
		return fmt.Errorf("%w: pool %q upstream proxy must enable trusted_remote_resolution", ErrInvalidSnapshot, pool.ID)
	}

	return nil
}

func validatePoolRestrictions(pool ExecutorPool) error {
	for name, values := range map[string][]string{
		"allowed_ip_types":  pool.AllowedIPTypes,
		"allowed_countries": pool.AllowedCountries,
		"allowed_regions":   pool.AllowedRegions,
	} {
		err := validateStringList("pool "+name, values, maxSnapshotListBytes)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateRoutes(routes []RoutingRule, pools map[string]struct{}) error {
	ids := make(map[string]struct{}, len(routes))
	for _, rule := range routes {
		if rule.ID == "" {
			return fmt.Errorf("%w: route id is required", ErrInvalidSnapshot)
		}

		if _, exists := ids[rule.ID]; exists {
			return fmt.Errorf("%w: duplicate route %q", ErrInvalidSnapshot, rule.ID)
		}

		ids[rule.ID] = struct{}{}
		if _, exists := pools[rule.TargetPoolID]; !exists {
			return fmt.Errorf("%w: route %q targets unknown pool %q", ErrInvalidSnapshot, rule.ID, rule.TargetPoolID)
		}

		if !validSnapshotID(rule.ID) || !validSnapshotID(rule.TargetPoolID) {
			return fmt.Errorf("%w: route %q has an invalid id or target_pool_id", ErrInvalidSnapshot, rule.ID)
		}

		if rule.AllowStickyFallback && rule.StickySessionTTLSeconds == 0 {
			return fmt.Errorf("%w: route %q enables sticky fallback without a sticky TTL", ErrInvalidSnapshot, rule.ID)
		}

		err := validateMatchConditions(rule.Match)
		if err != nil {
			return fmt.Errorf("%w: route %q match: %w", ErrInvalidSnapshot, rule.ID, err)
		}
	}

	return nil
}

func validateWorkerSettings(settings []WorkerSetting) error {
	ids := make(map[string]struct{}, len(settings))
	for _, worker := range settings {
		if worker.WorkerID == "" {
			return fmt.Errorf("%w: worker_id is required", ErrInvalidSnapshot)
		}

		if _, exists := ids[worker.WorkerID]; exists {
			return fmt.Errorf("%w: duplicate worker setting %q", ErrInvalidSnapshot, worker.WorkerID)
		}

		if !validSnapshotID(worker.WorkerID) {
			return fmt.Errorf("%w: worker setting %q has an invalid worker_id", ErrInvalidSnapshot, worker.WorkerID)
		}

		ids[worker.WorkerID] = struct{}{}
	}

	return nil
}

func validateMatchConditions(match MatchConditions) error {
	err := validateStringList("tags", match.Tags, maxSnapshotTagBytes)
	if err != nil {
		return err
	}

	err = validateMatchCountry(match.Country)
	if err != nil {
		return err
	}

	err = validateMatchValues(match)
	if err != nil {
		return err
	}

	return validateMatchHost(match.TargetHost)
}

func validateMatchCountry(country string) error {
	if country != "" && (len(country) != 2 || strings.ToUpper(country) != country || !asciiLetters(country)) {
		return fmt.Errorf("%w: country must be an uppercase two-letter code", errSnapshotFieldInvalid)
	}

	return nil
}

func validateMatchValues(match MatchConditions) error {
	values := map[string]string{"region": match.Region, "ip_type": match.IPType, "ingress_type": match.IngressType}
	for name, value := range values {
		if value != "" && (!validSnapshotString(value, maxSnapshotListBytes) || strings.TrimSpace(value) != value) {
			return fmt.Errorf("%w: %s is invalid", errSnapshotFieldInvalid, name)
		}
	}

	if match.IngressType != "" && match.IngressType != snapshotIngressREST && match.IngressType != snapshotIngressHTTPProxy && match.IngressType != snapshotIngressConnect {
		return fmt.Errorf("%w: ingress_type is unsupported", errSnapshotFieldInvalid)
	}

	return nil
}

func validateMatchHost(host string) error {
	if host == "" {
		return nil
	}

	pattern := strings.TrimPrefix(host, "*.")

	normalized, err := normalizeHostPattern(pattern)
	if err != nil {
		return fmt.Errorf("%w: target_host is invalid: %w", errSnapshotFieldInvalid, err)
	}

	if host != normalized && host != "*."+normalized {
		return fmt.Errorf("%w: target_host must be normalized", errSnapshotFieldInvalid)
	}

	return nil
}

func validateDestinationRules(rules []DenyRule) error {
	ids := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		if rule.ID == "" || !validSnapshotID(rule.ID) {
			return fmt.Errorf("%w: destination rule id is invalid", ErrInvalidSnapshot)
		}

		if _, exists := ids[rule.ID]; exists {
			return fmt.Errorf("%w: duplicate destination rule %q", ErrInvalidSnapshot, rule.ID)
		}

		ids[rule.ID] = struct{}{}
		if rule.Action != denyRuleActionDeny && rule.Action != denyRuleActionAllow {
			return fmt.Errorf("%w: destination rule %q has unsupported action", ErrInvalidSnapshot, rule.ID)
		}

		if !validSnapshotString(rule.Reason, maxSnapshotStringBytes) {
			return fmt.Errorf("%w: destination rule %q has an invalid reason", ErrInvalidSnapshot, rule.ID)
		}

		err := validateNormalizedDestination(rule)
		if err != nil {
			return fmt.Errorf("%w: destination rule %q: %w", ErrInvalidSnapshot, rule.ID, err)
		}
	}

	return nil
}

func normalizeDestinationRules(rules []DenyRule) error {
	for i := range rules {
		rule := &rules[i]

		err := normalizeDestinationRule(rule)
		if err != nil {
			return fmt.Errorf("%w: destination rule %q raw_pattern is invalid: %w", ErrInvalidSnapshot, rule.ID, err)
		}
	}

	return nil
}

func normalizeDestinationRule(rule *DenyRule) error {
	if rule.RawPattern == "" {
		if !hasNormalizedDestination(*rule) {
			return snapshotError("raw_pattern is required")
		}

		err := canonicalizeLegacyDestination(rule)
		if err != nil {
			return fmt.Errorf("invalid legacy normalization: %w", err)
		}

		return nil
	}

	raw := strings.TrimSpace(rule.RawPattern)
	if raw == "" {
		return snapshotError("raw_pattern is invalid")
	}

	rule.NormalizedHost, rule.NormalizedCIDR, rule.NormalizedIP, rule.NormalizedName = "", "", "", ""
	switch rule.RuleType {
	case denyRuleTypeHost:
		return normalizeRawHost(rule, raw)
	case denyRuleTypeHostSuffix, denyRuleTypeCNAMESuffix:
		return normalizeRawSuffix(rule, raw)
	case denyRuleTypeCIDR, denyRuleTypePrivateRange:
		return normalizeRawCIDR(rule, raw)
	case denyRuleTypeIP, denyRuleTypeMetadataIP:
		return normalizeRawIP(rule, raw)
	default:
		return snapshotError("unsupported rule_type")
	}
}

func normalizeRawHost(rule *DenyRule, raw string) error {
	normalized, err := normalizeHostPattern(raw)
	if err != nil {
		return err
	}

	_, ipErr := netip.ParseAddr(normalized)
	if ipErr == nil {
		return snapshotError("host rules must contain a hostname, not an IP")
	}

	rule.NormalizedHost = normalized

	return nil
}

func normalizeRawSuffix(rule *DenyRule, raw string) error {
	raw = strings.TrimPrefix(raw, "*.")

	raw = strings.TrimPrefix(raw, ".")
	if raw == "" || strings.ContainsAny(raw, "*?/") {
		return snapshotError("suffix is malformed")
	}

	normalized, err := normalizeHostPattern(raw)
	if err != nil {
		return err
	}

	_, ipErr := netip.ParseAddr(normalized)
	if ipErr == nil {
		return snapshotError("suffix rules must contain a hostname, not an IP")
	}

	if rule.RuleType == denyRuleTypeHostSuffix {
		rule.NormalizedHost = normalized
	} else {
		rule.NormalizedName = normalized
	}

	return nil
}

func normalizeRawCIDR(rule *DenyRule, raw string) error {
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return wrapSnapshotError(err)
	}

	prefix = prefix.Masked()
	if rule.RuleType == denyRuleTypePrivateRange && !prefix.Addr().IsPrivate() {
		return snapshotError("private_range must be a private CIDR")
	}

	rule.NormalizedCIDR = prefix.String()

	return nil
}

func normalizeRawIP(rule *DenyRule, raw string) error {
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return wrapSnapshotError(err)
	}

	addr = addr.Unmap()
	if rule.RuleType == denyRuleTypeMetadataIP && !isMetadataIP(addr) {
		return snapshotError("metadata_ip is not a recognized metadata address")
	}

	rule.NormalizedIP = addr.String()

	return nil
}

func canonicalizeLegacyDestination(rule *DenyRule) error {
	host, cidr, ip, name := rule.NormalizedHost, rule.NormalizedCIDR, rule.NormalizedIP, rule.NormalizedName

	rule.NormalizedHost, rule.NormalizedCIDR, rule.NormalizedIP, rule.NormalizedName = "", "", "", ""
	switch rule.RuleType {
	case denyRuleTypeHost:
		return normalizeRawHost(rule, host)
	case denyRuleTypeHostSuffix:
		return normalizeRawSuffix(rule, host)
	case denyRuleTypeCNAMESuffix:
		return normalizeRawSuffix(rule, name)
	case denyRuleTypeCIDR, denyRuleTypePrivateRange:
		return normalizeRawCIDR(rule, cidr)
	case denyRuleTypeIP, denyRuleTypeMetadataIP:
		return normalizeRawIP(rule, ip)
	default:
		return snapshotError("unsupported rule_type")
	}
}

func validateNormalizedDestination(rule DenyRule) error {
	if countNormalizedDestination(rule) != 1 {
		return snapshotError("exactly one normalized destination field is required")
	}

	switch rule.RuleType {
	case denyRuleTypeHost:
		return validateNormalizedHost(rule.NormalizedHost, true)
	case denyRuleTypeHostSuffix:
		return validateNormalizedHost(rule.NormalizedHost, false)
	case denyRuleTypeCNAMESuffix:
		return validateNormalizedHost(rule.NormalizedName, false)
	case denyRuleTypeCIDR, denyRuleTypePrivateRange:
		return validateNormalizedCIDR(rule)
	case denyRuleTypeIP, denyRuleTypeMetadataIP:
		return validateNormalizedIP(rule)
	default:
		return snapshotError("unsupported rule_type")
	}
}

func countNormalizedDestination(rule DenyRule) int {
	count := 0

	for _, value := range []string{rule.NormalizedHost, rule.NormalizedCIDR, rule.NormalizedIP, rule.NormalizedName} {
		if value != "" {
			count++
		}
	}

	return count
}

func validateNormalizedHost(value string, rejectIP bool) error {
	if value == "" {
		return snapshotError("hostname normalization is empty")
	}

	normalized, err := normalizeHostPattern(value)
	if err != nil || normalized != value {
		return snapshotError("hostname normalization is invalid")
	}

	if rejectIP {
		_, parseErr := netip.ParseAddr(value)
		if parseErr == nil {
			return snapshotError("host rules must contain a hostname, not an IP")
		}
	}

	return nil
}

func validateNormalizedCIDR(rule DenyRule) error {
	prefix, err := netip.ParsePrefix(rule.NormalizedCIDR)
	if err != nil || prefix.String() != rule.NormalizedCIDR {
		return snapshotError("CIDR normalization is invalid")
	}

	if rule.RuleType == denyRuleTypePrivateRange && !prefix.Addr().IsPrivate() {
		return snapshotError("private_range must be a private CIDR")
	}

	return nil
}

func validateNormalizedIP(rule DenyRule) error {
	addr, err := netip.ParseAddr(rule.NormalizedIP)
	if err != nil || addr.String() != rule.NormalizedIP {
		return snapshotError("IP normalization is invalid")
	}

	if rule.RuleType == denyRuleTypeMetadataIP && !isMetadataIP(addr) {
		return snapshotError("IP normalization is invalid")
	}

	return nil
}

func hasNormalizedDestination(rule DenyRule) bool {
	return rule.NormalizedHost != "" || rule.NormalizedCIDR != "" || rule.NormalizedIP != "" || rule.NormalizedName != ""
}

func normalizeHostPattern(raw string) (string, error) {
	raw = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if raw == "" || strings.ContainsAny(raw, " /\\\t\r\n") {
		return "", snapshotError("hostname is empty or contains invalid characters")
	}

	addr, err := netip.ParseAddr(raw)
	if err == nil {
		return addr.Unmap().String(), nil
	}

	ascii, err := snapshotIDNA.ToASCII(raw)
	if err != nil || ascii == "" || len(ascii) > 253 {
		return "", snapshotError("hostname is not valid IDNA")
	}

	ascii = strings.ToLower(strings.TrimSuffix(ascii, "."))

	err = validateHostnameLabels(ascii)
	if err != nil {
		return "", err
	}

	return ascii, nil
}

func validateHostnameLabels(host string) error {
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return snapshotError("hostname label is invalid")
		}

		err := validateHostnameLabel(label)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateHostnameLabel(label string) error {
	for _, c := range []byte(label) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}

		return snapshotError("hostname label contains an invalid character")
	}

	return nil
}

func validateInjectionPolicies(policies []InjectionPolicy) error {
	ids := make(map[string]struct{}, len(policies))
	seenSet := make(map[string]struct{})

	for _, policy := range policies {
		if policy.ID == "" || !validSnapshotID(policy.ID) {
			return fmt.Errorf("%w: injection policy id is invalid", ErrInvalidSnapshot)
		}

		if _, exists := ids[policy.ID]; exists {
			return fmt.Errorf("%w: duplicate injection policy %q", ErrInvalidSnapshot, policy.ID)
		}

		ids[policy.ID] = struct{}{}

		err := validateInjectionOperations(policy, seenSet)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateInjectionOperations(policy InjectionPolicy, seenSet map[string]struct{}) error {
	for _, op := range policy.Operations {
		err := validateInjectionOperation(policy.ID, op)
		if err != nil {
			return err
		}

		if policy.Enabled && op.Op == injectionOpSet {
			key := strings.ToLower(op.HeaderName)
			if _, exists := seenSet[key]; exists {
				return fmt.Errorf("%w: duplicate set operation for header %q", ErrInvalidSnapshot, op.HeaderName)
			}

			seenSet[key] = struct{}{}
		}
	}

	return nil
}

func validateInjectionOperation(policyID string, op InjectionOperation) error {
	if op.Op != injectionOpSet && op.Op != injectionOpAppend && op.Op != injectionOpRemove {
		return fmt.Errorf("%w: injection policy %q has an unsupported operation", ErrInvalidSnapshot, policyID)
	}

	if !validHTTPHeaderName(op.HeaderName) || isDeniedInjectionHeader(op.HeaderName) {
		return fmt.Errorf("%w: injection policy %q has an invalid or denied header", ErrInvalidSnapshot, policyID)
	}

	value, err := base64.StdEncoding.DecodeString(op.ValueBase64)
	if err != nil || strings.ContainsAny(string(value), "\r\n") {
		return fmt.Errorf("%w: injection policy %q has an invalid value", ErrInvalidSnapshot, policyID)
	}

	return nil
}

func validateFingerprintProfiles(profiles []FingerprintProfile) error {
	seen := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		err := validateFingerprintProfile(profile, seen)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateFingerprintProfile(profile FingerprintProfile, seen map[string]struct{}) error {
	if profile.Name == "" || !validSnapshotString(profile.Name, maxSnapshotListBytes) {
		return fmt.Errorf("%w: fingerprint profile name is invalid", ErrInvalidSnapshot)
	}

	if _, exists := seen[profile.Name]; exists {
		return fmt.Errorf("%w: duplicate fingerprint profile %q", ErrInvalidSnapshot, profile.Name)
	}

	seen[profile.Name] = struct{}{}
	if profile.ScopeType != "" && profile.ScopeType != "global" {
		return fmt.Errorf("%w: fingerprint profile %q has unsupported scope_type", ErrInvalidSnapshot, profile.Name)
	}

	if profile.Name == "default" {
		return nil
	}

	return validateFingerprintMetadata(profile)
}

func validateFingerprintMetadata(profile FingerprintProfile) error {
	if profile.Enabled && (!profile.SupportedByWorker || profile.ExecutorType == "" || profile.ProfileRef == "" || profile.ContractRevision == "") {
		return fmt.Errorf("%w: enabled fingerprint profile %q is not executable", ErrInvalidSnapshot, profile.Name)
	}

	if !validSnapshotString(profile.ExecutorType, maxSnapshotListBytes) || !validSnapshotString(profile.ProfileRef, maxSnapshotListBytes) || !validSnapshotString(profile.ContractRevision, maxSnapshotListBytes) {
		return fmt.Errorf("%w: fingerprint profile %q has invalid metadata", ErrInvalidSnapshot, profile.Name)
	}

	return nil
}

func validateStringList(name string, values []string, maxBytes int) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validSnapshotString(value, maxBytes) || strings.TrimSpace(value) != value || value == "" {
			return fmt.Errorf("%w: %s contains an invalid value", ErrInvalidSnapshot, name)
		}

		if _, exists := seen[value]; exists {
			return fmt.Errorf("%w: %s contains a duplicate value", ErrInvalidSnapshot, name)
		}

		seen[value] = struct{}{}
	}

	return nil
}

func validSnapshotString(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value)
}

func validSnapshotID(value string) bool {
	if value == "" || len(value) > MaxUpstreamProxyIDBytes {
		return false
	}

	for _, c := range []byte(value) {
		if !snapshotIDAllowed[c] {
			return false
		}
	}

	return true
}

func validHTTPHeaderName(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}

	for _, c := range []byte(value) {
		if !snapshotHTTPTokenAllowed[c] {
			return false
		}
	}

	return true
}

func isDeniedInjectionHeader(value string) bool {
	lower := strings.ToLower(value)

	return lower == denyRuleTypeHost || lower == "content-length" || lower == "transfer-encoding" || lower == "connection" || lower == "proxy-authorization" || strings.HasPrefix(lower, "x-straw-")
}

func asciiLetters(value string) bool {
	for _, c := range []byte(value) {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}

	return true
}

func isMetadataIP(addr netip.Addr) bool {
	for _, raw := range []string{metadataIP169254169254, "169.254.169.253", "169.254.170.2", "100.100.100.200", "100.100.100.201"} {
		if addr == netip.MustParseAddr(raw) {
			return true
		}
	}

	return false
}
