package egress

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	fhttphttp2 "github.com/bogdanfinn/fhttp/http2"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/http2"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
	sdkegress "github.com/beremaran/straw-oss/sdk/egress"
)

const (
	schemeHTTPS             = "https"
	defaultFallbackCacheTTL = 5 * time.Minute
	errorFactDetailKey      = "fact"

	injectionFactFailed     = "header_injection_failed"
	invalidRequestStartFact = "invalid_request_start"
	invalidDestinationFact  = "invalid_destination_policy"
	unsupportedModeFact     = "unsupported_resolution_mode"
	dnsDeniedIPFact         = "dns_denied_ip"
	hostDeniedSuffixFact    = "host_denied_suffix"
	cnameDeniedSuffixFact   = "cname_denied_suffix"
	dnsNoRecordsFact        = "dns_no_records"
	tcpRefusedFact          = "tcp_refused"
	tlsHandshakeFailedFact  = "tls_handshake_failed"
	deadlineExceededFact    = "deadline_exceeded_total"
	upstreamResetFact       = "upstream_reset_before_headers"
	executorInternalFact    = "executor_internal_error"
	unsupportedFingerprint  = "unsupported_fingerprint_profile"
	requestCancelledFact    = "request_cancelled"
	opSet                   = "set"
	opAppend                = "append"
	opRemove                = "remove"
	defaultHTTPPort         = "80"
	defaultHTTPSPort        = "443"
	responseFrameDataBytes  = 32 << 10
	defaultPoolIdleTimeout  = 30 * time.Second
	defaultPoolMaxLifetime  = 5 * time.Minute
)

// Resolver is the DNS boundary Egress uses before validating resolved IPs.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)

	// LookupCNAME returns all CNAME hops for host (each lowercase, trailing dot
	// stripped), or a slice containing only host if there are no CNAMEs.
	LookupCNAME(ctx context.Context, host string) ([]string, error)
}

// ExecutorOptions configures the P0 outbound executor.
type ExecutorOptions struct {
	Resolver           Resolver
	DialContext        func(context.Context, string, string) (net.Conn, error)
	Pool               UpstreamConnectionPoolOptions
	HTTP2Enabled       bool
	FallbackCacheTTL   time.Duration
	RootCAs            *x509.CertPool
	InsecureSkipVerify bool
	BodyRefHTTPClient  *http.Client
	Now                func() time.Time
}

// Executor performs P0 decoded HTTP/HTTPS outbound execution.
type Executor struct {
	resolver           Resolver
	dialContext        func(context.Context, string, string) (net.Conn, error)
	pool               *upstreamConnectionPool
	http2Enabled       bool
	fallbackCacheTTL   time.Duration
	http11Cache        sync.Map
	rootCAs            *x509.CertPool
	insecureSkipVerify bool
	bodyRefHTTPClient  *http.Client
	now                func() time.Time
}

// UpstreamConnectionPoolOptions configures optional direct-local HTTP
// connection reuse. Zero values keep pooling disabled.
type UpstreamConnectionPoolOptions struct {
	Enabled             bool
	MaxIdleConnsPerHost int
	IdleTimeout         time.Duration
	MaxLifetime         time.Duration
}

type defaultResolver struct{}

func (defaultResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("lookup IP %q: %w", host, err)
	}

	return ips, nil
}

func (defaultResolver) LookupCNAME(ctx context.Context, host string) ([]string, error) {
	return lookupCNAMEChain(ctx, host)
}

const (
	dnsQueryID    = 1234
	dnsMaxUDPSize = 512
	dnsMinParts   = 2
	dnsTimeout    = 500 * time.Millisecond
)

func getNameservers() []string {
	var servers []string

	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}

	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= dnsMinParts {
				servers = append(servers, parts[1])
			}
		}
	}

	return servers
}

func buildDNSQuery(host string) ([]byte, error) {
	if !strings.HasSuffix(host, ".") {
		host += "."
	}

	name, err := dnsmessage.NewName(host)
	if err != nil {
		return nil, fmt.Errorf("new name: %w", err)
	}

	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 dnsQueryID,
			Response:           false,
			OpCode:             0,
			Authoritative:      false,
			Truncated:          false,
			RecursionDesired:   true,
			RecursionAvailable: false,
			RCode:              dnsmessage.RCodeSuccess,
		},
		Questions: []dnsmessage.Question{
			{
				Name:  name,
				Type:  dnsmessage.TypeCNAME,
				Class: dnsmessage.ClassINET,
			},
		},
	}

	packed, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack message: %w", err)
	}

	return packed, nil
}

func sendUDPQuery(ctx context.Context, server string, packed []byte) ([]byte, error) {
	addr := server
	if !strings.Contains(addr, ":") {
		addr += ":53"
	}

	var d net.Dialer

	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial DNS server: %w", err)
	}

	defer func() {
		_ = conn.Close()
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	_, err = conn.Write(packed)
	if err != nil {
		return nil, fmt.Errorf("conn write: %w", err)
	}

	resp := make([]byte, dnsMaxUDPSize)

	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("conn read: %w", err)
	}

	return resp[:n], nil
}

func parseCNAMEAnswers(resp []byte) ([]string, error) {
	var respMsg dnsmessage.Message

	err := respMsg.Unpack(resp)
	if err != nil {
		return nil, fmt.Errorf("unpack message: %w", err)
	}

	var cnames []string

	for _, ans := range respMsg.Answers {
		if ans.Header.Type == dnsmessage.TypeCNAME {
			cnameBody, ok := ans.Body.(*dnsmessage.CNAMEResource)
			if ok {
				cnames = append(cnames, strings.TrimSuffix(strings.ToLower(cnameBody.CNAME.String()), "."))
			}
		}
	}

	return cnames, nil
}

func queryCNAME(ctx context.Context, server, host string) ([]string, error) {
	packed, err := buildDNSQuery(host)
	if err != nil {
		return nil, err
	}

	resp, err := sendUDPQuery(ctx, server, packed)
	if err != nil {
		return nil, err
	}

	cnames, err := parseCNAMEAnswers(resp)
	if err != nil {
		return nil, err
	}

	return cnames, nil
}

func lookupCNAMEChain(ctx context.Context, host string) ([]string, error) {
	servers := getNameservers()
	servers = append(servers, "8.8.8.8", "1.1.1.1")

	var chain []string

	visited := make(map[string]bool)

	current := strings.TrimSuffix(strings.ToLower(host), ".")

	for !visited[current] {
		visited[current] = true

		var (
			nextHop  string
			queryErr error
		)

		for _, server := range servers {
			subCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
			cnames, err := queryCNAME(subCtx, server, current)

			cancel()

			if err == nil {
				if len(cnames) > 0 {
					nextHop = cnames[0]

					break
				}

				break
			}

			queryErr = err
		}

		if nextHop == "" {
			if queryErr != nil && len(chain) == 0 {
				return nil, queryErr
			}

			break
		}

		chain = append(chain, nextHop)
		current = nextHop
	}

	return chain, nil
}

// NewExecutor builds an executor with P0 transport defaults.
func NewExecutor(opts ExecutorOptions) *Executor {
	resolver := opts.Resolver
	if resolver == nil {
		resolver = defaultResolver{}
	}

	dialContext := opts.DialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{}).DialContext
	}

	exec := &Executor{
		resolver:           resolver,
		dialContext:        dialContext,
		http2Enabled:       opts.HTTP2Enabled,
		fallbackCacheTTL:   opts.FallbackCacheTTL,
		rootCAs:            opts.RootCAs,
		insecureSkipVerify: opts.InsecureSkipVerify,
		bodyRefHTTPClient:  opts.BodyRefHTTPClient,
		now:                opts.Now,
	}

	if exec.now == nil {
		exec.now = time.Now
	}

	exec.pool = newUpstreamConnectionPool(opts.Pool, dialContext, exec)

	return exec
}

// NewP0Transport returns the official P0 HTTP transport defaults: no HTTP/2
// and no upstream keep-alives.
func NewP0Transport(dialContext func(context.Context, string, string) (net.Conn, error)) *http.Transport {
	if dialContext == nil {
		dialContext = (&net.Dialer{}).DialContext
	}

	return &http.Transport{
		Proxy:             nil,
		DialContext:       dialContext,
		DisableKeepAlives: true,
		ForceAttemptHTTP2: false,
		TLSNextProto:      map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
}

// NewPooledTransport returns the P1 pooled transport defaults: HTTP/2 remains
// disabled, but HTTP/1.1 keep-alives are allowed for a bounded exact-key pool.
func NewPooledTransport(dialContext func(context.Context, string, string) (net.Conn, error), maxIdle int, idleTimeout time.Duration) *http.Transport {
	tr := NewP0Transport(dialContext)
	tr.DisableKeepAlives = false
	tr.MaxIdleConnsPerHost = maxIdle
	tr.IdleConnTimeout = idleTimeout

	return tr
}

// CloseIdleConnections closes any reusable upstream connections owned by this
// executor.
func (e *Executor) CloseIdleConnections() {
	if e.pool == nil {
		return
	}

	e.pool.closeIdleConnections()
}

// Execute performs one outbound request from a Control-resolved RequestStart
// and returns the e2c stream frames for the attempt.
//
// send, when non-nil, is called with the OutboundStartFrame before DNS/connect
// so Control can stamp the egress phase start in real time (docs/public/architecture.md
// step 19); a frame passed to send is excluded from the returned batch. With a
// nil send every frame, OutboundStart included, is returned in the batch.
func (e *Executor) Execute(ctx context.Context, start *strawpb.RequestStart, body []byte, attempt uint32, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	return e.ExecuteWithDeployment(ctx, "", start, body, attempt, send)
}

// ExecuteWithDeployment performs one outbound request with the deployment
// scope included in the optional upstream connection-pool key.
func (e *Executor) ExecuteWithDeployment(ctx context.Context, deploymentID string, start *strawpb.RequestStart, body []byte, attempt uint32, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	frames := newFrameBuilder(attempt)

	target, failure := parseTarget(start)
	if failure != nil {
		return []*strawpb.StreamFrame{frames.error(failure)}
	}

	failure = validateStart(start)
	if failure != nil {
		return []*strawpb.StreamFrame{frames.error(failure)}
	}

	executedProfile, failure := resolveFingerprintInstruction(start.GetFingerprintInstruction())
	if failure != nil {
		return []*strawpb.StreamFrame{frames.error(failure)}
	}

	failure = validateHostSuffixPolicy(target.host, start.GetDestinationPolicy())
	if failure != nil {
		return []*strawpb.StreamFrame{frames.error(failure)}
	}

	reqCtx, cancel := e.deadlineContext(ctx, start.GetDeadlineUnixMs())
	defer cancel()

	err := reqCtx.Err()
	if err != nil {
		return []*strawpb.StreamFrame{frames.error(timeoutFailure())}
	}

	emit := emitOrBatch(frames.outboundStart(target.host, target.port, executedProfile), send)

	if start.GetFingerprintInstruction() != "" {
		return e.executeProfiled(reqCtx, target, start, body, frames, emit, send)
	}

	resp, failure := e.doRequestWithRetry(reqCtx, deploymentID, target, start, body)
	if failure != nil {
		return append(emit, frames.error(failure))
	}

	defer func() { _ = resp.Body.Close() }()

	status, failure := responseStatus(resp.StatusCode)
	if failure != nil {
		return append(emit, frames.error(failure))
	}

	emit = emitOrAppend(emit, frames.responseStart(status, responseHeaders(resp.Header)), send)

	emit, failure = streamResponseBody(reqCtx, resp.Body, func() []*strawpb.Header {
		return responseHeaders(resp.Trailer)
	}, frames, emit, send)
	if failure != nil {
		return append(emit, frames.error(failure))
	}

	return append(emit, frames.end())
}

func (e *Executor) executeProfiled(ctx context.Context, target target, start *strawpb.RequestStart, body []byte, frames *frameBuilder, emit []*strawpb.StreamFrame, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	resp, closeResponse, failure := e.doProfiledRequest(ctx, target, start, body)
	if failure != nil {
		return append(emit, frames.error(failure))
	}
	defer closeResponse()

	status, failure := responseStatus(resp.statusCode)
	if failure != nil {
		return append(emit, frames.error(failure))
	}

	emit = emitOrAppend(emit, frames.responseStart(status, responseHeaders(resp.header)), send)

	emit, failure = streamResponseBody(ctx, resp.body, func() []*strawpb.Header {
		return responseHeaders(resp.trailer)
	}, frames, emit, send)
	if failure != nil {
		return append(emit, frames.error(failure))
	}

	return append(emit, frames.end())
}

func (e *Executor) downloadBodyRef(ctx context.Context, frame *strawpb.BodyRefFrame) ([]byte, *executionError) {
	resolver := sdkegress.HTTPBodyRefResolver{Client: e.bodyRefHTTPClient, Now: e.now}

	body, failure := resolver.DownloadBodyRef(ctx, frame)
	if failure != nil {
		return nil, executorFailure(failure.GetCode(), failure.GetDetails()[errorFactDetailKey])
	}

	return body, nil
}

func (e *Executor) doRequestWithRetry(ctx context.Context, deploymentID string, target target, start *strawpb.RequestStart, body []byte) (*http.Response, *executionError) {
	isReplayable := start.GetReplayable()

	for attemptIdx := range 2 {
		resp, retry, execErr := e.attemptRequest(ctx, deploymentID, target, start, body, attemptIdx, isReplayable)
		if execErr != nil {
			return nil, execErr
		}

		if !retry {
			return resp, nil
		}
	}

	return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, "max_attempts_exceeded")
}

func (e *Executor) attemptRequest(ctx context.Context, deploymentID string, target target, start *strawpb.RequestStart, body []byte, attemptIdx int, isReplayable bool) (*http.Response, bool, *executionError) {
	tr, client, pooled, key, hasKey := e.httpClient(ctx, deploymentID, target, start)

	req, buildFailure := buildHTTPRequest(ctx, start, body)
	if buildFailure != nil {
		if !pooled {
			tr.CloseIdleConnections()
		}

		return nil, false, buildFailure
	}

	resp, err := client.Do(req)
	if err != nil {
		retry, execErr := e.handleDoError(ctx, err, target.host, attemptIdx, isReplayable, pooled, tr, hasKey, key)

		return nil, retry, execErr
	}

	if !pooled {
		defer tr.CloseIdleConnections()
	}

	return resp, false, nil
}

func (e *Executor) handleDoError(ctx context.Context, err error, host string, attemptIdx int, isReplayable bool, pooled bool, tr *http.Transport, hasKey bool, key upstreamPoolKey) (bool, *executionError) {
	if hasKey && e.pool != nil {
		e.pool.evict(key)
	}

	execErr := mapHTTPError(ctx, err)

	if execErr.fact == "http_1_1_required" {
		e.cacheHTTP11Only(host)

		if isReplayable && attemptIdx == 0 {
			if !pooled {
				tr.CloseIdleConnections()
			}

			return true, nil
		}
	}

	if !pooled {
		tr.CloseIdleConnections()
	}

	return false, execErr
}

// openTunnel validates and opens one raw CONNECT upstream connection using
// the same destination-policy resolver/dialer path as decoded HTTP.
func (e *Executor) openTunnel(ctx context.Context, start *strawpb.RequestStart) (net.Conn, target, *executionError) {
	target, failure := parseTarget(start)
	if failure != nil {
		return nil, target, failure
	}

	failure = validateTunnelStart(start)
	if failure != nil {
		return nil, target, failure
	}

	failure = validateHostSuffixPolicy(target.host, start.GetDestinationPolicy())
	if failure != nil {
		return nil, target, failure
	}

	reqCtx, cancel := e.deadlineContext(ctx, start.GetDeadlineUnixMs())
	defer cancel()

	conn, err := e.dialValidated(reqCtx, "tcp", net.JoinHostPort(target.host, strconv.FormatUint(uint64(target.port), 10)), start.GetDestinationPolicy())
	if err != nil {
		return nil, target, mapHTTPError(reqCtx, err)
	}

	return conn, target, nil
}

func streamResponseBody(ctx context.Context, body io.Reader, trailers func() []*strawpb.Header, frames *frameBuilder, emit []*strawpb.StreamFrame, send func(*strawpb.StreamFrame)) ([]*strawpb.StreamFrame, *executionError) {
	buf := make([]byte, responseFrameDataBytes)
	offset := uint64(0)

	for {
		n, err := body.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			emit = emitOrAppend(emit, frames.data(offset, chunk), send)
			offset += uint64FromInt(n)
		}

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return emit, mapHTTPError(ctx, err)
		}
	}

	if trailers != nil {
		trailerHeaders := trailers()
		if len(trailerHeaders) > 0 {
			emit = emitOrAppend(emit, frames.trailers(trailerHeaders), send)
		}
	}

	return emit, nil
}

func (e *Executor) httpClient(ctx context.Context, deploymentID string, target target, start *strawpb.RequestStart) (*http.Transport, *http.Client, bool, upstreamPoolKey, bool) {
	if e.pool != nil && e.pool.enabled {
		key, failure := e.poolKey(ctx, deploymentID, target, start)
		if failure == nil {
			tr := e.pool.transport(key)

			return tr, &http.Client{
				Transport: tr,
				CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}, true, key, true
		}
	}

	tr := NewP0Transport(func(ctx context.Context, network, address string) (net.Conn, error) {
		return e.dialValidated(ctx, network, address, start.GetDestinationPolicy())
	})

	useHTTP2 := e.http2Enabled && schemeFromURL(start.GetUrl()) == schemeHTTPS && !e.isHTTP11Only(target.host)

	e.configureHTTP2(tr, useHTTP2, target.host, nil, func(ctx context.Context, network, address string) (net.Conn, error) {
		return e.dialValidated(ctx, network, address, start.GetDestinationPolicy())
	})

	return tr, &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, false, upstreamPoolKey{}, false
}

func (e *Executor) isHTTP11Only(host string) bool {
	if val, ok := e.http11Cache.Load(host); ok {
		if expire, ok := val.(time.Time); ok {
			if time.Now().Before(expire) {
				return true
			}
		}
	}

	return false
}

func (e *Executor) cacheHTTP11Only(host string) {
	ttl := defaultFallbackCacheTTL
	if e.fallbackCacheTTL > 0 {
		ttl = e.fallbackCacheTTL
	}

	e.http11Cache.Store(host, time.Now().Add(ttl))
}

func (e *Executor) configureHTTP2(tr *http.Transport, useHTTP2 bool, host string, onFallback func(), dialContext func(context.Context, string, string) (net.Conn, error)) {
	e.setupTLSClientConfig(tr, host)

	if !useHTTP2 {
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}

		return
	}

	tr.ForceAttemptHTTP2 = true
	tr.TLSNextProto = nil
	tr.DialTLSContext = e.makeDialTLSContext(host, onFallback, dialContext)
}

func (e *Executor) setupTLSClientConfig(tr *http.Transport, host string) {
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{
			ServerName: host,
			RootCAs:    e.rootCAs,
		}

		if e.insecureSkipVerify {
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
	} else {
		tr.TLSClientConfig.ServerName = host
		tr.TLSClientConfig.RootCAs = e.rootCAs

		if e.insecureSkipVerify {
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
	}
}

func (e *Executor) makeDialTLSContext(host string, onFallback func(), dialContext func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		rawConn, err := dialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		tlsConfig := &tls.Config{
			ServerName: host,
			NextProtos: []string{"h2", "http/1.1"},
			RootCAs:    e.rootCAs,
		}

		if e.insecureSkipVerify {
			tlsConfig.InsecureSkipVerify = true
		}

		tlsConn := tls.Client(rawConn, tlsConfig)

		err = tlsConn.HandshakeContext(ctx)
		if err != nil {
			_ = rawConn.Close()

			return nil, fmt.Errorf("tls handshake failed: %w", err)
		}

		negotiated := tlsConn.ConnectionState().NegotiatedProtocol

		if negotiated == "http/1.1" || negotiated == "" {
			e.cacheHTTP11Only(host)

			if onFallback != nil {
				onFallback()
			}
		}

		return tlsConn, nil
	}
}

func (e *Executor) poolKey(ctx context.Context, deploymentID string, target target, start *strawpb.RequestStart) (upstreamPoolKey, *executionError) {
	ips, failure := e.validatedIPs(ctx, target.host, start.GetDestinationPolicy())
	if failure != nil {
		return upstreamPoolKey{}, failure
	}

	useHTTP2 := e.http2Enabled && schemeFromURL(start.GetUrl()) == schemeHTTPS && !e.isHTTP11Only(target.host)

	key := upstreamPoolKey{
		deploymentID:       deploymentID,
		resolutionMode:     start.GetDestinationPolicy().GetResolutionMode(),
		scheme:             schemeFromURL(start.GetUrl()),
		host:               target.host,
		port:               target.port,
		dialIP:             ips[0].String(),
		fingerprintProfile: start.GetFingerprintInstruction(),
		useHTTP2:           useHTTP2,
	}

	if e.pool != nil {
		e.pool.discardStale(key, ips)
	}

	return key, nil
}

func (e *Executor) deadlineContext(ctx context.Context, deadlineUnixMs int64) (context.Context, context.CancelFunc) {
	if deadlineUnixMs <= 0 {
		return context.WithCancel(ctx)
	}

	deadline := time.UnixMilli(deadlineUnixMs)
	if existing, ok := ctx.Deadline(); ok && existing.Before(deadline) {
		return context.WithCancel(ctx)
	}

	return context.WithDeadline(ctx, deadline)
}

func (e *Executor) dialValidated(ctx context.Context, network, address string, policy *strawpb.DestinationPolicy) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	ips, failure := e.validatedIPs(ctx, host, policy)
	if failure != nil {
		return nil, failure
	}

	return e.dialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

func (e *Executor) validatedIPs(ctx context.Context, host string, policy *strawpb.DestinationPolicy) ([]netip.Addr, *executionError) {
	ips, err := e.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, timeoutFailure()
		}

		return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE, dnsNoRecordsFact)
	}

	out := make([]netip.Addr, 0, len(ips))

	for _, ip := range ips {
		addr, ok := netIPAddr(ip)
		if !ok {
			return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE, dnsNoRecordsFact)
		}

		failure := validateResolvedIP(policy, addr)
		if failure != nil {
			return nil, failure
		}

		out = append(out, addr)
	}

	failure := validateCNAMESuffixPolicy(ctx, e.resolver, host, policy)
	if failure != nil {
		return nil, failure
	}

	return out, nil
}

type target struct {
	host string
	port uint32
}

func parseTarget(start *strawpb.RequestStart) (target, *executionError) {
	if start == nil {
		return target{}, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	parsed, failure := parseRequestURL(start.GetUrl())
	if failure != nil {
		return target{}, failure
	}

	port, failure := targetPort(parsed)
	if failure != nil {
		return target{}, failure
	}

	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")

	return target{host: host, port: port}, nil
}

func parseRequestURL(raw string) (*url.URL, *executionError) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return parsed, nil
}

func targetPort(parsed *url.URL) (uint32, *executionError) {
	port := parsed.Port()

	if port == "" {
		port = defaultPort(parsed.Scheme)
	}

	if port == "" {
		return 0, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	n, err := strconv.ParseUint(port, 10, 32)
	if err != nil || n == 0 || n > 65535 {
		return 0, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return uint32(n), nil
}

func defaultPort(scheme string) string {
	switch scheme {
	case "http":
		return defaultHTTPPort
	case "https":
		return defaultHTTPSPort
	default:
		return ""
	}
}

func schemeFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return parsed.Scheme
}

type upstreamPoolKey struct {
	deploymentID       string
	resolutionMode     strawpb.DestinationResolutionMode
	scheme             string
	host               string
	port               uint32
	dialIP             string
	fingerprintProfile string
	useHTTP2           bool
}

type upstreamConnectionPool struct {
	enabled     bool
	dialContext func(context.Context, string, string) (net.Conn, error)
	maxIdle     int
	idleTimeout time.Duration
	maxLifetime time.Duration
	mu          sync.Mutex
	transports  map[upstreamPoolKey]pooledTransport
	executor    *Executor
}

type pooledTransport struct {
	createdAt time.Time
	tr        *http.Transport
}

func newUpstreamConnectionPool(opts UpstreamConnectionPoolOptions, dialContext func(context.Context, string, string) (net.Conn, error), executor *Executor) *upstreamConnectionPool {
	if !opts.Enabled {
		return nil
	}

	if opts.MaxIdleConnsPerHost <= 0 {
		opts.MaxIdleConnsPerHost = 2
	}

	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = defaultPoolIdleTimeout
	}

	if opts.MaxLifetime <= 0 {
		opts.MaxLifetime = defaultPoolMaxLifetime
	}

	return &upstreamConnectionPool{
		enabled:     true,
		dialContext: dialContext,
		maxIdle:     opts.MaxIdleConnsPerHost,
		idleTimeout: opts.IdleTimeout,
		maxLifetime: opts.MaxLifetime,
		transports:  map[upstreamPoolKey]pooledTransport{},
		executor:    executor,
	}
}

func (p *upstreamConnectionPool) transport(key upstreamPoolKey) *http.Transport {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if pooled, ok := p.transports[key]; ok {
		if now.Sub(pooled.createdAt) < p.maxLifetime {
			return pooled.tr
		}

		pooled.tr.CloseIdleConnections()
	}

	tr := NewPooledTransport(func(ctx context.Context, network, _ string) (net.Conn, error) {
		return p.dialContext(ctx, network, net.JoinHostPort(key.dialIP, strconv.FormatUint(uint64(key.port), 10)))
	}, p.maxIdle, p.idleTimeout)

	p.executor.configureHTTP2(tr, key.useHTTP2, key.host, func() {
		p.evict(key)
	}, func(ctx context.Context, network, _ string) (net.Conn, error) {
		return p.dialContext(ctx, network, net.JoinHostPort(key.dialIP, strconv.FormatUint(uint64(key.port), 10)))
	})

	p.transports[key] = pooledTransport{createdAt: now, tr: tr}

	return tr
}

func (p *upstreamConnectionPool) evict(key upstreamPoolKey) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if pooled, ok := p.transports[key]; ok {
		pooled.tr.CloseIdleConnections()
		delete(p.transports, key)
	}
}

func (p *upstreamConnectionPool) discardStale(current upstreamPoolKey, validIPs []netip.Addr) {
	p.mu.Lock()
	defer p.mu.Unlock()

	valid := make(map[string]struct{}, len(validIPs))
	for _, ip := range validIPs {
		valid[ip.String()] = struct{}{}
	}

	for key, pooled := range p.transports {
		if !key.samePoolExceptIP(current) {
			continue
		}

		if _, ok := valid[key.dialIP]; ok {
			continue
		}

		pooled.tr.CloseIdleConnections()
		delete(p.transports, key)
	}
}

func (p *upstreamConnectionPool) closeIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, pooled := range p.transports {
		pooled.tr.CloseIdleConnections()
		delete(p.transports, key)
	}
}

func (k upstreamPoolKey) samePoolExceptIP(other upstreamPoolKey) bool {
	return k.deploymentID == other.deploymentID &&
		k.resolutionMode == other.resolutionMode &&
		k.scheme == other.scheme &&
		k.host == other.host &&
		k.port == other.port &&
		k.fingerprintProfile == other.fingerprintProfile &&
		k.useHTTP2 == other.useHTTP2
}

func validateStart(start *strawpb.RequestStart) *executionError {
	err := start.Validate()
	if err != nil {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetMode() != strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetRedirectPolicy() != strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetDestinationPolicy().GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, unsupportedModeFact)
	}

	if strings.EqualFold(start.GetMethod(), http.MethodConnect) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return nil
}

func validateTunnelStart(start *strawpb.RequestStart) *executionError {
	err := start.Validate()
	if err != nil {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetMode() != strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if !strings.EqualFold(start.GetMethod(), http.MethodConnect) {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetRedirectPolicy() != strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	if start.GetDestinationPolicy().GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED, unsupportedModeFact)
	}

	if start.GetFingerprintInstruction() != "" {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT, unsupportedFingerprint)
	}

	if len(start.GetInjectionOperations()) != 0 || len(start.GetHeaders()) != 0 {
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	return nil
}

func buildHTTPRequest(ctx context.Context, start *strawpb.RequestStart, body []byte) (*http.Request, *executionError) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, start.GetMethod(), start.GetUrl(), reader)
	if err != nil {
		return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	for _, h := range start.GetHeaders() {
		if !safeOutboundHeader(h.GetName(), h.GetValue()) {
			return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}

		req.Header.Add(h.GetName(), string(h.GetValue()))
	}

	failure := applyInjection(req.Header, start.GetInjectionOperations())
	if failure != nil {
		return nil, failure
	}

	return req, nil
}

func applyInjection(headers http.Header, ops []*strawpb.InjectionOperation) *executionError {
	seenSet := map[string]struct{}{}

	for _, op := range ops {
		name := op.GetHeaderName()
		if !safeOutboundHeader(name, op.GetValue()) {
			return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}

		key := strings.ToLower(name)

		switch op.GetOp() {
		case opSet:
			if _, ok := seenSet[key]; ok {
				return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
			}

			seenSet[key] = struct{}{}

			headers.Set(name, string(op.GetValue()))
		case opAppend:
			headers.Add(name, string(op.GetValue()))
		case opRemove:
			headers.Del(name)
		default:
			return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}
	}

	return nil
}

func safeOutboundHeader(name string, value []byte) bool {
	if !validHTTPToken(name) {
		return false
	}

	if deniedHeader(name) {
		return false
	}

	return !bytes.ContainsAny(value, "\r\n")
}

func deniedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "content-length", "transfer-encoding", "connection", "proxy-authorization":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(name), "x-straw-")
	}
}

func validHTTPToken(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		idx := int(r)
		if idx > 255 || !httpTokenAllowed[idx] {
			return false
		}
	}

	return true
}

var httpTokenAllowed = func() [256]bool {
	var allowed [256]bool

	for _, r := range "!#$%&'*+-.^_`|~" {
		allowed[byte(r)] = true
	}

	for r := byte('0'); r <= '9'; r++ {
		allowed[r] = true
	}

	for r := byte('A'); r <= 'Z'; r++ {
		allowed[r] = true
	}

	for r := byte('a'); r <= 'z'; r++ {
		allowed[r] = true
	}

	return allowed
}()

func responseHeaders(headers http.Header) []*strawpb.Header {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]*strawpb.Header, 0, len(headers))
	for _, name := range names {
		for _, value := range headers.Values(name) {
			out = append(out, &strawpb.Header{Name: name, Value: []byte(value)})
		}
	}

	return out
}

type executionError struct {
	code        strawpb.ErrorCode
	fact        string
	timeoutType strawpb.TimeoutType
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
	var profiledStreamErr fhttphttp2.StreamError
	if errors.As(err, &profiledStreamErr) {
		return mapProfiledHTTP2StreamError(profiledStreamErr.Code), true
	}

	var profiledStreamErrPtr *fhttphttp2.StreamError
	if errors.As(err, &profiledStreamErrPtr) && profiledStreamErrPtr != nil {
		return mapProfiledHTTP2StreamError(profiledStreamErrPtr.Code), true
	}

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

func mapProfiledHTTP2StreamError(code fhttphttp2.ErrCode) *executionError {
	switch code {
	case fhttphttp2.ErrCodeConnect:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED, "upstream_connection_refused")
	case fhttphttp2.ErrCodeInadequateSecurity:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE, "upstream_tls_failure")
	case fhttphttp2.ErrCodeEnhanceYourCalm:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "enhance_your_calm")
	case fhttphttp2.ErrCodeHTTP11Required:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "http_1_1_required")
	case fhttphttp2.ErrCodeNo,
		fhttphttp2.ErrCodeProtocol,
		fhttphttp2.ErrCodeInternal,
		fhttphttp2.ErrCodeFlowControl,
		fhttphttp2.ErrCodeSettingsTimeout,
		fhttphttp2.ErrCodeStreamClosed,
		fhttphttp2.ErrCodeFrameSize,
		fhttphttp2.ErrCodeRefusedStream,
		fhttphttp2.ErrCodeCancel,
		fhttphttp2.ErrCodeCompression:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "upstream_reset")
	default:
		return executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET, "upstream_reset")
	}
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

type frameBuilder struct {
	attempt uint32
	seq     uint64
}

func newFrameBuilder(attempt uint32) *frameBuilder {
	return &frameBuilder{attempt: attempt}
}

func (b *frameBuilder) outboundStart(host string, port uint32, executedProfile string) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload: &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{
			TargetHost:                 host,
			TargetPort:                 port,
			Attempt:                    b.attempt,
			ExecutedFingerprintProfile: executedProfile,
			WorkerTimestampMs:          time.Now().UnixMilli(),
		}},
	}
}

// emitOrBatch delivers frame through send when one is provided, otherwise
// returns it as the start of the batched frame slice.
func emitOrBatch(frame *strawpb.StreamFrame, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	if send != nil {
		send(frame)

		return nil
	}

	return []*strawpb.StreamFrame{frame}
}

func emitOrAppend(frames []*strawpb.StreamFrame, frame *strawpb.StreamFrame, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	if send != nil {
		send(frame)

		return frames
	}

	return append(frames, frame)
}

func responseStatus(status int) (uint32, *executionError) {
	if status < 0 || status > 999 {
		return 0, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, executorInternalFact)
	}

	return uint32(status), nil
}

func (b *frameBuilder) responseStart(status uint32, headers []*strawpb.Header) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{
			Status:  status,
			Headers: headers,
		}},
	}
}

func (b *frameBuilder) data(offset uint64, data []byte) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: offset, Data: data}},
	}
}

func (b *frameBuilder) trailers(headers []*strawpb.Header) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_Trailers{Trailers: &strawpb.TrailersFrame{Headers: headers}},
	}
}

func (b *frameBuilder) end() *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}},
	}
}

func (b *frameBuilder) error(failure *executionError) *strawpb.StreamFrame {
	errFrame := &strawpb.ErrorFrame{
		Code:    failure.code,
		Details: map[string]string{errorFactDetailKey: failure.fact},
	}
	if failure.timeoutType != strawpb.TimeoutType_TIMEOUT_TYPE_UNSPECIFIED {
		errFrame.TimeoutType = &failure.timeoutType
	}

	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_Error{Error: errFrame},
	}
}

func uint64FromInt(v int) uint64 {
	if v <= 0 {
		return 0
	}

	out, err := strconv.ParseUint(strconv.Itoa(v), 10, 64)
	if err != nil {
		return 0
	}

	return out
}
