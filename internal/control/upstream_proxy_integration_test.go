package control

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	internalegress "github.com/beremaran/straw-oss/internal/egress"
	"github.com/beremaran/straw-oss/internal/natsx"
	"github.com/beremaran/straw-oss/internal/testutil"
)

const (
	integrationProxyPoolID    = "integration-proxy-pool"
	integrationProxyProfileID = "integration-proxy-profile"
	integrationProxyWorkerID  = "integration-proxy-worker"
	integrationProxySessionID = "integration-proxy-session"
)

func TestUpstreamProxyControlToOfficialEgressIntegration(t *testing.T) {
	t.Parallel()

	destinationProxyAuthorization := ""
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationProxyAuthorization = r.Header.Get("Proxy-Authorization")
		_, _ = w.Write([]byte("through-integrated-proxy"))
	}))
	defer destination.Close()

	proxyURL, connects, closeProxy := startIntegrationConnectProxy(t, destination.Listener.Addr().String())
	defer closeProxy()

	profiles, err := internalegress.ResolveUpstreamProxyProfiles([]internalegress.UpstreamProxyConfig{{
		ID: integrationProxyProfileID, Endpoint: proxyURL,
		Auth: internalegress.UpstreamProxyAuthConfig{Type: "none"},
	}})
	if err != nil {
		t.Fatalf("resolve profiles: %v", err)
	}
	executor := internalegress.NewExecutor(internalegress.ExecutorOptions{
		UpstreamProxyProfiles: profiles,
		UpstreamProxyPools:    map[string]string{integrationProxyPoolID: integrationProxyProfileID},
	})

	server := testutil.NewFakeNATSServer(t, 1<<20)
	conn, err := natsx.Connect(natsx.ConnectOptions{Servers: []string{server.URL()}})
	if err != nil {
		t.Fatalf("connect fake NATS: %v", err)
	}
	defer conn.Close()

	worker, err := internalegress.NewWorker(conn, internalegress.Identity{WorkerID: integrationProxyWorkerID}, executor, integrationProxySessionID, 1)
	if err != nil {
		t.Fatalf("new official worker: %v", err)
	}
	stopWorker := make(chan struct{})
	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Serve(stopWorker) }()
	t.Cleanup(func() {
		close(stopWorker)
		serveErr := <-workerDone
		if serveErr != nil {
			t.Errorf("worker serve: %v", serveErr)
		}
	})
	time.Sleep(10 * time.Millisecond)

	assignSubject, err := natsx.AssignmentSubject(integrationProxyWorkerID, integrationProxySessionID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := PoolCandidate{
		WorkerID: integrationProxyWorkerID, SessionID: integrationProxySessionID, AssignSubject: assignSubject,
		ExecutorType: errorCategoryEgress, IngressModes: []string{IngressTypeREST}, MaxConcurrency: 1, AvailableCap: 1,
		ProtocolMinor: 2, SupportedProtocolMinor: 2, UpstreamProxyID: integrationProxyProfileID,
	}
	snapshot := config.NewSnapshot(1)
	snapshot.DefaultTimeoutMs = 2000
	snapshot.ExecutorPools = []config.ExecutorPool{{
		ID: integrationProxyPoolID, ExecutorType: errorCategoryEgress, Enabled: true,
		UpstreamProxy: &config.ExecutorPoolUpstreamProxy{ID: integrationProxyProfileID, TrustedRemoteResolution: true},
	}}
	snapshot.RoutingRules = []config.RoutingRule{{ID: "integration-proxy-route", Enabled: true, TargetPoolID: integrationProxyPoolID}}
	dispatcher := NewDefaultRequestDispatcher(RequestDispatcherOptions{
		ConfigCache: NewConfigCache(snapshot), Workers: testRoutingCandidates{integrationProxyPoolID: {candidate}}, NATS: conn,
		MaxInlineResponseBodyBytes: 1024, FrameIdleTimeout: time.Second,
	})

	destinationURL, err := url.Parse(destination.URL)
	if err != nil {
		t.Fatal(err)
	}
	targetURL, err := url.Parse("http://integration.test:" + destinationURL.Port() + "/products")
	if err != nil {
		t.Fatal(err)
	}
	response, perr := dispatcher.Dispatch(context.Background(), DispatchInput{
		RequestID: "integration-proxy-request", Identity: Identity{DeploymentID: config.DefaultDeploymentID},
		Request: &ValidatedRequest{Method: http.MethodGet, URL: targetURL, IngressType: IngressTypeREST},
	})
	if perr != nil {
		t.Fatalf("dispatch: %+v", perr)
	}
	body, err := base64.StdEncoding.DecodeString(response.Body.DataBase64)
	if err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if response.Status != http.StatusOK || string(body) != "through-integrated-proxy" {
		t.Fatalf("response = %+v", response)
	}
	if destinationProxyAuthorization != "" {
		t.Fatalf("destination received Proxy-Authorization %q", destinationProxyAuthorization)
	}
	select {
	case authority := <-connects:
		if !strings.HasPrefix(authority, "integration.test:") {
			t.Fatalf("CONNECT authority = %q", authority)
		}
	case <-time.After(time.Second):
		t.Fatal("proxy did not receive CONNECT")
	}
}

func startIntegrationConnectProxy(t *testing.T, destination string) (string, <-chan string, func()) {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy: %v", err)
	}
	connects := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		client, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = client.Close() }()

		request, readErr := http.ReadRequest(bufio.NewReader(client))
		if readErr != nil || request.Method != http.MethodConnect {
			return
		}
		connects <- request.RequestURI

		upstream, dialErr := (&net.Dialer{}).DialContext(context.Background(), "tcp", destination)
		if dialErr != nil {
			return
		}
		defer func() { _ = upstream.Close() }()

		_, _ = fmt.Fprint(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		copyDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(upstream, client)
			close(copyDone)
		}()
		_, _ = io.Copy(client, upstream)
		<-copyDone
	}()

	closeProxy := func() {
		_ = listener.Close()
		<-done
	}

	return "http://" + listener.Addr().String(), connects, closeProxy
}
