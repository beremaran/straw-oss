package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func TestProxyHandlerDispatchesAbsoluteFormHTTP(t *testing.T) {
	t.Parallel()

	dispatcher := &proxyDispatcherStub{}
	handler := NewProxyHandler(1024, NewDeploymentAuthenticator("secret"), dispatcher, dispatcher)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/path?q=1", bytes.NewBufferString("payload"))
	req.Header.Set("Proxy-Authorization", "Bearer secret")
	req.Header.Set("Authorization", "Bearer destination-token")
	req.Header.Set("X-Straw-Route-Tags", "datacenter, edge")
	req.Header.Set("X-Straw-Route-Country", "au")
	req.Header.Set("X-Straw-Route-Region", "ap-southeast-2")
	req.Header.Set("X-Straw-Route-IP-Type", routingIPType)
	req.Header.Set("X-Straw-Route-Sticky-Session", "checkout-42")
	req.Header.Set("X-Straw-Future-Control", "must-not-forward")
	req.Header.Set("Connection", "X-Hop")
	req.Header.Set("X-Hop", "remove-me")
	req.Header.Add("X-Keep", "one")
	req.Header.Add("X-Keep", "two")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated || rec.Body.String() != "proxied" {
		t.Fatalf("response = %d %q, want 201 proxied", rec.Code, rec.Body.String())
	}
	if dispatcher.raw.Request.IngressType != IngressTypeHTTPProxy || dispatcher.raw.Request.URL.String() != "http://example.com/path?q=1" {
		t.Fatalf("dispatch request = %#v", dispatcher.raw.Request)
	}
	if got := dispatcher.raw.Request.Routing; got.Country != "AU" || got.Region != "ap-southeast-2" || got.IPType != routingIPType || got.StickySessionID != "checkout-42" || len(got.Tags) != 2 || got.Tags[1] != "edge" {
		t.Fatalf("routing hints = %+v", got)
	}
	if string(dispatcher.raw.Request.BodyData) != "payload" {
		t.Fatalf("request body = %q, want payload", dispatcher.raw.Request.BodyData)
	}
	if got := decodedHeaderValues(dispatcher.raw.Request.Headers, "Authorization"); len(got) != 1 || got[0] != "Bearer destination-token" {
		t.Fatalf("destination Authorization = %v", got)
	}
	if got := decodedHeaderValues(dispatcher.raw.Request.Headers, "X-Keep"); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("X-Keep = %v", got)
	}
	for _, name := range []string{"Proxy-Authorization", "Connection", "X-Hop", "Content-Length", "X-Straw-Route-Tags", "X-Straw-Route-Country", "X-Straw-Route-Region", "X-Straw-Route-IP-Type", "X-Straw-Route-Sticky-Session", "X-Straw-Future-Control"} {
		if got := decodedHeaderValues(dispatcher.raw.Request.Headers, name); len(got) != 0 {
			t.Fatalf("stripped header %s = %v", name, got)
		}
	}
}

func TestProxyHandlerRejectsMalformedAndOversizedRoutingHintsAfterAuthentication(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{
		"too-many-tags":    strings.Repeat("tag,", maxRoutingTags),
		"too-long-region":  strings.Repeat("r", maxRoutingValueBytes+1),
		"bad-country":      "AUS",
		"duplicate-sticky": "first",
	} {
		t.Run(name, func(t *testing.T) {
			dispatcher := &proxyDispatcherStub{}
			handler := NewProxyHandler(1024, NewDeploymentAuthenticator("secret"), dispatcher, dispatcher)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
			req.Header.Set("Proxy-Authorization", "Bearer secret")
			switch name {
			case "too-many-tags":
				req.Header.Set("X-Straw-Route-Tags", value)
			case "duplicate-sticky":
				req.Header.Add("X-Straw-Route-Sticky-Session", value)
				req.Header.Add("X-Straw-Route-Sticky-Session", "second")
			default:
				req.Header.Set("X-Straw-Route-"+map[string]string{"too-long-region": "Region", "bad-country": "Country"}[name], value)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest || dispatcher.raw.Request != nil {
				t.Fatalf("response = %d, dispatched=%#v; want 400 without dispatch", rec.Code, dispatcher.raw.Request)
			}
		})
	}
}

func TestProxyHandlerRequiresProxyAuthorization(t *testing.T) {
	t.Parallel()

	dispatcher := &proxyDispatcherStub{}
	handler := NewProxyHandler(1024, NewDeploymentAuthenticator("secret"), dispatcher, dispatcher)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil))

	if rec.Code != http.StatusProxyAuthRequired || rec.Header().Get("Proxy-Authenticate") == "" {
		t.Fatalf("response = %d headers=%v, want proxy auth challenge", rec.Code, rec.Header())
	}
	if dispatcher.raw.Request != nil {
		t.Fatal("unauthorized request was dispatched")
	}
}

func TestProxyHandlerAuthenticatesBeforeParsingRoutingHints(t *testing.T) {
	t.Parallel()

	dispatcher := &proxyDispatcherStub{}
	handler := NewProxyHandler(1024, NewDeploymentAuthenticator("secret"), dispatcher, dispatcher)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
	req.Header.Set("X-Straw-Route-Country", "not-a-country")
	req.Header.Set("Proxy-Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusProxyAuthRequired || dispatcher.raw.Request != nil {
		t.Fatalf("response = %d, dispatched=%#v; want auth failure before hint validation", rec.Code, dispatcher.raw.Request)
	}
}

func TestProxyHandlerWrapSelectsIngressBeforeAPIPathRouting(t *testing.T) {
	t.Parallel()

	dispatcher := &proxyDispatcherStub{}
	proxy := NewProxyHandler(1024, NewDeploymentAuthenticator(""), dispatcher, dispatcher)
	apiCalled := false
	wrapped := proxy.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://destination.test/api/v1/requests", nil)

	wrapped.ServeHTTP(rec, req)

	if apiCalled || dispatcher.raw.Request == nil {
		t.Fatalf("api_called=%v proxy_request=%#v", apiCalled, dispatcher.raw.Request)
	}
}

func TestProxyHandlerEstablishesConnectTunnel(t *testing.T) {
	t.Parallel()

	server, client := net.Pipe()
	defer func() { _ = client.Close() }()

	dispatcher := &proxyDispatcherStub{tunnelFn: func(_ context.Context, _ DispatchInput, rw io.ReadWriter) (SuccessResponse, *PipelineError) {
		buf := make([]byte, 4)

		_, err := io.ReadFull(rw, buf)
		if err != nil {
			t.Errorf("read tunneled request: %v", err)

			return SuccessResponse{}, &PipelineError{Code: Cancelled}
		}

		_, err = rw.Write([]byte("pong"))
		if err != nil {
			t.Errorf("write tunneled response: %v", err)
		}

		return SuccessResponse{Status: http.StatusOK}, nil
	}}
	handler := NewProxyHandler(1024, NewDeploymentAuthenticator("secret"), dispatcher, dispatcher)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://"+proxyTestAuthority, nil)
	req.Host = proxyTestAuthority
	req.Header.Set("Proxy-Authorization", "Bearer secret")
	req.Header.Set("X-Straw-Route-Country", "au")
	req.Header.Set("X-Straw-Route-Tags", "datacenter")

	_, verr := handler.validateConnectRequest(req)
	if verr != nil {
		t.Fatalf("validateConnectRequest() error = %v (URL=%#v Host=%q)", verr, req.URL, req.Host)
	}
	w := &hijackResponseWriter{conn: server, header: make(http.Header)}
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(w, req)
		close(done)
	}()

	_, err := client.Write([]byte("ping"))
	if err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)

	_, err = io.ReadFull(client, response)
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "pong" {
		t.Fatalf("tunnel response = %q, want pong", response)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CONNECT handler did not return")
	}
	if dispatcher.tunnel.Request.IngressType != IngressTypeConnect || dispatcher.tunnel.Request.URL.String() != "connect://example.com:443" {
		t.Fatalf("tunnel request = %#v", dispatcher.tunnel.Request)
	}
	if len(dispatcher.tunnel.Request.Headers) != 0 || dispatcher.tunnel.Request.Fingerprint != "" {
		t.Fatalf("CONNECT forwarded decoded controls: headers=%v fingerprint=%q", dispatcher.tunnel.Request.Headers, dispatcher.tunnel.Request.Fingerprint)
	}
	if dispatcher.tunnel.Request.Routing.Country != "AU" || len(dispatcher.tunnel.Request.Routing.Tags) != 1 {
		t.Fatalf("tunnel routing hints = %+v", dispatcher.tunnel.Request.Routing)
	}
}

func TestTunnelStreamWritesHandshakeAndCreditsDeliveredBytes(t *testing.T) {
	t.Parallel()

	dispatcher := &DefaultRequestDispatcher{opts: RequestDispatcherOptions{Now: time.Now}}
	var client bytes.Buffer
	state := tunnelStreamState{
		dispatcher: dispatcher,
		validator:  natsx.NewStreamValidator(defaultRequestAttempt, 16, time.Second, time.Now),
		rw:         &client,
	}

	frames := []*strawpb.StreamFrame{
		{StreamSeq: 1, Attempt: 1, Payload: &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{TargetHost: "example.com", TargetPort: 443}}},
		{StreamSeq: 2, Attempt: 1, Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusOK}}},
		{StreamSeq: 3, Attempt: 1, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 0, Data: []byte("hello")}}},
		{StreamSeq: 4, Attempt: 1, Payload: &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}}},
	}

	for i, frame := range frames {
		done, perr := state.accept(frame)
		if perr != nil {
			t.Fatalf("frame %d error = %v", i, perr)
		}
		if done != (i == len(frames)-1) {
			t.Fatalf("frame %d done = %v", i, done)
		}
	}

	if got := client.String(); got != "HTTP/1.1 200 Connection Established\r\n\r\nhello" {
		t.Fatalf("client bytes = %q", got)
	}
	if state.result.status != http.StatusOK || state.result.size != 5 || state.validator.RemainingCredit() != 16 {
		t.Fatalf("result=%+v remaining_credit=%d", state.result, state.validator.RemainingCredit())
	}
}

func TestRawProxyResponseStripsHopByHopHeaders(t *testing.T) {
	t.Parallel()

	headers := []*strawpb.Header{
		{Name: "Connection", Value: []byte("X-Hop")},
		{Name: "X-Hop", Value: []byte("remove-me")},
		{Name: "Proxy-Authenticate", Value: []byte("Basic")},
		{Name: "X-Keep", Value: []byte("ok")},
	}
	connectionHeaders := responseConnectionHeaders(headers)
	if rawResponseHeaderAllowed("Connection", connectionHeaders) || rawResponseHeaderAllowed("X-Hop", connectionHeaders) || rawResponseHeaderAllowed("Proxy-Authenticate", connectionHeaders) {
		t.Fatal("hop-by-hop response header was allowed")
	}
	if !rawResponseHeaderAllowed("X-Keep", connectionHeaders) {
		t.Fatal("end-to-end response header was rejected")
	}
}

type proxyDispatcherStub struct {
	raw      DispatchInput
	tunnel   DispatchInput
	tunnelFn func(context.Context, DispatchInput, io.ReadWriter) (SuccessResponse, *PipelineError)
}

func (s *proxyDispatcherStub) DispatchRaw(_ context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	s.raw = in
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte("proxied"))

	return SuccessResponse{Status: http.StatusCreated}, nil, true
}

func (s *proxyDispatcherStub) DispatchTunnel(ctx context.Context, in DispatchInput, rw io.ReadWriter) (SuccessResponse, *PipelineError) {
	s.tunnel = in
	if s.tunnelFn != nil {
		return s.tunnelFn(ctx, in, rw)
	}

	return SuccessResponse{Status: http.StatusOK}, nil
}

type hijackResponseWriter struct {
	conn   net.Conn
	header http.Header
}

func (w *hijackResponseWriter) Header() http.Header         { return w.header }
func (w *hijackResponseWriter) WriteHeader(_ int)           {}
func (w *hijackResponseWriter) Write(p []byte) (int, error) { return w.conn.Write(p) }
func (w *hijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

func decodedHeaderValues(headers []HeaderPair, name string) []string {
	var values []string
	for _, header := range headers {
		if !strings.EqualFold(header.Name, name) {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(header.Value)
		if err == nil {
			values = append(values, string(decoded))
		}
	}

	return values
}
