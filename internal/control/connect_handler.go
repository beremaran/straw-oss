package control

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const connectScheme = "connect"

// ConnectHandler handles P1 raw CONNECT ingress.
type ConnectHandler struct {
	authenticator *Authenticator
	dispatcher    RequestDispatcher
}

// NewConnectHandler creates the raw CONNECT ingress handler.
func NewConnectHandler(auth *Authenticator) *ConnectHandler {
	return &ConnectHandler{authenticator: auth}
}

// SetDispatcher wires the request dispatcher used after validation.
func (h *ConnectHandler) SetDispatcher(dispatcher RequestDispatcher) {
	h.dispatcher = dispatcher
}

func (h *ConnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()

	if r.Method != http.MethodConnect {
		WriteValidationError(w, requestID, &ValidationError{Code: errorCodeUnsupportedIngressMode, Message: "only CONNECT is supported on this listener"})

		return
	}

	identity, err := h.authenticate(r)
	if err != nil {
		writeConnectAuthError(w, requestID, err)

		return
	}

	target, verr := validateConnectTarget(r.Host)
	if verr != nil {
		WriteValidationError(w, requestID, verr)

		return
	}

	tunnel, ok := h.dispatcher.(TunnelDispatcher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, requestID, nil))

		return
	}

	conn, rw, ok := hijackConnect(w, requestID)
	if !ok {
		return
	}
	defer func() { _ = conn.Close() }()

	in := DispatchInput{
		RequestID: requestID,
		Identity:  identity,
		Request: &ValidatedRequest{
			Method:      http.MethodConnect,
			URL:         target,
			IngressType: IngressTypeConnect,
			TimeoutMs:   0,
			Replayable:  false,
		},
	}

	_, _ = tunnel.DispatchTunnel(r.Context(), in, rw)
}

func (h *ConnectHandler) authenticate(r *http.Request) (Identity, error) {
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

func writeConnectAuthError(w http.ResponseWriter, requestID string, err error) {
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

func validateConnectTarget(authority string) (*url.URL, *ValidationError) {
	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "CONNECT target must be host:port"}
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "CONNECT host is required"}
	}

	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "CONNECT port must be between 1 and 65535"}
	}

	return &url.URL{Scheme: connectScheme, Host: net.JoinHostPort(host, port)}, nil
}

func hijackConnect(w http.ResponseWriter, requestID string) (net.Conn, *bufio.ReadWriter, bool) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, requestID, nil))

		return nil, nil, false
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, false
	}

	return conn, rw, true
}

func writeConnectEstablished(rw *bufio.ReadWriter) error {
	_, err := fmt.Fprint(rw, "HTTP/1.1 200 Connection Established\r\n\r\n")
	if err != nil {
		return fmt.Errorf("write connect established: %w", err)
	}

	err = rw.Flush()
	if err != nil {
		return fmt.Errorf("flush connect established: %w", err)
	}

	return nil
}
