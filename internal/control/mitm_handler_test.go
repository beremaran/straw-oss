package control

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beremaran/straw/v2/internal/config"
)

const (
	mitmTestHost     = testExampleHost
	mitmTestHostPort = mitmTestHost + ":443"
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
	req.Host = "other.example"
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
		TenantID:   "ten_mitm",
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

func TestMITMNormalizeHost(t *testing.T) {
	t.Parallel()

	if got := normalizeMITMHost("Example.COM.:8443"); got != mitmTestHost+":8443" {
		t.Fatalf("normalizeMITMHost() = %q", got)
	}
	if got := normalizeMITMHost("[2001:db8::1]:443"); got != "[2001:db8::1]:443" {
		t.Fatalf("normalizeMITMHost(ipv6) = %q", got)
	}
}
