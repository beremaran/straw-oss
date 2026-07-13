package control

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ProxyHandler accepts ordinary absolute-form HTTP proxy requests and
// authority-form CONNECT requests on Control's API listener.
type ProxyHandler struct {
	maxRequestBodyBytes uint64
	authenticator       *Authenticator
	rawDispatcher       RawResponseDispatcher
	tunnelDispatcher    TunnelDispatcher
}

// NewProxyHandler builds Control's forward-proxy ingress.
func NewProxyHandler(maxRequestBodyBytes uint64, auth *Authenticator, raw RawResponseDispatcher, tunnels TunnelDispatcher) *ProxyHandler {
	return &ProxyHandler{
		maxRequestBodyBytes: maxRequestBodyBytes,
		authenticator:       auth,
		rawDispatcher:       raw,
		tunnelDispatcher:    tunnels,
	}
}

// Wrap selects proxy ingress by request-target form before an API mux can
// interpret the destination path as a Control API route.
func (h *ProxyHandler) Wrap(api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect || (r.URL != nil && r.URL.IsAbs()) {
			h.ServeHTTP(w, r)

			return
		}

		api.ServeHTTP(w, r)
	})
}

// ServeHTTP dispatches forward-proxy traffic and leaves origin-form requests
// outside the public API as ordinary 404 responses.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		h.serveConnect(w, r)

		return
	}

	if r.URL == nil || !r.URL.IsAbs() {
		http.NotFound(w, r)

		return
	}

	h.serveHTTP(w, r)
}

func (h *ProxyHandler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()

	identity, err := h.authenticate(r)
	if err != nil {
		writeProxyAuthFailure(w, requestID)

		return
	}

	validated, validationErr := h.validateHTTPProxyRequest(r)
	if validationErr != nil {
		WriteValidationError(w, requestID, validationErr)

		return
	}

	if h.rawDispatcher == nil {
		writePipelineError(w, requestID, &PipelineError{Code: ControlInternalError})

		return
	}

	_, dispatchErr, wroteHeader := h.rawDispatcher.DispatchRaw(r.Context(), DispatchInput{
		RequestID: requestID,
		Identity:  identity,
		Request:   validated,
	}, w)
	if dispatchErr != nil && !wroteHeader {
		writePipelineError(w, requestID, dispatchErr)
	}
}

func (h *ProxyHandler) serveConnect(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()

	identity, err := h.authenticate(r)
	if err != nil {
		writeProxyAuthFailure(w, requestID)

		return
	}

	validated, validationErr := h.validateConnectRequest(r)
	if validationErr != nil {
		WriteValidationError(w, requestID, validationErr)

		return
	}

	if h.tunnelDispatcher == nil {
		writePipelineError(w, requestID, &PipelineError{Code: ControlInternalError})

		return
	}

	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		WriteError(w, http.StatusHTTPVersionNotSupported, ErrorResponseFromCode(UnsupportedIngressMode, requestID, nil))

		return
	}
	defer func() { _ = conn.Close() }()

	client := &bufferedTunnelClient{rw: rw}

	response, dispatchErr := h.tunnelDispatcher.DispatchTunnel(r.Context(), DispatchInput{
		RequestID: requestID,
		Identity:  identity,
		Request:   validated,
	}, client)
	if dispatchErr != nil && response.Status != http.StatusOK {
		writeHijackedPipelineError(client, requestID, dispatchErr)
	}
}

func (h *ProxyHandler) authenticate(r *http.Request) (Identity, error) {
	if h.authenticator == nil {
		return Identity{}, ErrAuthFailure
	}

	return h.authenticator.Authenticate(r.Context(), r.Header.Get("Proxy-Authorization"))
}

func (h *ProxyHandler) validateHTTPProxyRequest(r *http.Request) (*ValidatedRequest, *ValidationError) {
	err := validateMethod(r.Method)
	if err != nil {
		return nil, asValidationError(err)
	}

	parsedURL, err := validateURL(r.URL.String())
	if err != nil {
		return nil, asValidationError(err)
	}

	headers, err := proxyRequestHeaders(r.Header)
	if err != nil {
		return nil, asValidationError(err)
	}

	body, bodyErr := readBoundedProxyBody(r.Body, h.maxRequestBodyBytes)
	if bodyErr != nil {
		return nil, bodyErr
	}

	return &ValidatedRequest{
		Method:          r.Method,
		URL:             parsedURL,
		Headers:         headers,
		BodyData:        body,
		BodySizeBytes:   int64(len(body)),
		Routing:         RoutingHints{},
		IngressType:     IngressTypeHTTPProxy,
		TimeoutMs:       0,
		Replayable:      false,
		CaptureDecision: defaultPayloadCaptureDecision,
	}, nil
}

func (h *ProxyHandler) validateConnectRequest(r *http.Request) (*ValidatedRequest, *ValidationError) {
	_, err := proxyRequestHeaders(r.Header)
	if err != nil {
		return nil, asValidationError(err)
	}

	authority := r.Host
	if authority == "" && r.URL != nil && r.URL.Host != "" {
		authority = r.URL.Host
	}

	host, portText, err := net.SplitHostPort(authority)
	if err != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "CONNECT target must be host:port"}
	}

	host, err = normalizeHostname(host)
	if err != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "CONNECT target host is not valid"}
	}

	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "CONNECT target port must be between 1 and 65535"}
	}

	target := &url.URL{Scheme: "connect", Host: net.JoinHostPort(host, strconv.FormatUint(port, 10))}

	return &ValidatedRequest{
		Method:          http.MethodConnect,
		URL:             target,
		BodySizeBytes:   -1,
		Routing:         RoutingHints{},
		IngressType:     IngressTypeConnect,
		TimeoutMs:       0,
		Replayable:      false,
		CaptureDecision: defaultPayloadCaptureDecision,
	}, nil
}

func proxyRequestHeaders(header http.Header) ([]HeaderPair, error) {
	connectionHeaders := map[string]struct{}{}

	for _, value := range header.Values("Connection") {
		for name := range strings.SplitSeq(value, ",") {
			connectionHeaders[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
		}
	}

	keys := make([]string, 0, len(header))
	for name := range header {
		keys = append(keys, name)
	}

	sort.Strings(keys)

	out := make([]HeaderPair, 0, len(header))

	for _, name := range keys {
		if proxyHopByHopHeader(name, connectionHeaders) {
			continue
		}

		for _, value := range header.Values(name) {
			out = append(out, HeaderPair{Name: name, Value: base64.StdEncoding.EncodeToString([]byte(value))})
		}
	}

	return validateHeaders(out)
}

func proxyHopByHopHeader(name string, connectionHeaders map[string]struct{}) bool {
	lower := strings.ToLower(name)
	if _, ok := connectionHeaders[lower]; ok {
		return true
	}

	switch lower {
	case "connection", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func readBoundedProxyBody(body io.ReadCloser, maxBytes uint64) ([]byte, *ValidationError) {
	if body == nil {
		return nil, nil
	}
	defer func() { _ = body.Close() }()

	limit, limitErr := strconv.ParseInt(strconv.FormatUint(maxBytes, 10), 10, 64)
	if limitErr != nil {
		limit = math.MaxInt64
	} else if limit < math.MaxInt64 {
		limit++
	}

	data, err := io.ReadAll(io.LimitReader(body, limit))
	if err != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "request body could not be read"}
	}

	if uint64(len(data)) > maxBytes {
		return nil, &ValidationError{Code: errorCodeBodyTooLarge, Message: requestBodyExceedsLimit, Details: map[string]string{
			errorDetailDirectionKey:  errorDetailDirectionRequest,
			errorDetailLimitBytesKey: strconv.FormatUint(maxBytes, 10),
		}}
	}

	return data, nil
}

func asValidationError(err error) *ValidationError {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return validationErr
	}

	return &ValidationError{Code: errorCodeInvalidRequest, Message: "invalid proxy request"}
}

func writeProxyAuthFailure(w http.ResponseWriter, requestID string) {
	w.Header().Set("Proxy-Authenticate", `Bearer realm="straw"`)
	WriteError(w, http.StatusProxyAuthRequired, ErrorResponseFromCode(AuthFailure, requestID, nil))
}

type bufferedTunnelClient struct {
	rw *bufio.ReadWriter
}

func (c *bufferedTunnelClient) Read(p []byte) (int, error) {
	n, err := c.rw.Read(p)
	if err != nil {
		return n, fmt.Errorf("read tunnel client: %w", err)
	}

	return n, nil
}

func (c *bufferedTunnelClient) Write(p []byte) (int, error) {
	n, err := c.rw.Write(p)
	if err != nil {
		return n, fmt.Errorf("write tunnel client: %w", err)
	}

	err = c.rw.Flush()
	if err != nil {
		return n, fmt.Errorf("flush tunnel client: %w", err)
	}

	return n, nil
}

func writeHijackedPipelineError(client io.Writer, requestID string, perr *PipelineError) {
	status, response := pipelineHTTPError(requestID, perr)

	body, err := json.Marshal(response)
	if err != nil {
		return
	}

	_, _ = fmt.Fprintf(client, "HTTP/1.1 %d %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", status, http.StatusText(status), len(body))
	_, _ = client.Write(body)
}
