package control

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"
)

var errMITMLeafLookupRequired = errors.New("mitm leaf lookup required")

const mitmInnerReadHeaderTimeout = 5 * time.Second

// MITMConnectHandler handles explicit-proxy CONNECT bootstrap before decoded
// HTTPS MITM ingress.
type MITMConnectHandler struct {
	authenticator *Authenticator
	inner         http.Handler
	leafLookup    MITMLeafLookup
	leafPreflight MITMLeafPreflight
}

// NewMITMConnectHandler creates an authenticated CONNECT MITM bootstrap.
func NewMITMConnectHandler(auth *Authenticator, inner http.Handler, leafLookup MITMLeafLookup) *MITMConnectHandler {
	return &MITMConnectHandler{authenticator: auth, inner: inner, leafLookup: leafLookup}
}

// SetLeafPreflight wires an optional cache/admission check that runs before
// the CONNECT tunnel is established.
func (h *MITMConnectHandler) SetLeafPreflight(preflight MITMLeafPreflight) {
	h.leafPreflight = preflight
}

func (h *MITMConnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()

	if r.Method != http.MethodConnect {
		WriteValidationError(w, requestID, &ValidationError{Code: errorCodeUnsupportedIngressMode, Message: "only CONNECT is supported on this listener"})

		return
	}

	identity, err := authenticateConnect(h.authenticator, r)
	if err != nil {
		writeConnectAuthError(w, requestID, err)

		return
	}

	target, verr := validateConnectTarget(r.Host)
	if verr != nil {
		WriteValidationError(w, requestID, verr)

		return
	}

	if h.leafPreflight != nil {
		err := h.leafPreflight(r, identity, target.Host)
		if err != nil {
			writeMITMLeafPreflightError(w, requestID, err)

			return
		}
	}

	h.serveTunnel(w, r, requestID, identity, target.Host)
}

func writeMITMLeafPreflightError(w http.ResponseWriter, requestID string, err error) {
	if errors.Is(err, errMITMLeafGenerationLimit) {
		writePipelineError(w, requestID, &PipelineError{Code: RateLimitExceeded})

		return
	}

	writePipelineError(w, requestID, &PipelineError{Code: ControlInternalError})
}

func (h *MITMConnectHandler) serveTunnel(w http.ResponseWriter, r *http.Request, requestID string, identity Identity, authority string) {
	conn, rw, ok := hijackConnect(w, requestID)
	if !ok {
		return
	}
	defer func() { _ = conn.Close() }()

	err := writeConnectEstablished(rw)
	if err != nil {
		return
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if h.leafLookup == nil {
				return nil, errMITMLeafLookupRequired
			}

			sni := normalizeMITMHostNameOnly(hello.ServerName)

			verr := validateMITMHostSNI(authority, sni)
			if verr != nil {
				return nil, verr
			}

			return h.leafLookup(r, identity, sni, authority)
		},
	}

	tlsConn := tls.Server(conn, tlsConfig)

	err = tlsConn.HandshakeContext(r.Context())
	if err != nil {
		return
	}

	state := tlsConn.ConnectionState()

	verr := validateMITMHostSNI(authority, state.ServerName)
	if verr != nil {
		return
	}

	server := &http.Server{
		Handler:           h.inner,
		ReadHeaderTimeout: mitmInnerReadHeaderTimeout,
	}
	server.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
		return WithMITMIdentity(ctx, identity)
	}
	ln := newSingleConnListener(tlsConn)
	server.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			_ = ln.Close()
		}
	}
	_ = server.Serve(ln)
}

type singleConnListener struct {
	conn   net.Conn
	addr   net.Addr
	closed chan struct{}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		conn := l.conn
		l.conn = nil

		return conn, nil
	}

	<-l.closed

	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}

	return nil
}

func newSingleConnListener(conn net.Conn) *singleConnListener {
	return &singleConnListener{conn: conn, addr: conn.LocalAddr(), closed: make(chan struct{})}
}

func (l *singleConnListener) Addr() net.Addr {
	return l.addr
}
