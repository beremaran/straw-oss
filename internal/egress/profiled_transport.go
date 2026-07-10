package egress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

type profiledResponse struct {
	statusCode int
	header     http.Header
	trailer    http.Header
	body       io.ReadCloser
}

var errProfiledDialTargetRejected = errors.New("profiled dial target rejected")

// doProfiledRequest executes one named profile request. Resolution is done by
// Straw before the tls-client dial hook is installed; the hook only accepts
// the original host/port and always dials the selected validated IP.
func (e *Executor) doProfiledRequest(ctx context.Context, target target, start *strawpb.RequestStart, body []byte) (profiledResponse, func(), *executionError) {
	ips, failure := e.validatedIPs(ctx, target.host, start.GetDestinationPolicy())
	if failure != nil {
		return profiledResponse{}, func() {}, failure
	}

	if len(ips) == 0 {
		return profiledResponse{}, func() {}, executorFailure(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE, dnsNoRecordsFact)
	}

	profile, ok := executableFingerprintProfiles[start.GetFingerprintInstruction()]
	if !ok {
		return profiledResponse{}, func() {}, executorFailure(strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT, unsupportedFingerprint)
	}

	dial := e.profiledDialContext(ctx, target, ips[0])
	client, err := e.newProfiledClient(profile, dial)
	//nolint:wsl_v5
	if err != nil {
		return profiledResponse{}, func() {}, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, executorInternalFact)
	}

	closeClient := func() { client.CloseIdleConnections() }
	req, requestFailure := buildProfiledRequest(ctx, start, body)
	//nolint:wsl_v5
	if requestFailure != nil {
		closeClient()

		return profiledResponse{}, func() {}, requestFailure
	}

	resp, err := client.Do(req)
	if err != nil {
		//nolint:wsl_v5
		closeClient()

		return profiledResponse{}, func() {}, mapHTTPError(ctx, err)
	}

	closeResponse := func() {
		_ = resp.Body.Close()

		closeClient()
	}

	return profiledResponse{
		statusCode: resp.StatusCode,
		header:     http.Header(resp.Header),
		trailer:    http.Header(resp.Trailer),
		body:       resp.Body,
	}, closeResponse, nil
}

func (e *Executor) newProfiledClient(profile profiles.ClientProfile, dial func(context.Context, string, string) (net.Conn, error)) (tlsclient.HttpClient, error) {
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(profile),
		tlsclient.WithRandomTLSExtensionOrder(),
		tlsclient.WithTimeoutSeconds(0),
		tlsclient.WithNotFollowRedirects(),
		tlsclient.WithDisableHttp3(),
		tlsclient.WithDialContext(dial),
		tlsclient.WithTransportOptions(&tlsclient.TransportOptions{
			RootCAs:            e.rootCAs,
			DisableKeepAlives:  true,
			DisableCompression: true,
		}),
	}
	if e.insecureSkipVerify {
		options = append(options, tlsclient.WithInsecureSkipVerify())
	}

	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("create profiled client: %w", err)
	}

	return client, nil
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

func buildProfiledRequest(ctx context.Context, start *strawpb.RequestStart, body []byte) (*fhttp.Request, *executionError) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := fhttp.NewRequestWithContext(ctx, start.GetMethod(), start.GetUrl(), reader)
	if err != nil {
		return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, invalidRequestStartFact)
	}

	req.Close = true
	req.Header = make(fhttp.Header)

	order := make([]string, 0, len(start.GetHeaders())+len(start.GetInjectionOperations()))

	for _, header := range start.GetHeaders() {
		if !safeOutboundHeader(header.GetName(), header.GetValue()) {
			return nil, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, injectionFactFailed)
		}

		req.Header.Add(header.GetName(), string(header.GetValue()))
		appendProfiledHeaderOrder(&order, header.GetName())
	}

	failure := applyProfiledInjection(req.Header, &order, start.GetInjectionOperations())
	if failure != nil {
		return nil, failure
	}

	req.Header[fhttp.HeaderOrderKey] = order

	return req, nil
}

func applyProfiledInjection(headers fhttp.Header, order *[]string, ops []*strawpb.InjectionOperation) *executionError {
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
