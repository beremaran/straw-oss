package control

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

const (
	mitmTestHost     = testExampleHost
	mitmTestHostPort = mitmTestHost + ":443"
	mitmTestTenant   = "ten_mitm"
	mitmOtherHost    = "other.example"
)

func TestMITMHandlerMapsDecodedTLSRequest(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestMITMHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/path?q=1", strings.NewReader("hi"))
	req.Host = "Example.COM:443"
	req.TLS = &tls.ConnectionState{ServerName: mitmTestHost}
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	req.Header.Set("Authorization", "Bearer upstream")
	req.Header.Set(headerCanonicalConnection, "X-Hop")
	req.Header.Set("X-Hop", "drop")
	req.Header.Set("X-Straw-Route", "drop")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	in := dispatcher.last
	if in.Request == nil {
		t.Fatal("dispatcher request is nil")
	}
	if got := in.Request.URL.String(); got != "https://"+mitmTestHostPort+"/path?q=1" {
		t.Fatalf("url = %q, want decoded HTTPS target", got)
	}
	if in.Request.IngressType != IngressTypeMITM {
		t.Fatalf("ingress = %q, want %q", in.Request.IngressType, IngressTypeMITM)
	}
	if string(in.Request.BodyData) != "hi" {
		t.Fatalf("body = %q, want hi", in.Request.BodyData)
	}
	if in.Request.Replayable {
		t.Fatal("POST MITM request should not be replayable")
	}

	got := decodedProxyHeaders(in.Request.Headers)
	for _, name := range []string{http.CanonicalHeaderKey(headerNameProxyAuthorization), headerCanonicalConnection, "X-Hop", "X-Straw-Route", "Host"} {
		if _, ok := got[name]; ok {
			t.Fatalf("header %q was forwarded: %#v", name, got)
		}
	}
	if got["Authorization"] != "Bearer upstream" {
		t.Fatalf("Authorization = %q, want upstream header forwarded", got["Authorization"])
	}
}

func TestMITMHandlerRequiresTLS(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestMITMHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Host = mitmTestHost
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestMITMHandlerRouteNoMatchUses421(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestMITMHandler(t)
	dispatcher.err = &PipelineError{Code: RouteNoMatch}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Host = mitmTestHost
	req.TLS = &tls.ConnectionState{ServerName: mitmTestHost}
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMisdirectedRequest)
	}
}

func TestMITMHandlerWritesDeniedDestination(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestMITMHandler(t)
	dispatcher.err = &PipelineError{Code: DestinationDenied}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Host = "169.254.169.254"
	req.TLS = &tls.ConnectionState{ServerName: "169.254.169.254"}
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if dispatcher.last.Request.URL.String() != "https://169.254.169.254/" {
		t.Fatalf("url = %q, want denied destination target", dispatcher.last.Request.URL.String())
	}

	_, verr := ResolveDestinationPolicy(DestinationPolicyRequest{
		Snapshot:               config.TenantSnapshot{FingerprintProfiles: fingerprintSnapshot()},
		TargetURL:              dispatcher.last.Request.URL,
		MaxInjectedHeaderBytes: 1024,
	})
	if verr == nil || verr.Code != errorCodeDestinationDenied {
		t.Fatalf("ResolveDestinationPolicy() = %+v, want destination_denied", verr)
	}
}

func TestMITMHandlerRejectsHostSNIMismatch(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestMITMHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Host = mitmOtherHost
	req.TLS = &tls.ConnectionState{ServerName: mitmTestHost}
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestMITMConnectAuthenticatesBeforeLeafLookupAndServesInnerRequest(t *testing.T) {
	t.Parallel()

	inner, token, dispatcher := newTestMITMHandler(t)
	var gotIdentity Identity
	var gotSNI, gotAuthority string
	cert, pool := newTestServerCertificate(t, mitmTestHost)
	h := NewMITMConnectHandler(inner.Authenticator(), inner, func(_ *http.Request, identity Identity, sni, authority string) (*tls.Certificate, error) {
		gotIdentity = identity
		gotSNI = sni
		gotAuthority = authority

		return cert, nil
	})

	server := httptest.NewServer(h)
	defer server.Close()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err = io.WriteString(conn, "CONNECT Example.COM.:443 HTTP/1.1\r\nHost: Example.COM.:443\r\nProxy-Authorization: Bearer "+token+"\r\n\r\n")
	if err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want 200", resp.StatusCode)
	}

	tlsConn := tls.Client(&bufferedConn{Conn: conn, r: br}, &tls.Config{RootCAs: pool, ServerName: mitmTestHost})
	err = tlsConn.HandshakeContext(context.Background())
	if err != nil {
		t.Fatalf("inner TLS handshake: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+mitmTestHostPort+"/path", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Host = mitmTestHostPort
	err = req.Write(tlsConn)
	if err != nil {
		t.Fatalf("write inner request: %v", err)
	}
	innerResp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		t.Fatalf("read inner response: %v", err)
	}
	defer func() { _ = innerResp.Body.Close() }()
	if innerResp.StatusCode != http.StatusNotFound {
		t.Fatalf("inner status = %d, want dispatch response", innerResp.StatusCode)
	}

	if gotIdentity.TenantID != mitmTestTenant {
		t.Fatalf("leaf identity tenant = %q, want %s", gotIdentity.TenantID, mitmTestTenant)
	}
	if gotSNI != mitmTestHost {
		t.Fatalf("leaf sni = %q, want %q", gotSNI, mitmTestHost)
	}
	if gotAuthority != mitmTestHostPort {
		t.Fatalf("leaf authority = %q, want %q", gotAuthority, mitmTestHostPort)
	}
	if dispatcher.last.Identity.TenantID != mitmTestTenant {
		t.Fatalf("dispatch tenant = %q, want CONNECT-authenticated tenant", dispatcher.last.Identity.TenantID)
	}
}

func TestMITMConnectRequiresProxyAuthorizationBeforeLeafLookup(t *testing.T) {
	t.Parallel()

	inner, _, dispatcher := newTestMITMHandler(t)
	leafCalls := 0
	h := NewMITMConnectHandler(inner.Authenticator(), inner, func(_ *http.Request, _ Identity, _, _ string) (*tls.Certificate, error) {
		leafCalls++

		cert, _ := newTestServerCertificate(t, mitmTestHost)

		return cert, nil
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://"+mitmTestHostPort, nil)
	req.Host = mitmTestHostPort
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if leafCalls != 0 {
		t.Fatalf("leaf calls = %d, want 0", leafCalls)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestMITMConnectLeafPreflightWritesRateLimitBeforeTunnel(t *testing.T) {
	t.Parallel()

	inner, token, dispatcher := newTestMITMHandler(t)
	leafCalls := 0
	h := NewMITMConnectHandler(inner.Authenticator(), inner, func(_ *http.Request, _ Identity, _, _ string) (*tls.Certificate, error) {
		leafCalls++

		cert, _ := newTestServerCertificate(t, mitmTestHost)

		return cert, nil
	})
	h.SetLeafPreflight(func(_ *http.Request, identity Identity, authority string) error {
		if identity.TenantID != mitmTestTenant {
			t.Fatalf("preflight tenant = %q, want %s", identity.TenantID, mitmTestTenant)
		}
		if authority != mitmTestHostPort {
			t.Fatalf("preflight authority = %q, want %s", authority, mitmTestHostPort)
		}

		return errMITMLeafGenerationLimit
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://"+mitmTestHostPort, nil)
	req.Host = mitmTestHostPort
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}

	var resp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Code != errorCodeRateLimitExceeded {
		t.Fatalf("error code = %q, want %q", resp.Code, errorCodeRateLimitExceeded)
	}
	if leafCalls != 0 {
		t.Fatalf("leaf calls = %d, want 0", leafCalls)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestMITMConnectRejectsSNIMismatchBeforeLeafLookup(t *testing.T) {
	t.Parallel()

	inner, token, _ := newTestMITMHandler(t)
	leafCalls := 0
	h := NewMITMConnectHandler(inner.Authenticator(), inner, func(_ *http.Request, _ Identity, _, _ string) (*tls.Certificate, error) {
		leafCalls++

		cert, _ := newTestServerCertificate(t, mitmTestHost)

		return cert, nil
	})

	server := httptest.NewServer(h)
	defer server.Close()

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err = io.WriteString(conn, "CONNECT "+mitmTestHostPort+" HTTP/1.1\r\nHost: "+mitmTestHostPort+"\r\nProxy-Authorization: Bearer "+token+"\r\n\r\n")
	if err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want 200", resp.StatusCode)
	}

	tlsConn := tls.Client(&bufferedConn{Conn: conn, r: br}, &tls.Config{ServerName: mitmOtherHost})
	err = tlsConn.HandshakeContext(context.Background())
	if err == nil {
		t.Fatal("inner TLS handshake error = nil, want SNI mismatch failure")
	}
	if leafCalls != 0 {
		t.Fatalf("leaf calls = %d, want 0", leafCalls)
	}
}

func newTestMITMHandler(t *testing.T) (*MITMHandler, string, *captureProxyDispatcher) {
	t.Helper()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("test-pepper")

	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	err = store.Create(context.Background(), APIKeyRecord{
		ID:         "key_mitm_requester",
		ScopeType:  ScopeTenant,
		TenantID:   mitmTestTenant,
		Role:       RoleRequester,
		Prefix:     generated.Prefix,
		SecretHash: HashAPIKeySecret(generated.Secret, pepper),
		Status:     APIKeyStatusActive,
	})
	if err != nil {
		t.Fatalf("store.Create() error = %v", err)
	}

	dispatcher := &captureProxyDispatcher{}
	h := NewMITMHandler(1_048_576, 1_048_576, 120_000, NewAuthenticator(store, pepper))
	h.SetDispatcher(dispatcher)

	return h, generated.Secret, dispatcher
}

func TestMITMRawDispatcherWritesDecodedResponse(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestMITMHandler(t)
	dispatcher := &rawProxyDispatcher{
		status: http.StatusOK,
		headers: http.Header{
			headerCanonicalContentType: []string{mediaTypeTextPlain},
		},
		chunks: [][]byte{[]byte("ok")},
	}
	h.SetDispatcher(dispatcher)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Host = mitmTestHost
	req.TLS = &tls.ConnectionState{ServerName: mitmTestHost}
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "ok" {
		t.Fatalf("body = %q, want ok", got)
	}
	if dispatcher.last.Request.IngressType != IngressTypeMITM {
		t.Fatalf("ingress = %q, want %q", dispatcher.last.Request.IngressType, IngressTypeMITM)
	}
}

func TestMITMHTTP2PseudoHeadersNormalizeToRequest(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestMITMHandler(t)
	server := newMITMHTTP2TestServer(t, h)
	req := newMITMHTTP2Request(t, server.URL+"/path?q=1", token)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if dispatcher.last.Request == nil {
		t.Fatal("dispatcher request is nil")
	}
	if got := dispatcher.last.Request.Method; got != http.MethodGet {
		t.Fatalf("method = %q, want GET", got)
	}
	if got := dispatcher.last.Request.URL.String(); got != "https://"+mitmTestHost+"/path?q=1" {
		t.Fatalf("url = %q, want normalized h2 pseudo-header target", got)
	}
	if got := decodedProxyHeaders(dispatcher.last.Request.Headers); got[":authority"] != "" || got[":path"] != "" || got[":scheme"] != "" || got[":method"] != "" {
		t.Fatalf("pseudo-headers forwarded: %#v", got)
	}
}

func TestMITMRejectsColonPrefixedHeader(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestMITMHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Host = mitmTestHost
	req.TLS = &tls.ConnectionState{ServerName: mitmTestHost}
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)
	req.Header[":evil"] = []string{"inject"}
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestMITMHTTP2ForwardsTrailersAfterBody(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestMITMHandler(t)
	h.SetDispatcher(&rawProxyDispatcher{
		status: http.StatusOK,
		headers: http.Header{
			headerCanonicalContentType: []string{mediaTypeTextPlain},
		},
		trailers: http.Header{handlerTestUpstreamTrailer: []string{"done"}},
		chunks:   [][]byte{[]byte("ok")},
	})
	server := newMITMHTTP2TestServer(t, h)

	resp, err := server.Client().Do(newMITMHTTP2Request(t, server.URL+"/trailers", token))
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
	if got := resp.Trailer.Get(handlerTestUpstreamTrailer); got != "done" {
		t.Fatalf("trailer = %q, want done", got)
	}
}

func TestMITMHTTP2StreamsHaveUniqueRequestIDs(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestMITMHandler(t)
	dispatcher := &mitmHTTP2TrackingDispatcher{release: make(chan struct{})}
	h.SetDispatcher(dispatcher)
	server := newMITMHTTP2TestServer(t, h)

	reqs := []*http.Request{
		newMITMHTTP2Request(t, server.URL+"/one", token),
		newMITMHTTP2Request(t, server.URL+"/two", token),
	}

	responses := make(chan error, len(reqs))
	for _, req := range reqs {
		go func() {
			resp, err := server.Client().Do(req)
			if err != nil {
				responses <- err

				return
			}
			responses <- resp.Body.Close()
		}()
	}
	dispatcher.waitStarted(t, 2)
	close(dispatcher.release)

	for range reqs {
		err := <-responses
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
	}

	ids := dispatcher.requestIDs()
	if len(ids) != 2 {
		t.Fatalf("request IDs = %v, want 2", ids)
	}
	if ids[0] == "" || ids[1] == "" || ids[0] == ids[1] {
		t.Fatalf("request IDs = %v, want unique non-empty IDs", ids)
	}
}

func TestMITMHTTP2StreamCancelIsIsolated(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestMITMHandler(t)
	dispatcher := &mitmHTTP2TrackingDispatcher{release: make(chan struct{})}
	h.SetDispatcher(dispatcher)
	server := newMITMHTTP2TestServer(t, h)

	ctx1, cancel1 := context.WithCancel(t.Context())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()

	go doMITMHTTP2Request(t, server.Client(), newMITMHTTP2Request(t, server.URL+"/one", token).WithContext(ctx1))
	go doMITMHTTP2Request(t, server.Client(), newMITMHTTP2Request(t, server.URL+"/two", token).WithContext(ctx2))

	dispatcher.waitStarted(t, 2)
	firstID := dispatcher.requestIDForPath(t, "/one")
	cancel1()
	dispatcher.waitCancelled(t, firstID)

	if cancelled := dispatcher.cancelledIDs(); len(cancelled) != 1 || cancelled[0] != firstID {
		t.Fatalf("cancelled IDs = %v, want only %q", cancelled, firstID)
	}
	close(dispatcher.release)
}

func TestMITMHTTP2ConnectionCloseCancelsAllStreams(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestMITMHandler(t)
	dispatcher := &mitmHTTP2TrackingDispatcher{release: make(chan struct{})}
	h.SetDispatcher(dispatcher)
	server := newMITMHTTP2TestServer(t, h)

	client := server.Client()
	go doMITMHTTP2Request(t, client, newMITMHTTP2Request(t, server.URL+"/one", token))
	go doMITMHTTP2Request(t, client, newMITMHTTP2Request(t, server.URL+"/two", token))

	dispatcher.waitStarted(t, 2)
	client.CloseIdleConnections()
	server.CloseClientConnections()

	for _, id := range dispatcher.requestIDs() {
		dispatcher.waitCancelled(t, id)
	}
	close(dispatcher.release)
}

func TestMITMNormalizeHost(t *testing.T) {
	t.Parallel()

	if got := normalizeMITMHost("Example.COM.:8443"); got != mitmTestHost+":8443" {
		t.Fatalf("normalizeMITMHost() = %q", got)
	}
	if got := normalizeMITMHost("[2001:db8::1]:443"); got != "[2001:db8::1]:443" {
		t.Fatalf("normalizeMITMHost(ipv6) = %q", got)
	}
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

type mitmHTTP2TrackingDispatcher struct {
	mu        sync.Mutex
	started   map[string]chan struct{}
	cancelled map[string]chan struct{}
	paths     map[string]string
	order     []string
	release   chan struct{}
}

func (d *mitmHTTP2TrackingDispatcher) Dispatch(ctx context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	resp, perr, _ := d.DispatchRaw(ctx, in, httptest.NewRecorder())

	return resp, perr
}

func (d *mitmHTTP2TrackingDispatcher) DispatchRaw(ctx context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	d.mu.Lock()
	if d.started == nil {
		d.started = make(map[string]chan struct{})
		d.cancelled = make(map[string]chan struct{})
		d.paths = make(map[string]string)
	}
	d.order = append(d.order, in.RequestID)
	d.paths[in.RequestID] = in.Request.URL.Path
	started := make(chan struct{})
	cancelled := make(chan struct{})
	d.started[in.RequestID] = started
	d.cancelled[in.RequestID] = cancelled
	close(started)
	d.mu.Unlock()

	select {
	case <-ctx.Done():
		close(cancelled)

		return SuccessResponse{}, &PipelineError{Code: Cancelled}, false
	case <-d.release:
		w.WriteHeader(http.StatusNoContent)

		return SuccessResponse{RequestID: in.RequestID, Status: http.StatusNoContent}, nil, true
	}
}

func (d *mitmHTTP2TrackingDispatcher) waitStarted(t *testing.T, n int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		d.mu.Lock()
		count := len(d.order)
		d.mu.Unlock()
		if count >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("started streams = %d, want %d", count, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (d *mitmHTTP2TrackingDispatcher) requestIDs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string(nil), d.order...)
}

func (d *mitmHTTP2TrackingDispatcher) requestIDForPath(t *testing.T, path string) string {
	t.Helper()

	d.mu.Lock()
	defer d.mu.Unlock()

	for requestID, got := range d.paths {
		if got == path {
			return requestID
		}
	}
	t.Fatalf("no request for path %q in %v", path, d.paths)

	return ""
}

func (d *mitmHTTP2TrackingDispatcher) waitCancelled(t *testing.T, requestID string) {
	t.Helper()

	d.mu.Lock()
	ch := d.cancelled[requestID]
	d.mu.Unlock()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("request %q was not cancelled", requestID)
	}
}

func (d *mitmHTTP2TrackingDispatcher) cancelledIDs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]string, 0, len(d.cancelled))
	for requestID, ch := range d.cancelled {
		select {
		case <-ch:
			out = append(out, requestID)
		default:
		}
	}

	return out
}

func newMITMHTTP2TestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("request protocol = %s, want HTTP/2", r.Proto)
		}
		h.ServeHTTP(w, r)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	return server
}

func newMITMHTTP2Request(t *testing.T, rawURL, token string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Host = mitmTestHost
	req.Header.Set(headerNameProxyAuthorization, "Bearer "+token)

	return req
}

func doMITMHTTP2Request(t *testing.T, client *http.Client, req *http.Request) {
	t.Helper()

	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func newTestServerCertificate(t *testing.T, host string) (*tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair() error = %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)

	return &cert, pool
}
