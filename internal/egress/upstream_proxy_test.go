package egress

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	testUpstreamProxyID       = "proxy-profile"
	testUpstreamProxyPool     = "proxy-pool"
	testUpstreamProxyUsername = "account"
)

func TestResolveUpstreamProxyProfilesRendersProviderStyles(t *testing.T) {
	t.Setenv("TEST_PROXY_USERNAME", testUpstreamProxyUsername)
	t.Setenv("TEST_PROXY_PASSWORD", "secret")

	instruction := &strawpb.UpstreamProxyInstruction{
		ProviderSessionId: upstreamProxySentinelSession,
		Country:           "NZ",
	}
	tests := map[string]struct {
		template string
		want     string
	}{
		"bright-data": {
			template: "{{.Username}}{{if .Country}}-country-{{lower .Country}}{{end}}{{if .Session}}-session-{{.Session}}{{end}}",
			want:     "account-country-nz-session-0123456789abcdef0123456789abcdef",
		},
		"oxylabs": {
			template: "{{.Username}}{{if .Country}}-cc-{{upper .Country}}{{end}}{{if .Session}}-sessid-{{.Session}}{{end}}",
			want:     "account-cc-NZ-sessid-0123456789abcdef0123456789abcdef",
		},
		"apify": {
			template: "{{.Username}}{{if .Session}},session-{{.Session}}{{end}}{{if .Country}},country-{{upper .Country}}{{end}}",
			want:     "account,session-0123456789abcdef0123456789abcdef,country-NZ",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			profiles, err := ResolveUpstreamProxyProfiles([]UpstreamProxyConfig{{
				ID:       testUpstreamProxyID,
				Endpoint: "http://proxy.test:8080",
				Auth: UpstreamProxyAuthConfig{
					Type:             upstreamProxyAuthBasic,
					UsernameEnv:      "TEST_PROXY_USERNAME",
					PasswordEnv:      "TEST_PROXY_PASSWORD",
					UsernameTemplate: test.template,
				},
				Defaults: UpstreamProxyDefaults{Country: "AU", Region: "nsw", IPType: "residential"},
			}})
			if err != nil {
				t.Fatalf("ResolveUpstreamProxyProfiles() error = %v", err)
			}

			got, err := profiles[testUpstreamProxyID].renderUsername(instruction)
			if err != nil {
				t.Fatalf("renderUsername() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("rendered username = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveUpstreamProxyProfilesOptionalFieldsAndEmptyPassword(t *testing.T) {
	t.Setenv("TEST_PROXY_USERNAME", testUpstreamProxyUsername)
	t.Setenv("TEST_PROXY_EMPTY_PASSWORD", "")

	profiles, err := ResolveUpstreamProxyProfiles([]UpstreamProxyConfig{{
		ID:       testUpstreamProxyID,
		Endpoint: "https://proxy.test:443",
		Auth: UpstreamProxyAuthConfig{
			Type:             upstreamProxyAuthBasic,
			UsernameEnv:      "TEST_PROXY_USERNAME",
			PasswordEnv:      "TEST_PROXY_EMPTY_PASSWORD",
			UsernameTemplate: "{{.Username}}{{if .Session}}-session-{{.Session}}{{end}}{{if .Region}}-region-{{lower .Region}}{{end}}",
		},
	}})
	if err != nil {
		t.Fatalf("ResolveUpstreamProxyProfiles() error = %v", err)
	}

	profile := profiles[testUpstreamProxyID]
	username, err := profile.renderUsername(&strawpb.UpstreamProxyInstruction{})
	if err != nil {
		t.Fatalf("renderUsername() error = %v", err)
	}
	if username != testUpstreamProxyUsername {
		t.Fatalf("rendered username = %q, want optional segments omitted", username)
	}
	authorization, err := profile.proxyAuthorization(&strawpb.UpstreamProxyInstruction{})
	if err != nil {
		t.Fatalf("proxyAuthorization() error = %v", err)
	}
	if authorization != "Basic "+base64.StdEncoding.EncodeToString([]byte("account:")) {
		t.Fatalf("authorization = %q, want Basic account with empty password", authorization)
	}
}

func TestResolveUpstreamProxyProfilesRejectsInvalidCredentialsAndTemplates(t *testing.T) {
	const missingEnvironment = "TEST_PROXY_ENVIRONMENT_THAT_MUST_NOT_EXIST"
	previous, existed := os.LookupEnv(missingEnvironment)
	err := os.Unsetenv(missingEnvironment)
	if err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(missingEnvironment, previous)
		}
	})

	tests := map[string]struct {
		usernameEnv string
		username    string
		password    string
		template    string
	}{
		"missing-environment":  {usernameEnv: missingEnvironment, template: upstreamProxyDefaultUsernameTemplate},
		"invalid-template":     {username: testUpstreamProxyUsername, template: "{{"},
		"unsupported-function": {username: testUpstreamProxyUsername, template: "{{printf \"%s\" .Username}}"},
		"username-control":     {username: "account\r\nleak", template: upstreamProxyDefaultUsernameTemplate},
		"password-control":     {username: testUpstreamProxyUsername, password: "secret\r\nleak", template: upstreamProxyDefaultUsernameTemplate},
		"template-nul":         {username: testUpstreamProxyUsername, template: upstreamProxyDefaultUsernameTemplate + "\x00"},
		"username-colon":       {username: "account:zone", template: upstreamProxyDefaultUsernameTemplate},
		"rendered-colon":       {username: testUpstreamProxyUsername, template: upstreamProxyDefaultUsernameTemplate + ":zone"},
		"rendered-oversize":    {username: strings.Repeat("x", maxUpstreamProxyRenderedCredential+1), template: upstreamProxyDefaultUsernameTemplate},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			usernameEnv := test.usernameEnv
			if usernameEnv == "" {
				usernameEnv = "TEST_PROXY_INVALID_USERNAME"
				t.Setenv(usernameEnv, test.username)
			}
			passwordEnv := ""
			if test.password != "" {
				passwordEnv = "TEST_PROXY_INVALID_PASSWORD"
				t.Setenv(passwordEnv, test.password)
			}

			_, err := ResolveUpstreamProxyProfiles([]UpstreamProxyConfig{{
				ID:       testUpstreamProxyID,
				Endpoint: "http://proxy.test:8080",
				Auth: UpstreamProxyAuthConfig{
					Type:             upstreamProxyAuthBasic,
					UsernameEnv:      usernameEnv,
					PasswordEnv:      passwordEnv,
					UsernameTemplate: test.template,
				},
			}})
			if err == nil {
				t.Fatal("ResolveUpstreamProxyProfiles() error = nil, want rejection")
			}
			if strings.Contains(err.Error(), test.username) && test.username != "" {
				t.Fatalf("error leaked username value: %v", err)
			}
			if strings.Contains(err.Error(), test.password) && test.password != "" {
				t.Fatalf("error leaked password value: %v", err)
			}
		})
	}
}

func TestUpstreamProxyConnectorSendsCONNECTAndPreservesBufferedBytes(t *testing.T) {
	endpoint, requests, closeProxy := startStaticResponseProxy(t, "HTTP/1.1 200 Connection Established\r\nContent-Length: 0\r\n\r\nhello")
	defer closeProxy()
	profiles := resolveTestProfiles(t, endpoint, true)
	connector := newUpstreamProxyConnector(profiles, (&net.Dialer{}).DialContext, nil)
	instruction := testProxyInstruction()

	conn, failure := connector.Open(context.Background(), instruction, "destination.test", 8443)
	if failure != nil {
		t.Fatalf("Open() failure = %v", failure)
	}
	defer func() { _ = conn.Close() }()

	request := <-requests
	wantAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("account-country-au-session-0123456789abcdef0123456789abcdef:secret"))
	if request.authority != "destination.test:8443" || request.host != request.authority || request.authorization != wantAuthorization {
		t.Fatalf("CONNECT request = %+v, want authority/Host/auth for destination.test:8443", request)
	}

	buffer := make([]byte, 5)
	_, err := io.ReadFull(conn, buffer)
	if err != nil {
		t.Fatalf("read buffered tunnel bytes: %v", err)
	}
	if string(buffer) != "hello" {
		t.Fatalf("buffered bytes = %q, want hello", buffer)
	}
}

func TestUpstreamProxyConnectorMapsResponses(t *testing.T) {
	oversized := "HTTP/1.1 200 OK\r\nX-Oversized: " + strings.Repeat("x", upstreamProxyResponseHeaderBytes) + "\r\n\r\n"
	tests := map[string]struct {
		response string
		fact     string
		status   uint32
	}{
		"authentication": {response: "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n", fact: upstreamProxyAuthenticationFact, status: 407},
		"rejected":       {response: "HTTP/1.1 502 Bad Gateway\r\n\r\n", fact: upstreamProxyConnectRejectedFact, status: 502},
		"status-line":    {response: "HTTP/1.1 two Bad\r\n\r\n", fact: upstreamProxyProtocolErrorFact},
		"bare-lf":        {response: "HTTP/1.1 200 OK\n\n", fact: upstreamProxyProtocolErrorFact},
		"folded":         {response: "HTTP/1.1 200 OK\r\nX-Test: value\r\n folded\r\n\r\n", fact: upstreamProxyProtocolErrorFact},
		"body":           {response: "HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\nx", fact: upstreamProxyProtocolErrorFact},
		"transfer":       {response: "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n", fact: upstreamProxyProtocolErrorFact},
		"oversized":      {response: oversized, fact: upstreamProxyProtocolErrorFact},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			endpoint, _, closeProxy := startStaticResponseProxy(t, test.response)
			defer closeProxy()
			connector := newUpstreamProxyConnector(resolveTestProfiles(t, endpoint, false), (&net.Dialer{}).DialContext, nil)

			conn, failure := connector.Open(context.Background(), testProxyInstruction(), "destination.test", 443)
			if conn != nil {
				_ = conn.Close()
				t.Fatal("Open() returned a connection for a rejected response")
			}
			if failure == nil || failure.code != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE || failure.fact != test.fact {
				t.Fatalf("Open() failure = %#v, want upstream_proxy_failure/%s", failure, test.fact)
			}
			if test.status == 0 && failure.upstreamStatus != nil {
				t.Fatalf("upstream status = %v, want absent", *failure.upstreamStatus)
			}
			if test.status != 0 && (failure.upstreamStatus == nil || *failure.upstreamStatus != test.status) {
				t.Fatalf("upstream status = %v, want %d", failure.upstreamStatus, test.status)
			}
		})
	}
}

func TestUpstreamProxyConnectorCancellationClosesSocket(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	requestRead := make(chan struct{})
	socketClosed := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		reader := bufio.NewReader(conn)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}
		close(requestRead)
		_, _ = reader.ReadByte()
		close(socketClosed)
	}()

	profiles := resolveTestProfiles(t, "http://"+listener.Addr().String(), false)
	connector := newUpstreamProxyConnector(profiles, (&net.Dialer{}).DialContext, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *executionError, 1)
	go func() {
		_, failure := connector.Open(ctx, testProxyInstruction(), "destination.test", 443)
		done <- failure
	}()
	select {
	case <-requestRead:
	case <-time.After(time.Second):
		t.Fatal("proxy did not receive CONNECT request")
	}
	cancel()

	select {
	case failure := <-done:
		if failure == nil || failure.code != strawpb.ErrorCode_ERROR_CODE_CANCELLED {
			t.Fatalf("cancellation failure = %#v, want cancelled", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("connector did not return after cancellation")
	}
	select {
	case <-socketClosed:
	case <-time.After(time.Second):
		t.Fatal("connector cancellation did not close proxy socket")
	}
}

func TestUpstreamProxyConnectorMapsDeadlineAndDialFailure(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		client, server := net.Pipe()
		defer func() { _ = server.Close() }()
		requestRead := make(chan struct{})
		go func() {
			request, err := http.ReadRequest(bufio.NewReader(server))
			if err == nil {
				_ = request.Body.Close()
				close(requestRead)
			}
		}()

		connector := newUpstreamProxyConnector(resolveTestProfiles(t, "http://proxy.test:8080", false), func(context.Context, string, string) (net.Conn, error) {
			return client, nil
		}, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, failure := connector.Open(ctx, testProxyInstruction(), "destination.test", 443)
		if failure == nil || failure.code != strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED || failure.timeoutType != strawpb.TimeoutType_TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT {
			t.Fatalf("deadline failure = %#v, want total timeout", failure)
		}
		select {
		case <-requestRead:
		default:
			t.Fatal("proxy did not receive CONNECT before deadline")
		}
	})

	t.Run("dial", func(t *testing.T) {
		var address string
		connector := newUpstreamProxyConnector(resolveTestProfiles(t, "http://configured.proxy:3128", false), func(_ context.Context, _, got string) (net.Conn, error) {
			address = got

			return nil, errors.New("dial failed")
		}, nil)
		_, failure := connector.Open(context.Background(), testProxyInstruction(), "destination.test", 443)
		if failure == nil || failure.code != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE || failure.fact != upstreamProxyConnectFailedFact {
			t.Fatalf("dial failure = %#v, want upstream proxy connect failure", failure)
		}
		if address != "configured.proxy:3128" {
			t.Fatalf("dial address = %q, want configured endpoint only", address)
		}
	})
}

func TestUpstreamProxyConnectorUsesStrictOuterTLS(t *testing.T) {
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	proxy.Config.ErrorLog = log.New(io.Discard, "", 0)
	proxy.StartTLS()
	defer proxy.Close()

	connector := newUpstreamProxyConnector(resolveTestProfiles(t, proxy.URL, false), (&net.Dialer{}).DialContext, nil)
	_, failure := connector.Open(context.Background(), testProxyInstruction(), "destination.test", 443)
	if failure == nil || failure.code != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE || failure.fact != upstreamProxyTLSFailedFact {
		t.Fatalf("Open() failure = %#v, want strict outer TLS failure", failure)
	}
}

func TestExecutorRoutesThroughValidatedTLSProxyEndpoint(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("tls-proxy-endpoint"))
	}))
	defer destination.Close()
	proxy, observations := startForwardingConnectProxy(t, destination.Listener.Addr().String(), true)
	defer proxy.Close()
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate())
	executor := newProxyExecutor(t, proxy.URL, &countingResolver{}, ExecutorOptions{RootCAs: roots})

	frames := executor.Execute(context.Background(), remoteRequestStart(rewriteHost(t, destination.URL, "remote.test")), nil, 1, nil)
	if errFrame := terminalErrorOrNil(frames); errFrame != nil {
		t.Fatalf("TLS proxy endpoint request error = %#v", errFrame)
	}
	if len(observations) != 1 {
		t.Fatalf("CONNECT count = %d, want 1", len(observations))
	}
}

func TestExecutorRoutesBaselineHTTPThroughUpstreamProxy(t *testing.T) {
	var destinationProxyAuthorization string
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationProxyAuthorization = r.Header.Get("Proxy-Authorization")
		_, _ = w.Write([]byte("through-proxy"))
	}))
	defer destination.Close()

	proxy, observations := startForwardingConnectProxy(t, destination.Listener.Addr().String(), false)
	defer proxy.Close()
	resolver := &countingResolver{}
	executor := newProxyExecutor(t, proxy.URL, resolver, ExecutorOptions{
		Pool: UpstreamConnectionPoolOptions{Enabled: true},
	})
	start := remoteRequestStart(rewriteHost(t, destination.URL, "remote.test"))

	frames := executor.Execute(context.Background(), start, nil, 1, nil)
	if errFrame := terminalErrorOrNil(frames); errFrame != nil {
		t.Fatalf("Execute() error = %#v", errFrame)
	}
	if got := frames[0].GetOutboundStart().GetUpstreamProxyId(); got != testUpstreamProxyID {
		t.Fatalf("outbound upstream proxy id = %q, want %q", got, testUpstreamProxyID)
	}
	if got := string(frames[2].GetData().GetData()); got != "through-proxy" {
		t.Fatalf("response body = %q, want through-proxy", got)
	}
	if destinationProxyAuthorization != "" {
		t.Fatalf("destination received Proxy-Authorization %q", destinationProxyAuthorization)
	}
	if resolver.lookups.Load() != 0 {
		t.Fatalf("remote hostname resolver lookups = %d, want 0", resolver.lookups.Load())
	}

	observation := <-observations
	wantAuthority := authorityFromURL(t, start.GetUrl())
	wantAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("account-country-au-session-0123456789abcdef0123456789abcdef:secret"))
	if observation.authority != wantAuthority || observation.authorization != wantAuthorization {
		t.Fatalf("proxy observation = %+v, want authority %q and generated auth", observation, wantAuthority)
	}
}

func TestExecutorRoutesHTTP2AndNamedUTLSThroughUpstreamProxy(t *testing.T) {
	destination := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(xProtoHeader, r.Proto)
		_, _ = w.Write([]byte("tls-proxy"))
	}))
	destination.EnableHTTP2 = true
	destination.StartTLS()
	defer destination.Close()

	proxy, observations := startForwardingConnectProxy(t, destination.Listener.Addr().String(), false)
	defer proxy.Close()
	roots := x509.NewCertPool()
	roots.AddCert(destination.Certificate())
	executor := newProxyExecutor(t, proxy.URL, &countingResolver{}, ExecutorOptions{HTTP2Enabled: true, RootCAs: roots})
	targetURL := rewriteHost(t, destination.URL, "example.com")

	baseline := remoteRequestStart(targetURL)
	frames := executor.Execute(context.Background(), baseline, nil, 1, nil)
	if errFrame := terminalErrorOrNil(frames); errFrame != nil {
		t.Fatalf("HTTP/2 proxy request error = %#v", errFrame)
	}
	if got := responseHeaderValue(frames[1].GetResponseStart(), xProtoHeader); got != httpProtocol20 {
		t.Fatalf("baseline target protocol = %q, want HTTP/2.0", got)
	}

	profiled := remoteRequestStart(targetURL)
	profiled.FingerprintInstruction = chrome120FingerprintProfile
	frames = executor.Execute(context.Background(), profiled, nil, 1, nil)
	if errFrame := terminalErrorOrNil(frames); errFrame != nil {
		t.Fatalf("named uTLS proxy request error = %#v", errFrame)
	}
	if got := frames[0].GetOutboundStart().GetExecutedFingerprintProfile(); got != chrome120FingerprintProfile {
		t.Fatalf("executed fingerprint = %q, want %q", got, chrome120FingerprintProfile)
	}
	if len(observations) != 2 {
		t.Fatalf("CONNECT count = %d, want 2", len(observations))
	}
}

func TestExecutorRemoteRequestsBypassPoolAndOpenSeparateCONNECTs(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer destination.Close()
	proxy, observations := startForwardingConnectProxy(t, destination.Listener.Addr().String(), false)
	defer proxy.Close()

	executor := newProxyExecutor(t, proxy.URL, &countingResolver{}, ExecutorOptions{
		Pool: UpstreamConnectionPoolOptions{Enabled: true, MaxIdleConnsPerHost: 2, IdleTimeout: time.Minute, MaxLifetime: time.Minute},
	})
	start := remoteRequestStart(rewriteHost(t, destination.URL, "sticky.test"))
	for range 2 {
		if errFrame := terminalErrorOrNil(executor.Execute(context.Background(), start, nil, 1, nil)); errFrame != nil {
			t.Fatalf("Execute() error = %#v", errFrame)
		}
	}
	if len(observations) != 2 {
		t.Fatalf("CONNECT count = %d, want one per request", len(observations))
	}
	if len(executor.pool.transports) != 0 {
		t.Fatalf("application pool transports = %d, want no proxy transports", len(executor.pool.transports))
	}
}

func TestExecutorRoutesRawTunnelThroughUpstreamProxy(t *testing.T) {
	destination, closeDestination := startEchoServer(t)
	defer closeDestination()
	proxy, observations := startForwardingConnectProxy(t, destination, false)
	defer proxy.Close()
	port := portFromAddress(t, destination)
	executor := newProxyExecutor(t, proxy.URL, &countingResolver{}, ExecutorOptions{})
	start := remoteTunnelStart("connect://opaque.test:" + port)

	conn, target, failure := (tunnelAdapter{executor: executor}).OpenTunnel(context.Background(), start)
	if failure != nil {
		t.Fatalf("OpenTunnel() failure = %#v", failure)
	}
	defer func() { _ = conn.Close() }()
	if target.Host != "opaque.test" || target.UpstreamProxyID != testUpstreamProxyID {
		t.Fatalf("raw target = %+v", target)
	}
	_, err := conn.Write([]byte("opaque"))
	if err != nil {
		t.Fatalf("raw tunnel write: %v", err)
	}

	buffer := make([]byte, len("opaque"))
	_, err = io.ReadFull(conn, buffer)
	if err != nil {
		t.Fatalf("raw tunnel read: %v", err)
	}
	if string(buffer) != "opaque" {
		t.Fatalf("raw tunnel bytes = %q, want opaque", buffer)
	}
	if len(observations) != 1 {
		t.Fatalf("CONNECT count = %d, want 1", len(observations))
	}
}

func TestExecutorRejectsRemoteLiteralBeforeProxyDial(t *testing.T) {
	var dials atomic.Int32
	profiles := resolveTestProfiles(t, "http://proxy.invalid:8080", false)
	executor := NewExecutor(ExecutorOptions{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)

			return nil, errors.New("unexpected dial")
		},
		UpstreamProxyProfiles: profiles,
		UpstreamProxyPools:    map[string]string{testUpstreamProxyPool: testUpstreamProxyID},
	})
	start := remoteRequestStart("http://169.254.169.254/latest/meta-data")

	frames := executor.Execute(context.Background(), start, nil, 1, nil)
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
		t.Fatalf("error code = %v, want destination_denied", got)
	}
	if dials.Load() != 0 {
		t.Fatalf("proxy dials = %d, want 0", dials.Load())
	}
	if frames[0].GetOutboundStart() != nil {
		t.Fatal("denied literal emitted OutboundStart")
	}
}

func TestExecutorRejectsPoolInstructionProfileMismatchesWithoutDial(t *testing.T) {
	profiles := resolveTestProfiles(t, "http://proxy.invalid:8080", false)
	tests := map[string]struct {
		pools    map[string]string
		profiles map[string]UpstreamProxyProfile
		start    *strawpb.RequestStart
	}{
		"proxy-pool-direct-mode": {
			pools:    map[string]string{testUpstreamProxyPool: testUpstreamProxyID},
			profiles: profiles,
			start: func() *strawpb.RequestStart {
				start := requestStart("http://unit.test", directPolicy(false))
				start.SelectedPoolId = testUpstreamProxyPool

				return start
			}(),
		},
		"remote-unbound-pool": {
			profiles: profiles,
			start:    remoteRequestStart("http://unit.test"),
		},
		"instruction-mismatch": {
			pools:    map[string]string{testUpstreamProxyPool: testUpstreamProxyID},
			profiles: profiles,
			start: func() *strawpb.RequestStart {
				start := remoteRequestStart("http://unit.test")
				start.UpstreamProxy.UpstreamProxyId = "other-profile"

				return start
			}(),
		},
		"missing-local-profile": {
			pools: map[string]string{testUpstreamProxyPool: testUpstreamProxyID},
			start: remoteRequestStart("http://unit.test"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var dials atomic.Int32
			executor := NewExecutor(ExecutorOptions{
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)

					return nil, errors.New("unexpected dial")
				},
				UpstreamProxyProfiles: test.profiles,
				UpstreamProxyPools:    test.pools,
			})

			frames := executor.Execute(context.Background(), test.start, nil, 1, nil)
			failure := terminalError(t, frames)
			if failure.GetCode() != strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR || failure.GetDetails()[errorFactDetailKey] != upstreamProxyInstructionInvalidFact {
				t.Fatalf("failure = %#v, want instruction-invalid executor error", failure)
			}
			if dials.Load() != 0 {
				t.Fatalf("dials = %d, want 0", dials.Load())
			}
			if frames[0].GetOutboundStart() != nil {
				t.Fatal("binding mismatch emitted OutboundStart")
			}
		})
	}
}

func TestExecutorEmitsUpstreamStatusAndDirectProxyIDIsEmpty(t *testing.T) {
	endpoint, _, closeProxy := startStaticResponseProxy(t, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
	defer closeProxy()
	executor := newProxyExecutor(t, endpoint, &countingResolver{}, ExecutorOptions{})
	frames := executor.Execute(context.Background(), remoteRequestStart("http://remote.test"), nil, 1, nil)
	failure := terminalError(t, frames)
	if failure.UpstreamStatus == nil || failure.GetUpstreamStatus() != 407 {
		t.Fatalf("upstream status = %v, want present 407", failure.UpstreamStatus)
	}

	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer destination.Close()
	direct := NewExecutor(ExecutorOptions{Resolver: staticResolver{"direct.test": loopbackIP(t, destination.URL)}})
	directFrames := direct.Execute(context.Background(), requestStart(rewriteHost(t, destination.URL, "direct.test"), directPolicy(true)), nil, 1, nil)
	if errFrame := terminalErrorOrNil(directFrames); errFrame != nil {
		t.Fatalf("direct request error = %#v", errFrame)
	}
	if got := directFrames[0].GetOutboundStart().GetUpstreamProxyId(); got != "" {
		t.Fatalf("direct outbound proxy id = %q, want empty", got)
	}
}

type countingResolver struct {
	lookups atomic.Int32
}

func (r *countingResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.lookups.Add(1)

	return nil, errors.New("remote target must not be resolved locally")
}

func (r *countingResolver) LookupCNAME(context.Context, string) ([]string, error) {
	r.lookups.Add(1)

	return nil, errors.New("remote target CNAME must not be resolved locally")
}

type connectRequestObservation struct {
	authority     string
	host          string
	authorization string
}

func startStaticResponseProxy(t *testing.T, response string) (string, <-chan connectRequestObservation, func()) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	requests := make(chan connectRequestObservation, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		request, readErr := http.ReadRequest(bufio.NewReader(conn))
		if readErr != nil {
			return
		}
		requests <- connectRequestObservation{authority: request.RequestURI, host: request.Host, authorization: request.Header.Get("Proxy-Authorization")}
		_, _ = io.WriteString(conn, response)
	}()

	return "http://" + listener.Addr().String(), requests, func() { _ = listener.Close() }
}

func startForwardingConnectProxy(t *testing.T, destination string, useTLS bool) (*httptest.Server, chan connectRequestObservation) {
	t.Helper()
	observations := make(chan connectRequestObservation, 16)
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		observations <- connectRequestObservation{authority: request.RequestURI, host: request.Host, authorization: request.Header.Get("Proxy-Authorization")}
		upstream, err := (&net.Dialer{}).DialContext(request.Context(), "tcp", destination)
		if err != nil {
			http.Error(w, "connect failed", http.StatusBadGateway)

			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			_ = upstream.Close()
			http.Error(w, "hijacking unavailable", http.StatusInternalServerError)

			return
		}
		client, buffered, err := hijacker.Hijack()
		if err != nil {
			_ = upstream.Close()

			return
		}

		_, err = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		if err != nil {
			_ = client.Close()
			_ = upstream.Close()

			return
		}

		err = buffered.Flush()
		if err != nil {
			_ = client.Close()
			_ = upstream.Close()

			return
		}
		go func() {
			_, _ = io.Copy(upstream, client)
			_ = upstream.Close()
		}()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
	}))
	proxy.Config.ErrorLog = log.New(io.Discard, "", 0)
	if useTLS {
		proxy.StartTLS()
	} else {
		proxy.Start()
	}

	return proxy, observations
}

func startEchoServer(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(conn, conn)
	}()

	return listener.Addr().String(), func() { _ = listener.Close() }
}

func resolveTestProfiles(t *testing.T, endpoint string, basic bool) map[string]UpstreamProxyProfile {
	t.Helper()
	auth := UpstreamProxyAuthConfig{Type: upstreamProxyAuthNone}
	if basic {
		t.Setenv("TEST_CONNECT_PROXY_USERNAME", testUpstreamProxyUsername)
		t.Setenv("TEST_CONNECT_PROXY_PASSWORD", "secret")
		auth = UpstreamProxyAuthConfig{
			Type:             upstreamProxyAuthBasic,
			UsernameEnv:      "TEST_CONNECT_PROXY_USERNAME",
			PasswordEnv:      "TEST_CONNECT_PROXY_PASSWORD",
			UsernameTemplate: "{{.Username}}-country-{{lower .Country}}-session-{{.Session}}",
		}
	}
	profiles, err := ResolveUpstreamProxyProfiles([]UpstreamProxyConfig{{ID: testUpstreamProxyID, Endpoint: endpoint, Auth: auth}})
	if err != nil {
		t.Fatalf("ResolveUpstreamProxyProfiles() error = %v", err)
	}

	return profiles
}

func newProxyExecutor(t *testing.T, endpoint string, resolver Resolver, additional ExecutorOptions) *Executor {
	t.Helper()
	additional.Resolver = resolver
	additional.UpstreamProxyProfiles = resolveTestProfiles(t, endpoint, true)
	additional.UpstreamProxyPools = map[string]string{testUpstreamProxyPool: testUpstreamProxyID}

	return NewExecutor(additional)
}

func remoteRequestStart(rawURL string) *strawpb.RequestStart {
	start := requestStart(rawURL, remotePolicy())
	start.SelectedPoolId = testUpstreamProxyPool
	start.UpstreamProxy = testProxyInstruction()

	return start
}

func remoteTunnelStart(rawURL string) *strawpb.RequestStart {
	start := tunnelStart(rawURL, remotePolicy())
	start.SelectedPoolId = testUpstreamProxyPool
	start.UpstreamProxy = testProxyInstruction()

	return start
}

func remotePolicy() *strawpb.DestinationPolicy {
	return &strawpb.DestinationPolicy{
		RedirectPolicy: strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		ResolutionMode: strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE,
	}
}

func testProxyInstruction() *strawpb.UpstreamProxyInstruction {
	return &strawpb.UpstreamProxyInstruction{
		UpstreamProxyId:   testUpstreamProxyID,
		ProviderSessionId: upstreamProxySentinelSession,
		Country:           "AU",
		Region:            "nsw",
		IpType:            "residential",
	}
}

func authorityFromURL(t *testing.T, rawURL string) string {
	t.Helper()
	target, failure := parseTarget(&strawpb.RequestStart{Url: rawURL})
	if failure != nil {
		t.Fatalf("parseTarget() failure = %v", failure)
	}

	return net.JoinHostPort(target.host, strconv.FormatUint(uint64(target.port), 10))
}

func responseHeaderValue(response *strawpb.ResponseStart, name string) string {
	for _, header := range response.GetHeaders() {
		if header.GetName() == name {
			return string(header.GetValue())
		}
	}

	return ""
}

func portFromAddress(t *testing.T, address string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", address, err)
	}

	return port
}
