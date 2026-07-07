package control

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
)

// MITMHandler handles decoded HTTPS MITM ingress.
type MITMHandler struct {
	*ProxyHandler
}

type mitmIdentityKey struct{}

// MITMLeafLookup receives the CONNECT-authenticated tenant identity before the
// inner TLS handshake selects a certificate.
type MITMLeafLookup func(r *http.Request, identity Identity, sni, authority string) (*tls.Certificate, error)

// MITMLeafPreflight checks whether an authenticated CONNECT target may proceed
// before the tunnel is established.
type MITMLeafPreflight func(r *http.Request, identity Identity, authority string) error

// MITMHTTP2Policy decides whether an authenticated tenant may negotiate h2.
type MITMHTTP2Policy func(ctx context.Context, identity Identity) bool

// NewMITMHandler creates the HTTPS MITM ingress handler.
func NewMITMHandler(maxRequestBodyBytes, maxResponseBodyBytes, maxTimeoutMs uint64, auth *Authenticator, metadataWriter ...RequestMetadataRecorder) *MITMHandler {
	return &MITMHandler{ProxyHandler: NewProxyHandler(maxRequestBodyBytes, maxResponseBodyBytes, maxTimeoutMs, auth, metadataWriter...)}
}

// Authenticator returns the authenticator used by this MITM handler.
func (h *MITMHandler) Authenticator() *Authenticator {
	return h.authenticator
}

// AllowsHTTP2 reports whether tenant routing policy permits MITM h2 ingress.
func (h *MITMHandler) AllowsHTTP2(ctx context.Context, identity Identity) bool {
	if h.configCache == nil {
		return false
	}

	snapshot, err := h.configCache.Snapshot(ctx, identity.TenantID)
	if err != nil {
		return false
	}

	for _, rule := range snapshot.RoutingRules {
		if rule.Enabled && ruleAllowsMITM(rule) {
			return true
		}
	}

	return false
}

// WithMITMIdentity attaches the CONNECT-authenticated identity to the decoded
// inner HTTPS request.
func WithMITMIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, mitmIdentityKey{}, identity)
}

func (h *MITMHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()

	identity, ok := r.Context().Value(mitmIdentityKey{}).(Identity)
	if !ok {
		var err error

		identity, err = h.authenticateProxy(r)
		if err != nil {
			h.writeProxyAuthError(w, requestID, err)

			return
		}
	}

	validated, err := h.validateMITMRequest(w, r)
	if err != nil {
		var verr *ValidationError
		if asValidationError(err, &verr) {
			WriteValidationError(w, requestID, verr)

			return
		}

		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, requestID, nil))

		return
	}

	event := buildRequestEvent(requestID, identity, validated, h.tenantPolicy(r.Context(), identity.TenantID))

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

func (h *MITMHandler) validateMITMRequest(w http.ResponseWriter, r *http.Request) (*ValidatedRequest, error) {
	if r.TLS == nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "MITM ingress requires TLS"}
	}

	err := validateMethod(r.Method)
	if err != nil {
		return nil, err
	}

	host := mitmTargetHost(r)
	if host == "" {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "MITM target host is required"}
	}

	mismatch := validateMITMHostSNI(host, r.TLS.ServerName)
	if mismatch != nil {
		return nil, mismatch
	}

	parsedURL, err := validateURL("https://" + host + r.URL.RequestURI())
	if err != nil {
		return nil, err
	}

	headers, err := proxyHeaders(r.Context(), r.Header)
	if err != nil {
		return nil, err
	}

	body, bodyReader, bodySize, err := h.mitmRequestBody(w, r)
	if err != nil {
		return nil, err
	}

	return &ValidatedRequest{
		Method:        r.Method,
		URL:           parsedURL,
		Headers:       headers,
		BodyData:      body,
		BodyReader:    bodyReader,
		BodySizeBytes: bodySize,
		IngressType:   IngressTypeMITM,
		TimeoutMs:     0,
		Replayable:    proxyReplayable(r.Method),
	}, nil
}

func (h *MITMHandler) mitmRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, io.ReadCloser, int64, error) {
	if r.ProtoMajor >= 2 && r.Body != nil {
		limit, err := proxyBodyLimit(h.maxRequestBodyBytes)
		if err != nil {
			return nil, nil, 0, err
		}

		return nil, http.MaxBytesReader(w, r.Body, limit+1), r.ContentLength, nil
	}

	body, err := readProxyBody(w, r, h.maxRequestBodyBytes)
	if err != nil {
		return nil, nil, 0, err
	}

	return body, nil, int64(len(body)), nil
}

func validateMITMHostSNI(host, sni string) *ValidationError {
	if sni == "" {
		return nil
	}

	hostName := normalizeMITMHostNameOnly(host)

	sni = normalizeMITMHostNameOnly(sni)
	if hostName != "" && sni != "" && hostName != sni {
		return &ValidationError{Code: errorCodeDestinationDenied, Message: "MITM Host and SNI mismatch"}
	}

	return nil
}

func mitmTargetHost(r *http.Request) string {
	host := r.Host
	if host == "" && r.TLS != nil {
		host = r.TLS.ServerName
	}

	return normalizeMITMHost(host)
}

func normalizeMITMHostNameOnly(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		host = h
	}

	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func normalizeMITMHost(host string) string {
	h, port, err := net.SplitHostPort(host)
	if err == nil {
		return net.JoinHostPort(strings.ToLower(strings.TrimSuffix(h, ".")), port)
	}

	return strings.ToLower(strings.TrimSuffix(host, "."))
}
