package control

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"strings"
)

// ProxyHandler handles P1 HTTP forward-proxy requests.
type ProxyHandler struct {
	*RequestHandler
}

type rawProxyHeader struct {
	name  string
	value string
}

type proxyRawHeaderSource interface {
	ProxyRawHeaderBlock() []byte
}

type proxyRawHeaderSourceKey struct{}

// WithProxyRawHeaderSource attaches the raw request header block captured by
// the proxy listener so forwarded headers can preserve wire order.
func WithProxyRawHeaderSource(ctx context.Context, source proxyRawHeaderSource) context.Context {
	return context.WithValue(ctx, proxyRawHeaderSourceKey{}, source)
}

// NewProxyHandler creates the HTTP forward-proxy handler.
func NewProxyHandler(maxRequestBodyBytes, maxResponseBodyBytes, maxTimeoutMs uint64, auth *Authenticator, metadataWriter ...RequestMetadataRecorder) *ProxyHandler {
	return &ProxyHandler{RequestHandler: NewRequestHandler(maxRequestBodyBytes, maxResponseBodyBytes, maxTimeoutMs, auth, metadataWriter...)}
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()

	identity, err := h.authenticateProxy(r)
	if err != nil {
		h.writeProxyAuthError(w, requestID, err)

		return
	}

	validated, err := h.validateProxyRequest(w, r)
	if err != nil {
		var verr *ValidationError
		if asValidationError(err, &verr) {
			WriteValidationError(w, requestID, verr)

			return
		}

		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, requestID, nil))

		return
	}

	event := buildRequestEvent(requestID, identity, validated)

	if h.dispatcher == nil {
		perr := &PipelineError{Code: ControlInternalError}
		h.recordOutcome(event, SuccessResponse{}, perr)
		h.writeProxyPipelineError(w, requestID, perr)

		return
	}

	in := DispatchInput{RequestID: requestID, Identity: identity, Request: validated}
	if raw, ok := h.dispatcher.(RawResponseDispatcher); ok {
		resp, perr, wroteHeader := raw.DispatchRaw(r.Context(), in, w)
		h.recordOutcome(event, resp, perr)

		if perr != nil && !wroteHeader {
			h.writeProxyPipelineError(w, requestID, perr)
		}

		return
	}

	resp, perr := h.dispatcher.Dispatch(r.Context(), in)
	h.recordOutcome(event, resp, perr)

	if perr != nil {
		h.writeProxyPipelineError(w, requestID, perr)

		return
	}

	writeProxySuccess(w, resp)
}

func (h *ProxyHandler) authenticateProxy(r *http.Request) (Identity, error) {
	if h.authenticator == nil {
		return Identity{}, ErrAuthFailure
	}

	identity, err := h.authenticator.Authenticate(r.Context(), r.Header.Get(headerNameProxyAuthorization))
	if err != nil {
		return Identity{}, err
	}

	if !CanExecuteDataPlane(identity) {
		return Identity{}, ErrInsufficientPermissions
	}

	return identity, nil
}

func (h *ProxyHandler) writeProxyAuthError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, ErrInsufficientPermissions):
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, requestID, nil))
	case errors.Is(err, ErrTenantNotFound):
		w.Header().Set("Proxy-Authenticate", "Bearer")
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(TenantNotFound, requestID, nil))
	default:
		w.Header().Set("Proxy-Authenticate", "Bearer")
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(AuthFailure, requestID, nil))
	}
}

func (h *ProxyHandler) validateProxyRequest(w http.ResponseWriter, r *http.Request) (*ValidatedRequest, error) {
	if r.Method == http.MethodConnect {
		return nil, &ValidationError{Code: errorCodeUnsupportedIngressMode, Message: "CONNECT is not supported on the HTTP forward proxy listener"}
	}

	err := validateMethod(r.Method)
	if err != nil {
		return nil, err
	}

	target := r.URL.String()
	if !r.URL.IsAbs() {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "proxy request target must be absolute-form"}
	}

	parsedURL, err := validateURL(target)
	if err != nil {
		return nil, err
	}

	headers, err := proxyHeaders(r.Context(), r.Header)
	if err != nil {
		return nil, err
	}

	body, err := readProxyBody(w, r, h.maxRequestBodyBytes)
	if err != nil {
		return nil, err
	}

	return &ValidatedRequest{
		Method:      r.Method,
		URL:         parsedURL,
		Headers:     headers,
		BodyData:    body,
		IngressType: IngressTypeHTTPProxy,
		TimeoutMs:   0,
		Replayable:  proxyReplayable(r.Method),
	}, nil
}

func proxyHeaders(ctx context.Context, headers http.Header) ([]HeaderPair, error) {
	if source, ok := ctx.Value(proxyRawHeaderSourceKey{}).(proxyRawHeaderSource); ok {
		raw := source.ProxyRawHeaderBlock()
		if len(raw) > 0 {
			return proxyHeadersFromRaw(raw)
		}
	}

	return proxyHeadersFromMap(headers)
}

func proxyHeadersFromMap(headers http.Header) ([]HeaderPair, error) {
	connectionHeaders := map[string]bool{}

	for _, value := range headers.Values(http.CanonicalHeaderKey(headerNameConnection)) {
		for token := range strings.SplitSeq(value, ",") {
			name := strings.TrimSpace(token)
			if name != "" {
				connectionHeaders[strings.ToLower(name)] = true
			}
		}
	}

	out := make([]HeaderPair, 0, len(headers))
	for name, values := range headers {
		if stripProxyRequestHeader(name, connectionHeaders) {
			continue
		}

		for _, value := range values {
			pair := HeaderPair{Name: name, Value: base64.StdEncoding.EncodeToString([]byte(value))}

			_, err := validateHeaderPair(pair)
			if err != nil {
				return nil, err
			}

			out = append(out, pair)
		}
	}

	return validateHeaders(out)
}

func proxyHeadersFromRaw(raw []byte) ([]HeaderPair, error) {
	rawHeaders, err := parseRawProxyHeaders(raw)
	if err != nil {
		return nil, err
	}

	connectionHeaders := rawProxyConnectionHeaders(rawHeaders)

	out := make([]HeaderPair, 0, len(rawHeaders))
	for _, h := range rawHeaders {
		if stripProxyRequestHeader(h.name, connectionHeaders) {
			continue
		}

		pair := HeaderPair{Name: h.name, Value: base64.StdEncoding.EncodeToString([]byte(h.value))}

		_, err := validateHeaderPair(pair)
		if err != nil {
			return nil, err
		}

		out = append(out, pair)
	}

	return validateHeaders(out)
}

func parseRawProxyHeaders(raw []byte) ([]rawProxyHeader, error) {
	lines := strings.Split(string(raw), "\r\n")
	if len(lines) == 0 {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "malformed proxy request headers"}
	}

	rawHeaders := make([]rawProxyHeader, 0, len(lines)-1)

	for _, line := range lines[1:] {
		if line == "" {
			break
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "folded proxy request headers are rejected"}
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "malformed proxy request header"}
		}

		value = textproto.TrimString(value)
		rawHeaders = append(rawHeaders, rawProxyHeader{name: name, value: value})
	}

	return rawHeaders, nil
}

func rawProxyConnectionHeaders(rawHeaders []rawProxyHeader) map[string]bool {
	connectionHeaders := map[string]bool{}

	for _, h := range rawHeaders {
		if !strings.EqualFold(h.name, headerNameConnection) {
			continue
		}

		for token := range strings.SplitSeq(h.value, ",") {
			token = strings.TrimSpace(token)
			if token != "" {
				connectionHeaders[strings.ToLower(token)] = true
			}
		}
	}

	return connectionHeaders
}

func stripProxyRequestHeader(name string, connectionHeaders map[string]bool) bool {
	if connectionHeaders[strings.ToLower(name)] || strings.HasPrefix(strings.ToLower(name), "x-straw-") {
		return true
	}

	switch strings.ToLower(name) {
	case "host", headerNameProxyAuthorization, "proxy-connection", headerNameContentLength, headerNameTransferEncoding,
		headerNameConnection, "keep-alive", "te", "trailer", "upgrade":
		return true
	default:
		return false
	}
}

func readProxyBody(w http.ResponseWriter, r *http.Request, maxBytes uint64) ([]byte, error) {
	const maxReadableBodyBytes = uint64(1<<63 - 2)

	if r.Body == nil {
		return nil, nil
	}

	if maxBytes > maxReadableBodyBytes {
		return nil, &ValidationError{Code: errorCodeBodyTooLarge, Message: requestBodyExceedsLimit}
	}

	limit := int64(maxBytes)
	r.Body = http.MaxBytesReader(w, r.Body, limit+1)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, &ValidationError{Code: errorCodeBodyTooLarge, Message: requestBodyExceedsLimit}
	}

	if uint64(len(body)) > maxBytes {
		return nil, &ValidationError{Code: errorCodeBodyTooLarge, Message: requestBodyExceedsLimit}
	}

	return body, nil
}

func proxyReplayable(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func (h *ProxyHandler) writeProxyPipelineError(w http.ResponseWriter, requestID string, err *PipelineError) {
	if err != nil && err.Code == RouteNoMatch {
		resp := ErrorResponseFromCodeWithRetry(err.Code, requestID, err.Details, err.RetryAfterMs)
		WriteError(w, http.StatusMisdirectedRequest, resp)

		return
	}

	writePipelineError(w, requestID, err)
}

func writeProxySuccess(w http.ResponseWriter, resp SuccessResponse) {
	for _, h := range resp.Headers {
		value, err := base64.StdEncoding.DecodeString(h.Value)
		if err == nil {
			w.Header().Add(h.Name, string(value))
		}
	}

	body, _ := base64.StdEncoding.DecodeString(resp.Body.DataBase64)
	w.WriteHeader(resp.Status)
	_, _ = w.Write(body)
}
