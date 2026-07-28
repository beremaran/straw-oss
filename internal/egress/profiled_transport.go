package egress

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	utls "github.com/bogdanfinn/utls"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type profiledResponse struct {
	statusCode int
	header     http.Header
	trailer    http.Header
	body       io.ReadCloser
}

type profiledRequest struct {
	request     *http.Request
	headerOrder []string
}

type profiledTLSConfig struct {
	rootCAs            *x509.CertPool
	insecureSkipVerify bool
	sessionCache       utls.ClientSessionCache
}

var errProfiledDialTargetRejected = errors.New("profiled dial target rejected")

// doProfiledRequest executes one named profile request. Resolution is done by
// Straw before the profile dial hook is installed; the hook only accepts
// the original host/port and always dials the selected validated IP.
func (e *Executor) doProfiledRequest(ctx context.Context, target target, start *strawpb.RequestStart, body []byte) (profiledResponse, func(), *executionError) {
	profile, ok := executableFingerprintProfiles[start.GetFingerprintInstruction()]
	if !ok {
		return profiledResponse{}, func() {}, executorFailure(strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT, unsupportedFingerprint)
	}

	var dial func(context.Context, string, string) (net.Conn, error)
	if start.GetDestinationPolicy().GetResolutionMode() == strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE {
		dial = e.profiledProxyDialContext(ctx, target, start.GetUpstreamProxy())
	} else {
		ips, failure := e.validatedIPs(ctx, target.host, start.GetDestinationPolicy())
		if failure != nil {
			return profiledResponse{}, func() {}, failure
		}

		if len(ips) == 0 {
			return profiledResponse{}, func() {}, executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE, dnsNoRecordsFact)
		}

		dial = e.profiledDialContext(ctx, target, ips[0])
	}

	req, requestFailure := buildProfiledRequest(ctx, start, body)
	if requestFailure != nil {
		return profiledResponse{}, func() {}, requestFailure
	}

	resp, conn, err := doProfiledRoundTrip(ctx, dial, target, profile, profiledTLSConfig{
		rootCAs:            e.rootCAs,
		insecureSkipVerify: e.insecureSkipVerify,
		sessionCache:       e.profileSessionCaches[start.GetFingerprintInstruction()],
	}, req)
	if err != nil {
		return profiledResponse{}, func() {}, mapHTTPError(ctx, err)
	}

	keepBodyOpen := false
	defer func() {
		if !keepBodyOpen {
			_ = resp.Body.Close()
			_ = conn.Close()
		}
	}()

	closeResponse := func() {
		_ = resp.Body.Close()
		_ = conn.Close()
	}

	result := profiledResponse{
		statusCode: resp.StatusCode,
		header:     resp.Header,
		trailer:    resp.Trailer,
		body:       resp.Body,
	}
	keepBodyOpen = true

	return result, closeResponse, nil
}

func (e *Executor) profiledProxyDialContext(requestCtx context.Context, target target, instruction *strawpb.UpstreamProxyInstruction) func(context.Context, string, string) (net.Conn, error) {
	return func(libraryCtx context.Context, _, address string) (net.Conn, error) {
		if !proxyDialTargetMatches(address, target) {
			return nil, upstreamProxyInstructionFailure()
		}

		ctx, cancel := joinedDialContext(requestCtx, libraryCtx)
		defer cancel()

		conn, failure := e.upstreamProxy.Open(ctx, instruction, target.host, target.port)
		if failure != nil {
			return nil, failure
		}

		return conn, nil
	}
}

func (e *Executor) profiledDialContext(requestCtx context.Context, target target, validatedIP netip.Addr) func(context.Context, string, string) (net.Conn, error) {
	return func(libraryCtx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || strings.TrimSuffix(strings.ToLower(host), ".") != target.host || port != strconv.FormatUint(uint64(target.port), 10) {
			return nil, errProfiledDialTargetRejected
		}

		ctx, cancel := joinedDialContext(requestCtx, libraryCtx)
		defer cancel()

		return e.dialContext(ctx, network, net.JoinHostPort(validatedIP.String(), port))
	}
}

func joinedDialContext(requestCtx, libraryCtx context.Context) (context.Context, context.CancelFunc) {
	if libraryCtx == nil {
		return context.WithCancel(requestCtx)
	}

	ctx, cancel := context.WithCancel(requestCtx)

	go func() {
		select {
		case <-libraryCtx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func buildProfiledRequest(ctx context.Context, start *strawpb.RequestStart, body []byte) (profiledRequest, *executionError) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, start.GetMethod(), start.GetUrl(), reader)
	if err != nil {
		return profiledRequest{}, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	req.Close = true
	req.Header = make(http.Header)

	order := make([]string, 0, len(start.GetHeaders())+len(start.GetInjectionOperations()))

	for _, header := range start.GetHeaders() {
		if !safeOutboundHeader(header.GetName(), header.GetValue()) {
			return profiledRequest{}, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}

		req.Header.Add(header.GetName(), string(header.GetValue()))
		appendProfiledHeaderOrder(&order, header.GetName())
	}

	failure := applyProfiledInjection(req.Header, &order, start.GetInjectionOperations())
	if failure != nil {
		return profiledRequest{}, failure
	}

	return profiledRequest{request: req, headerOrder: order}, nil
}

func applyProfiledInjection(headers http.Header, order *[]string, ops []*strawpb.InjectionOperation) *executionError {
	seenSet := make(map[string]struct{})

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
			appendProfiledHeaderOrder(order, name)
		case opAppend:
			headers.Add(name, string(op.GetValue()))
			appendProfiledHeaderOrder(order, name)
		case opRemove:
			headers.Del(name)
			removeProfiledHeaderOrder(order, key)
		default:
			return executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}
	}

	return nil
}

func appendProfiledHeaderOrder(order *[]string, name string) {
	name = strings.ToLower(name)
	if slices.Contains(*order, name) {
		return
	}

	*order = append(*order, name)
}

func removeProfiledHeaderOrder(order *[]string, name string) {
	filtered := (*order)[:0]
	for _, existing := range *order {
		if existing != name {
			filtered = append(filtered, existing)
		}
	}

	*order = filtered
}
