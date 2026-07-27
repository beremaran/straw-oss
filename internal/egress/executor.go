package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	utls "github.com/bogdanfinn/utls"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
	sdkegress "github.com/beremaran/straw-sdk-go/egress"
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
	Metrics            *Metrics
}

// Executor performs P0 decoded HTTP/HTTPS outbound execution.
type Executor struct {
	resolver             Resolver
	dialContext          func(context.Context, string, string) (net.Conn, error)
	pool                 *upstreamConnectionPool
	http2Enabled         bool
	fallbackCacheTTL     time.Duration
	http11Cache          sync.Map
	rootCAs              *x509.CertPool
	insecureSkipVerify   bool
	bodyRefHTTPClient    *http.Client
	profileSessionCaches map[string]utls.ClientSessionCache
	now                  func() time.Time
	metrics              *Metrics
}

// UpstreamConnectionPoolOptions configures optional direct-local HTTP
// connection reuse. Zero values keep pooling disabled.
type UpstreamConnectionPoolOptions struct {
	Enabled             bool
	MaxIdleConnsPerHost int
	IdleTimeout         time.Duration
	MaxLifetime         time.Duration
}

// NewExecutor constructs the official HTTP/HTTPS request executor.
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
		resolver:             resolver,
		dialContext:          dialContext,
		http2Enabled:         opts.HTTP2Enabled,
		fallbackCacheTTL:     opts.FallbackCacheTTL,
		rootCAs:              opts.RootCAs,
		insecureSkipVerify:   opts.InsecureSkipVerify,
		bodyRefHTTPClient:    opts.BodyRefHTTPClient,
		profileSessionCaches: newProfileSessionCaches(),
		now:                  opts.Now,
		metrics:              opts.Metrics,
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
//
// Instrumentation lives in this wrapper rather than inside the execution body
// because the body has eight failure returns; classifying the terminal frame
// once here keeps every one of them counted without a metrics call per return.
// Request bytes are the body handed to the executor, so they are counted for
// an attempt that failed before reaching the wire too.
func (e *Executor) ExecuteWithDeployment(ctx context.Context, deploymentID string, start *strawpb.RequestStart, body []byte, attempt uint32, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	started := e.now()
	frames := e.executeWithDeployment(ctx, deploymentID, start, body, attempt, send)

	e.metrics.AddRequestBytes(uint64FromInt(len(body)))
	e.metrics.ObserveRequest(terminalErrorCode(frames), e.now().Sub(started))

	return frames
}

func (e *Executor) executeWithDeployment(ctx context.Context, deploymentID string, start *strawpb.RequestStart, body []byte, attempt uint32, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
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

	emit, failure = e.streamResponseBody(reqCtx, resp.Body, func() []*strawpb.Header {
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

	emit, failure = e.streamResponseBody(ctx, resp.body, func() []*strawpb.Header {
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

func (e *Executor) streamResponseBody(ctx context.Context, body io.Reader, trailers func() []*strawpb.Header, frames *frameBuilder, emit []*strawpb.StreamFrame, send func(*strawpb.StreamFrame)) ([]*strawpb.StreamFrame, *executionError) {
	buf := make([]byte, responseFrameDataBytes)
	offset := uint64(0)

	for {
		n, err := body.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			emit = emitOrAppend(emit, frames.data(offset, chunk), send)
			offset += uint64FromInt(n)

			// Counted per chunk so an aborted download still reports what it
			// managed to pull down before the failure.
			e.metrics.AddResponseBytes(uint64FromInt(n))
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
