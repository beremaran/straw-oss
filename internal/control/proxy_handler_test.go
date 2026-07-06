package control

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxyHandlerMapsRequestAndWritesRawResponse(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestProxyHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/path?q=1", nil)
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	req.Header.Set("Accept", mediaTypeTextPlain)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if got := w.Body.String(); got != "missing" {
		t.Fatalf("body = %q, want missing", got)
	}
	if got := w.Header().Get(headerCanonicalContentType); got != mediaTypeTextPlain {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}

	in := dispatcher.last
	if in.Request == nil {
		t.Fatal("dispatcher request is nil")
	}
	if in.Request.URL.String() != "http://example.com/path?q=1" {
		t.Fatalf("url = %q", in.Request.URL.String())
	}
	if in.Request.IngressType != IngressTypeHTTPProxy {
		t.Fatalf("ingress = %q, want %q", in.Request.IngressType, IngressTypeHTTPProxy)
	}
	if !in.Request.Replayable {
		t.Fatal("GET proxy request should be replayable")
	}
}

func TestProxyHandlerUsesRawDispatcherWithoutJSONEnvelope(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestProxyHandler(t)
	dispatcher := &rawProxyDispatcher{
		status: http.StatusInternalServerError,
		headers: http.Header{
			headerCanonicalContentType: []string{mediaTypeTextPlain},
		},
		chunks: [][]byte{[]byte("up"), []byte("stream")},
	}
	h.SetDispatcher(dispatcher)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/path", nil)
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if got := w.Body.String(); got != "upstream" {
		t.Fatalf("body = %q, want upstream", got)
	}
	if strings.Contains(w.Body.String(), "request_id") {
		t.Fatal("raw proxy response was wrapped in JSON envelope")
	}
	if dispatcher.calls != 1 {
		t.Fatalf("raw dispatcher calls = %d, want 1", dispatcher.calls)
	}
}

func TestProxyHandlerDoesNotRenderSecondErrorAfterPartialRawResponse(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestProxyHandler(t)
	h.SetDispatcher(&rawProxyDispatcher{
		status: http.StatusOK,
		chunks: [][]byte{[]byte("partial")},
		err:    &PipelineError{Code: UpstreamReset},
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/path", nil)
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want only partial upstream bytes", got)
	}
}

func TestProxyHandlerClientCancellationReachesRawDispatcher(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestProxyHandler(t)
	dispatcher := &blockingRawProxyDispatcher{started: make(chan struct{}), done: make(chan struct{})}
	h.SetDispatcher(dispatcher)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/path", nil)
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	go h.ServeHTTP(w, req)
	<-dispatcher.started
	cancel()

	select {
	case <-dispatcher.done:
	case <-time.After(time.Second):
		t.Fatal("raw dispatcher did not observe client cancellation")
	}
}

func TestProxyHandlerAuthenticatesWithProxyAuthorizationOnly(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestProxyHandler(t)

	for _, tt := range []struct {
		name      string
		proxyAuth string
		auth      string
	}{
		{name: "missing proxy auth", auth: "Bearer " + token},
		{name: "malformed proxy auth", proxyAuth: "Basic " + token},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
			req.Header.Set("Authorization", tt.auth)
			req.Header.Set("Proxy-Authorization", tt.proxyAuth)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
			}
			if got := w.Header().Get("Proxy-Authenticate"); got != "Bearer" {
				t.Fatalf("Proxy-Authenticate = %q, want Bearer", got)
			}
		})
	}
}

func TestProxyHandlerRejectsRevokedAndUnauthorizedKeys(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		role   Role
		status APIKeyStatus
		want   int
	}{
		{name: "revoked", role: RoleRequester, status: APIKeyStatusRevoked, want: http.StatusUnauthorized},
		{name: "viewer", role: RoleViewer, status: APIKeyStatusActive, want: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, token, _ := newTestProxyHandlerWithKey(t, tt.role, tt.status)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
			req.Header.Set("Proxy-Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestProxyHandlerStripsProxyAndInternalHeaders(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestProxyHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/", strings.NewReader("hi"))
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	req.Header.Set("Authorization", "Bearer upstream")
	req.Header.Set(headerCanonicalConnection, "X-Hop")
	req.Header.Set("X-Hop", "drop")
	req.Header.Set("X-Straw-Route", "drop")
	req.Header.Set("Proxy-Connection", "keep-alive")
	req.Header.Set("Transfer-Encoding", "chunked")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	got := decodedProxyHeaders(dispatcher.last.Request.Headers)
	for _, name := range []string{"Proxy-Authorization", headerCanonicalConnection, "X-Hop", "X-Straw-Route", "Proxy-Connection", "Transfer-Encoding", "Host", headerCanonicalContentLength} {
		if _, ok := got[name]; ok {
			t.Fatalf("header %q was forwarded: %#v", name, got)
		}
	}
	if got["Authorization"] != "Bearer upstream" {
		t.Fatalf("Authorization = %q, want upstream header forwarded", got["Authorization"])
	}
	if string(dispatcher.last.Request.BodyData) != "hi" {
		t.Fatalf("body = %q, want hi", dispatcher.last.Request.BodyData)
	}
	if dispatcher.last.Request.Replayable {
		t.Fatal("POST proxy request should not be replayable")
	}
}

func TestProxyHandlerRejectsCONNECTAndRelativeTargets(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestProxyHandler(t)
	userinfoTarget := "http://user:" + "pass@example.com/"

	for _, tt := range []struct {
		name   string
		method string
		target string
		code   string
	}{
		{name: "connect", method: http.MethodConnect, target: "http://example.com:443", code: errorCodeUnsupportedIngressMode},
		{name: "relative", method: http.MethodGet, target: "/relative", code: errorCodeInvalidRequest},
		{name: "userinfo", method: http.MethodGet, target: userinfoTarget, code: errorCodeInvalidRequest},
		{name: "empty host", method: http.MethodGet, target: "http:///path", code: errorCodeInvalidRequest},
		{name: "ipv6 zone", method: http.MethodGet, target: "http://[fe80::1%25en0]/", code: errorCodeInvalidRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), tt.method, tt.target, nil)
			req.Header.Set("Proxy-Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			var resp ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			if err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if resp.Code != tt.code {
				t.Fatalf("code = %q, want %q", resp.Code, tt.code)
			}
		})
	}

	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestProxyURLValidationRejectsFragments(t *testing.T) {
	t.Parallel()

	_, err := validateURL("http://example.com/path#fragment")
	if err == nil {
		t.Fatal("expected fragment validation error")
	}
}

func TestProxyHeadersRejectMalformedHeaderName(t *testing.T) {
	t.Parallel()

	_, err := proxyHeaders(context.Background(), http.Header{"Bad Header": []string{"x"}})
	if err == nil {
		t.Fatal("expected malformed header name error")
	}
}

func TestProxyHeadersRejectBareCRLFValues(t *testing.T) {
	t.Parallel()

	_, err := proxyHeaders(context.Background(), http.Header{"X-Test": []string{"bad\rvalue"}})
	if err == nil {
		t.Fatal("expected malformed header value error")
	}
}

func TestProxyHeadersFromRawPreserveOrderAndStrip(t *testing.T) {
	t.Parallel()

	raw := []byte("GET http://example.com/ HTTP/1.1\r\n" +
		"X-First: 1\r\n" +
		"Connection: X-Hop\r\n" +
		"X-Hop: drop\r\n" +
		"X-Second: 2\r\n" +
		"X-First: 3\r\n" +
		"Proxy-Authorization: Bearer secret\r\n" +
		"\r\n")

	headers, err := proxyHeadersFromRaw(raw)
	if err != nil {
		t.Fatalf("proxyHeadersFromRaw() error = %v", err)
	}

	got := decodedProxyHeaderPairs(headers)
	want := []string{"X-First=1", "X-Second=2", "X-First=3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("headers = %#v, want %#v", got, want)
	}
}

func TestProxyHandlerRouteNoMatchUses421(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestProxyHandler(t)
	dispatcher.err = &PipelineError{Code: RouteNoMatch}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMisdirectedRequest)
	}
}

func newTestProxyHandler(t *testing.T) (*ProxyHandler, string, *captureProxyDispatcher) {
	t.Helper()

	return newTestProxyHandlerWithKey(t, RoleRequester, APIKeyStatusActive)
}

func newTestProxyHandlerWithKey(t *testing.T, role Role, status APIKeyStatus) (*ProxyHandler, string, *captureProxyDispatcher) {
	t.Helper()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("test-pepper")

	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	err = store.Create(context.Background(), APIKeyRecord{
		ID:         "key_proxy_requester",
		ScopeType:  ScopeTenant,
		TenantID:   "ten_proxy",
		Role:       role,
		Prefix:     generated.Prefix,
		SecretHash: HashAPIKeySecret(generated.Secret, pepper),
		Status:     status,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("store.Create() error = %v", err)
	}

	dispatcher := &captureProxyDispatcher{}
	h := NewProxyHandler(1_048_576, 1_048_576, 120_000, NewAuthenticator(store, pepper))
	h.SetDispatcher(dispatcher)

	return h, generated.Secret, dispatcher
}

type captureProxyDispatcher struct {
	last  DispatchInput
	calls int
	err   *PipelineError
}

func (d *captureProxyDispatcher) Dispatch(_ context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	d.calls++
	d.last = in
	if d.err != nil {
		return SuccessResponse{}, d.err
	}

	return SuccessResponse{
		RequestID: in.RequestID,
		Status:    http.StatusNotFound,
		Headers: []HeaderPair{{
			Name:  headerCanonicalContentType,
			Value: base64.StdEncoding.EncodeToString([]byte(mediaTypeTextPlain)),
		}},
		Body: ResponseBody{
			Mode:       handlerTestInlineBase64,
			DataBase64: base64.StdEncoding.EncodeToString([]byte("missing")),
		},
		ResponseSizeBytes: 7,
	}, nil
}

type rawProxyDispatcher struct {
	last    DispatchInput
	calls   int
	status  int
	headers http.Header
	chunks  [][]byte
	err     *PipelineError
}

func (d *rawProxyDispatcher) Dispatch(_ context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	d.last = in

	return SuccessResponse{}, &PipelineError{Code: ControlInternalError}
}

func (d *rawProxyDispatcher) DispatchRaw(_ context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	d.calls++
	d.last = in

	for name, values := range d.headers {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	status := d.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)

	var size uint64
	for _, chunk := range d.chunks {
		n, _ := w.Write(chunk)
		size += uint64FromInt(n)
	}

	return SuccessResponse{RequestID: in.RequestID, Status: status, ResponseSizeBytes: size}, d.err, true
}

type blockingRawProxyDispatcher struct {
	started chan struct{}
	done    chan struct{}
}

func (d *blockingRawProxyDispatcher) Dispatch(context.Context, DispatchInput) (SuccessResponse, *PipelineError) {
	return SuccessResponse{}, &PipelineError{Code: ControlInternalError}
}

func (d *blockingRawProxyDispatcher) DispatchRaw(ctx context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	w.WriteHeader(http.StatusOK)
	close(d.started)
	<-ctx.Done()
	close(d.done)

	return SuccessResponse{RequestID: in.RequestID, Status: http.StatusOK}, &PipelineError{Code: Cancelled}, true
}

func decodedProxyHeaders(headers []HeaderPair) map[string]string {
	out := map[string]string{}
	for _, h := range headers {
		value, _ := base64.StdEncoding.DecodeString(h.Value)
		out[h.Name] = string(value)
	}

	return out
}

func decodedProxyHeaderPairs(headers []HeaderPair) []string {
	out := make([]string, 0, len(headers))
	for _, h := range headers {
		value, _ := base64.StdEncoding.DecodeString(h.Value)
		out = append(out, h.Name+"="+string(value))
	}

	return out
}
