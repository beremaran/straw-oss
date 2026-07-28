package egress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/net/http2"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type executionError struct {
	code           strawpb.ErrorCode
	fact           string
	timeoutType    strawpb.TimeoutType
	upstreamStatus *uint32
}

func (e *executionError) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.fact)
}

func executorFailure(code strawpb.ErrorCode, fact string) *executionError {
	return &executionError{code: code, fact: fact}
}

func timeoutFailure() *executionError {
	return &executionError{
		code:        strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED,
		fact:        deadlineExceededFact,
		timeoutType: strawpb.TimeoutType_TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT,
	}
}

func mapHTTPError(ctx context.Context, err error) *executionError {
	var failure *executionError
	if errors.As(err, &failure) {
		return failure
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return timeoutFailure()
	}

	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_CANCELLED, requestCancelledFact)
	}

	return mapHTTPErrorOther(err)
}

func mapHTTPErrorOther(err error) *executionError {
	if h2Err, ok := mapHTTP2Error(err); ok {
		return h2Err
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED, tcpRefusedFact)
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, upstreamResetFact)
	}

	if looksLikeTLSError(err) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE, tlsHandshakeFailedFact)
	}

	return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, executorInternalFact)
}

func mapHTTP2Error(err error) (*executionError, bool) {
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return mapHTTP2StreamError(streamErr.Code), true
	}

	s := strings.ToLower(err.Error())
	if strings.Contains(s, "http2") && strings.Contains(s, "stream") {
		if strings.Contains(s, "refused_stream") {
			return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "upstream_reset"), true
		}

		if strings.Contains(s, "http_1_1_required") {
			return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "http_1_1_required"), true
		}
	}

	return nil, false
}

func mapHTTP2StreamError(code http2.ErrCode) *executionError {
	switch code {
	case http2.ErrCodeConnect:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED, "upstream_connection_refused")
	case http2.ErrCodeInadequateSecurity:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE, "upstream_tls_failure")
	case http2.ErrCodeEnhanceYourCalm:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "enhance_your_calm")
	case http2.ErrCodeHTTP11Required:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "http_1_1_required")
	case http2.ErrCodeNo,
		http2.ErrCodeProtocol,
		http2.ErrCodeInternal,
		http2.ErrCodeFlowControl,
		http2.ErrCodeSettingsTimeout,
		http2.ErrCodeStreamClosed,
		http2.ErrCodeFrameSize,
		http2.ErrCodeRefusedStream,
		http2.ErrCodeCancel,
		http2.ErrCodeCompression:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "upstream_reset")
	default:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "upstream_reset")
	}
}

func looksLikeTLSError(err error) bool {
	s := strings.ToLower(err.Error())

	return strings.Contains(s, "tls") || strings.Contains(s, "certificate")
}

func netIPAddr(ip net.IPAddr) (netip.Addr, bool) {
	if v4 := ip.IP.To4(); v4 != nil {
		addr, ok := netip.AddrFromSlice(v4)

		return addr, ok
	}

	addr, ok := netip.AddrFromSlice(ip.IP)

	return addr, ok
}

// validateResolvedIP applies the precedence order Control's resolver promises
// (internal/control/destination_policy.go evaluateLiteralIPDeny):
// Is4In6 (unconditional) -> allowed_cidrs override -> denied_cidrs ->
// metadata/private/loopback/link-local/multicast -> default-deny prefixes.
// allowed_cidrs is a true override: a match short-circuits every later check,
// including denied_cidrs. Is4In6 is never overridable.
func validateResolvedIP(policy *strawpb.DestinationPolicy, addr netip.Addr) *executionError {
	if policy == nil {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidDestinationFact)
	}

	if addr.Is4In6() {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, dnsDeniedIPFact)
	}

	allowed, failure := parsePrefixes(policy.GetAllowedCidrs())
	if failure != nil {
		return failure
	}

	if prefixesContain(allowed, addr) {
		return nil
	}

	denied, failure := parsePrefixes(policy.GetDeniedCidrs())
	if failure != nil {
		return failure
	}

	if prefixesContain(denied, addr) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, dnsDeniedIPFact)
	}

	if deniedByDefault(policy, addr) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, dnsDeniedIPFact)
	}

	return nil
}

// validateHostSuffixPolicy rejects a request whose target host matches a
// denied_host_suffixes entry as an exact match or dot-boundary suffix.
func validateHostSuffixPolicy(host string, policy *strawpb.DestinationPolicy) *executionError {
	if matchesAnySuffix(host, policy.GetDeniedHostSuffixes()) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, hostDeniedSuffixFact)
	}

	return nil
}

// validateCNAMESuffixPolicy rejects a request whose CNAME chain (any hop)
// matches a denied_cname_suffixes entry.
func validateCNAMESuffixPolicy(ctx context.Context, resolver Resolver, host string, policy *strawpb.DestinationPolicy) *executionError {
	suffixes := policy.GetDeniedCnameSuffixes()
	if len(suffixes) == 0 {
		return nil
	}

	cnames, lookupErr := resolver.LookupCNAME(ctx, host)
	if lookupErr == nil {
		normalizedHost := strings.TrimSuffix(strings.ToLower(host), ".")
		for _, cname := range cnames {
			cn := strings.TrimSuffix(strings.ToLower(cname), ".")
			if cn != "" && cn != normalizedHost && matchesAnySuffix(cn, suffixes) {
				return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, cnameDeniedSuffixFact)
			}
		}
	}

	return nil
}

func matchesAnySuffix(host string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if hostMatchesSuffix(host, suffix) {
			return true
		}
	}

	return false
}

func hostMatchesSuffix(host, suffix string) bool {
	if suffix == "" {
		return false
	}

	if host == suffix {
		return true
	}

	return strings.HasSuffix(host, "."+suffix)
}

func deniedByDefault(policy *strawpb.DestinationPolicy, addr netip.Addr) bool {
	if blockedUnless(policy.GetAllowMetadataIps(), isMetadataIP(addr)) {
		return true
	}

	if blockedUnless(policy.GetAllowLoopback(), addr.IsLoopback()) {
		return true
	}

	if blockedUnless(policy.GetAllowLinkLocal(), addr.IsLinkLocalUnicast()) {
		return true
	}

	if blockedUnless(policy.GetAllowMulticast(), addr.IsMulticast()) {
		return true
	}

	if blockedUnless(policy.GetAllowPrivateRanges(), addr.IsPrivate()) {
		return true
	}

	return prefixesContain(defaultDeniedPrefixes, addr)
}

func blockedUnless(allowed bool, blocked bool) bool {
	return blocked && !allowed
}

func parsePrefixes(raw []string) ([]netip.Prefix, *executionError) {
	prefixes := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidDestinationFact)
		}

		prefixes = append(prefixes, prefix)
	}

	return prefixes, nil
}

func prefixesContain(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

func isMetadataIP(addr netip.Addr) bool {
	return slices.Contains(metadataIPs, addr)
}

var metadataIPs = []netip.Addr{
	netip.MustParseAddr("169.254.169.254"),
	netip.MustParseAddr("169.254.169.253"),
	netip.MustParseAddr("169.254.170.2"),
	netip.MustParseAddr("100.100.100.200"),
	netip.MustParseAddr("100.100.100.201"),
}

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
