package handlers

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/protocol"
	"github.com/beremaran/straw/internal/protocol/wirepb"
	"github.com/beremaran/straw/internal/validator"
)

const (
	rsaKeyBits    = 2048
	caYears       = 10
	certFilePerm  = 0o600
	defaultStatus = http.StatusOK
	maxTCPPort    = 65535
)

var (
	errMITMCAFilesRequired = errors.New("both MITM_CA_CERT_FILE and MITM_CA_KEY_FILE are required")
	errInvalidMITMCAFiles  = errors.New("invalid MITM CA files")
)

type tunnelControlBroker interface {
	brokerClient
	CoreRequest(ctx context.Context, subject string, body []byte) ([]byte, error)
	CorePublish(ctx context.Context, subject string, body []byte) error
	CoreSubscribe(ctx context.Context, subject string, handler broker.Handler) (broker.Subscription, error)
}

// ProxyHandler handles browser CONNECT proxy traffic.
type ProxyHandler struct {
	control   *ControlHandler
	broker    tunnelControlBroker
	conf      config.ControlConfig
	semaphore chan struct{}
	caCert    *x509.Certificate
	caKey     *rsa.PrivateKey
	certs     map[string]*tls.Certificate
	certMu    sync.Mutex
}

// NewProxyHandler creates a proxy handler for tunnel and MITM CONNECT traffic.
func NewProxyHandler(b tunnelControlBroker, conf config.ControlConfig) (*ProxyHandler, error) {
	h := &ProxyHandler{
		control: NewControlHandler(
			b,
			conf.EgressID,
			conf.AuthToken,
			conf.Routes,
			conf.ResultTimeout,
			conf.AllowPrivateIPs,
		),
		broker:    b,
		conf:      conf,
		semaphore: make(chan struct{}, conf.MaxConcurrentTunnels),
		certs:     make(map[string]*tls.Certificate),
	}

	if conf.MITMCACertFile != "" || conf.MITMCAKeyFile != "" {
		cert, key, err := loadOrCreateCA(conf.MITMCACertFile, conf.MITMCAKeyFile)
		if err != nil {
			return nil, err
		}

		h.caCert = cert
		h.caKey = key
	}

	return h, nil
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		writeError(w, http.StatusMethodNotAllowed, "proxy only supports CONNECT")

		return
	}

	params, ok := h.authorizeProxy(w, r)
	if !ok {
		return
	}

	host, port, ok := splitConnectHost(w, r.Host)
	if !ok {
		return
	}

	err := validator.ValidateTargetURL(r.Context(), "https://"+net.JoinHostPort(host, port), h.conf.AllowPrivateIPs)
	if err != nil {
		writeError(w, http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))

		return
	}

	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()
	default:
		writeError(w, http.StatusTooManyRequests, "too many tunnels")

		return
	}

	if params.Get("mode") == "mitm" {
		h.handleMITM(w, r, params, host)

		return
	}

	h.handleTunnel(w, r, params, host, port)
}

func (h *ProxyHandler) authorizeProxy(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	raw := r.Header.Get("Proxy-Authorization")
	if !strings.HasPrefix(raw, "Basic ") {
		w.Header().Set("Proxy-Authenticate", `Basic realm="straw"`)
		writeError(w, http.StatusProxyAuthRequired, "proxy authorization required")

		return nil, false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "Basic "))
	if err != nil {
		writeError(w, http.StatusProxyAuthRequired, "invalid proxy authorization")

		return nil, false
	}

	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok || password != h.conf.AuthToken {
		writeError(w, http.StatusProxyAuthRequired, "invalid proxy authorization")

		return nil, false
	}

	params, err := url.ParseQuery(username)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid proxy parameters")

		return nil, false
	}

	return params, true
}

func splitConnectHost(w http.ResponseWriter, addr string) (string, string, bool) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" || port == "" {
		writeError(w, http.StatusBadRequest, "CONNECT target must be host:port")

		return "", "", false
	}

	return host, port, true
}

func (h *ProxyHandler) resolveProxyEgress(ctx context.Context, w http.ResponseWriter, params url.Values) (string, bool) {
	req := httptestRequest(ctx, params)

	return h.control.resolveEgress(w, req)
}

func httptestRequest(ctx context.Context, params url.Values) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, http.MethodConnect, "http://proxy", nil)
	req.Header.Set("X-Straw-Egress-ID", params.Get("egress_id"))
	req.Header.Set("X-Straw-Country", params.Get("country"))
	req.Header.Set("X-Straw-IP-Type", params.Get("ip_type"))

	return req
}

func (h *ProxyHandler) handleTunnel(w http.ResponseWriter, r *http.Request, params url.Values, host string, port string) {
	egressID, ok := h.resolveProxyEgress(r.Context(), w, params)
	if !ok {
		return
	}

	tunnelID := "tun_" + strconv.FormatInt(time.Now().UnixNano(), 10)

	portNum, _ := strconv.ParseInt(port, 10, 32)
	if portNum < 1 || portNum > maxTCPPort {
		writeError(w, http.StatusBadRequest, "invalid CONNECT port")

		return
	}

	openBody, err := protocol.MarshalTunnelOpen(&wirepb.TunnelOpen{
		TunnelId: tunnelID,
		Host:     host,
		Port:     int32(portNum),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode tunnel open")

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.conf.TunnelOpenTimeout)
	defer cancel()

	resultBody, err := h.broker.CoreRequest(ctx, "tunnels."+egressID+".open", openBody)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to open tunnel")

		return
	}

	result, err := protocol.UnmarshalTunnelOpenResult(resultBody)
	if err != nil || result.GetError() != nil {
		writeError(w, http.StatusBadGateway, "failed to open tunnel")

		return
	}

	conn, rw, ok := hijack(w)
	if !ok {
		return
	}
	defer func() { _ = conn.Close() }()

	_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = rw.Flush()

	h.pipeTunnel(r.Context(), conn, tunnelID)
}

func (h *ProxyHandler) pipeTunnel(ctx context.Context, conn net.Conn, tunnelID string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	e2c := "tunnels." + tunnelID + ".e2c"
	c2e := "tunnels." + tunnelID + ".c2e"
	closeSubject := "tunnels." + tunnelID + ".close"

	sub, err := h.broker.CoreSubscribe(ctx, e2c, func(_ context.Context, body []byte) error {
		chunk, err := protocol.UnmarshalTunnelChunk(body)
		if err != nil {
			return fmt.Errorf("unmarshal tunnel chunk: %w", err)
		}

		_, err = conn.Write(chunk.GetData())
		if err != nil {
			return fmt.Errorf("write tunnel tcp: %w", err)
		}

		return nil
	})
	if err != nil {
		return
	}
	defer func() { _ = sub.Unsubscribe() }()

	var seq atomic.Uint64

	buf := make([]byte, h.conf.TunnelChunkSize)
	deadline := time.Now().Add(h.conf.TunnelMaxLifetime)

	for {
		_ = conn.SetReadDeadline(time.Now().Add(h.conf.TunnelIdleTimeout))
		if time.Now().After(deadline) {
			break
		}

		n, err := conn.Read(buf)
		if n > 0 {
			msg, marshalErr := protocol.MarshalTunnelChunk(&wirepb.TunnelChunk{
				TunnelId: tunnelID,
				Seq:      seq.Add(1),
				Data:     append([]byte(nil), buf[:n]...),
			})
			if marshalErr == nil {
				_ = h.broker.CorePublish(ctx, c2e, msg)
			}
		}

		if err != nil {
			break
		}
	}

	closeMsg, _ := protocol.MarshalTunnelClose(&wirepb.TunnelClose{TunnelId: tunnelID, Side: "control", Reason: "closed"})
	_ = h.broker.CorePublish(ctx, closeSubject, closeMsg)
}

func hijack(w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, bool) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		writeError(w, http.StatusInternalServerError, "hijacking unsupported")

		return nil, nil, false
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hijack connection")

		return nil, nil, false
	}

	return conn, rw, true
}

func (h *ProxyHandler) handleMITM(w http.ResponseWriter, r *http.Request, params url.Values, host string) {
	if h.caCert == nil || h.caKey == nil {
		writeError(w, http.StatusServiceUnavailable, "MITM CA is not configured")

		return
	}

	conn, rw, ok := hijack(w)
	if !ok {
		return
	}
	defer func() { _ = conn.Close() }()

	_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = rw.Flush()

	cert, err := h.leafCert(host)
	if err != nil {
		return
	}

	tlsConn := tls.Server(conn, &tls.Config{
		Certificates: []tls.Certificate{*cert},
		NextProtos:   []string{"http/1.1"},
		MinVersion:   tls.VersionTLS12,
	})

	err = tlsConn.HandshakeContext(r.Context())
	if err != nil {
		return
	}

	h.serveMITMRequests(r.Context(), tlsConn, bufio.NewReader(tlsConn), params, host)
}

func (h *ProxyHandler) serveMITMRequests(
	ctx context.Context,
	tlsConn *tls.Conn,
	reader *bufio.Reader,
	params url.Values,
	host string,
) {
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}

		if strings.EqualFold(req.Header.Get("Upgrade"), "websocket") || req.Header.Get("Connection") == "Upgrade" {
			_ = (&http.Response{StatusCode: http.StatusNotImplemented, ProtoMajor: 1, ProtoMinor: 1, Body: http.NoBody}).Write(tlsConn)

			return
		}

		resp := h.dispatchMITMRequest(ctx, req, params, host)
		_ = resp.Write(tlsConn)
		_ = resp.Body.Close()

		if req.Close {
			return
		}
	}
}

func (h *ProxyHandler) dispatchMITMRequest(ctx context.Context, req *http.Request, params url.Values, host string) *http.Response {
	rec := newProxyResponse()

	egressID, ok := h.resolveProxyEgress(ctx, rec, params)
	if !ok {
		return rec.response()
	}

	body, _ := io.ReadAll(req.Body)

	wireReq := &wirepb.Request{
		Id:     "req_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Method: req.Method,
		Url:    "https://" + host + req.URL.RequestURI(),
		Body:   body,
	}
	for key, values := range req.Header {
		for _, value := range values {
			wireReq.Headers = append(wireReq.Headers, &wirepb.Header{Key: key, Value: value})
		}
	}

	err := validator.ValidateTargetURL(ctx, wireReq.GetUrl(), h.conf.AllowPrivateIPs)
	if err != nil {
		writeError(rec, http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))

		return rec.response()
	}

	result, ok := h.control.sendAndWait(ctx, rec, egressID, wireReq)
	if !ok {
		return rec.response()
	}

	writeControlResult(rec, result)

	return rec.response()
}

type proxyResponse struct {
	header http.Header
	body   strings.Builder
	status int
}

func newProxyResponse() *proxyResponse {
	return &proxyResponse{header: make(http.Header), status: defaultStatus}
}

func (r *proxyResponse) Header() http.Header { return r.header }

func (r *proxyResponse) WriteHeader(status int) { r.status = status }

func (r *proxyResponse) Write(body []byte) (int, error) {
	n, err := r.body.WriteString(string(body))
	if err != nil {
		return n, fmt.Errorf("write proxy response: %w", err)
	}

	return n, nil
}

func (r *proxyResponse) response() *http.Response {
	return &http.Response{
		StatusCode: r.status,
		Status:     fmt.Sprintf("%d %s", r.status, http.StatusText(r.status)),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     r.header,
		Body:       io.NopCloser(strings.NewReader(r.body.String())),
	}
}

func loadOrCreateCA(certFile, keyFile string) (*x509.Certificate, *rsa.PrivateKey, error) {
	if certFile == "" || keyFile == "" {
		return nil, nil, errMITMCAFilesRequired
	}

	cert, key, err := loadExistingCA(certFile, keyFile)
	if err == nil {
		return cert, key, nil
	}

	key, err = rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, nil, fmt.Errorf("generate MITM CA key: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "Straw MITM CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(caYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create MITM CA cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	err = os.WriteFile(certFile, certPEM, certFilePerm)
	if err != nil {
		return nil, nil, fmt.Errorf("write MITM CA cert: %w", err)
	}

	err = os.WriteFile(keyFile, keyPEM, certFilePerm)
	if err != nil {
		return nil, nil, fmt.Errorf("write MITM CA key: %w", err)
	}

	return parseCA(certPEM, keyPEM)
}

func loadExistingCA(certFile, keyFile string) (*x509.Certificate, *rsa.PrivateKey, error) {
	keyPair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load MITM CA files: %w", err)
	}

	if len(keyPair.Certificate) == 0 {
		return nil, nil, errInvalidMITMCAFiles
	}

	cert, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parse MITM CA cert: %w", err)
	}

	key, ok := keyPair.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errInvalidMITMCAFiles
	}

	return cert, key, nil
}

func parseCA(certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)

	keyBlock, _ := pem.Decode(keyPEM)
	if certBlock == nil || keyBlock == nil {
		return nil, nil, errInvalidMITMCAFiles
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse MITM CA cert: %w", err)
	}

	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse MITM CA key: %w", err)
	}

	return cert, key, nil
}

func (h *ProxyHandler) leafCert(host string) (*tls.Certificate, error) {
	h.certMu.Lock()
	defer h.certMu.Unlock()

	if cert := h.certs[host]; cert != nil {
		return cert, nil
	}

	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("generate MITM leaf key: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, h.caCert, &key.PublicKey, h.caKey)
	if err != nil {
		return nil, fmt.Errorf("create MITM leaf cert: %w", err)
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{der, h.caCert.Raw},
		PrivateKey:  key,
	}
	h.certs[host] = cert

	return cert, nil
}
