package egress

import (
	"bytes"
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fhttphttp2 "github.com/bogdanfinn/fhttp/http2"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func TestProfilePinnedDialPreservesOriginalHostAndCertificate(t *testing.T) {
	t.Parallel()

	var seenHost, seenSNI string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHost = r.Host
		if r.TLS != nil {
			seenSNI = r.TLS.ServerName
		}
		_, _ = w.Write([]byte("profiled"))
	}))
	server.StartTLS()
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, "example.com")
	wantURL, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}

	var dialAddress string
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"example.com": loopbackIP(t, server.URL)},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialAddress = address

			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
		RootCAs: func() *x509.CertPool {
			pool := x509.NewCertPool()
			pool.AddCert(server.Certificate())

			return pool
		}(),
	})
	start := requestStart(target, directPolicy(true))
	start.FingerprintInstruction = chrome120FingerprintProfile

	frames := exec.Execute(context.Background(), start, nil, 1, nil)
	if errFrame := terminalErrorOrNil(frames); errFrame != nil {
		t.Fatalf("profiled request error = %#v", errFrame)
	}

	dialHost, _, err := net.SplitHostPort(dialAddress)
	if err != nil {
		t.Fatalf("split dial address %q: %v", dialAddress, err)
	}
	if dialHost != loopbackIP(t, server.URL).String() {
		t.Fatalf("dial host = %q, want validated IP %q", dialHost, loopbackIP(t, server.URL))
	}
	if seenHost != wantURL.Host {
		t.Fatalf("upstream Host = %q, want original URL host %q", seenHost, wantURL.Host)
	}
	if seenSNI != "example.com" {
		t.Fatalf("upstream SNI = %q, want example.com", seenSNI)
	}
}

func TestProfileConformanceStreamsBodyAndLateTrailers(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	body := bytes.Repeat([]byte("x"), responseFrameDataBytes+1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Trailer", "X-Late")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-release
		w.Header().Set("X-Late", "done")
	}))
	server.StartTLS()
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, "stream.profile.test")
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{"stream.profile.test": loopbackIP(t, server.URL)},
		InsecureSkipVerify: true,
	})
	start := requestStart(target, directPolicy(true))
	start.FingerprintInstruction = chrome120FingerprintProfile

	var (
		mu       sync.Mutex
		frames   []*strawpb.StreamFrame
		first    = make(chan struct{})
		firstOne sync.Once
		done     = make(chan struct{})
	)
	go func() {
		defer close(done)
		exec.Execute(context.Background(), start, nil, 1, func(frame *strawpb.StreamFrame) {
			mu.Lock()
			frames = append(frames, frame)
			mu.Unlock()
			if frame.GetData() != nil {
				firstOne.Do(func() { close(first) })
			}
		})
	}()

	select {
	case <-first:
	case <-time.After(2 * time.Second):
		t.Fatal("profiled transport did not stream the first body frame")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("profiled transport did not finish after trailer release")
	}

	mu.Lock()
	defer mu.Unlock()
	dataFrames := 0
	trailerSeen := false
	for _, frame := range frames {
		if frame.GetData() != nil {
			dataFrames++
		}
		if trailer := frame.GetTrailers(); trailer != nil {
			trailerSeen = trailer.GetHeaders()[0].GetName() == "X-Late" && string(trailer.GetHeaders()[0].GetValue()) == "done"
		}
		if errFrame := frame.GetError(); errFrame != nil {
			t.Fatalf("profiled streaming error = %#v", errFrame)
		}
	}
	if dataFrames < 2 {
		t.Fatalf("data frames = %d, want at least 2 for 32 KiB streaming", dataFrames)
	}
	if !trailerSeen {
		t.Fatal("late response trailer was not emitted")
	}
}

func TestProfileConnectionIsolationDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	var newConnections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)

			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, "reuse.profile.test")
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{"reuse.profile.test": loopbackIP(t, server.URL)},
		InsecureSkipVerify: true,
	})

	redirectURL, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	redirectURL.Path = "/redirect"
	redirect := requestStart(redirectURL.String(), directPolicy(true))
	redirect.FingerprintInstruction = chrome120FingerprintProfile
	redirectFrames := exec.Execute(context.Background(), redirect, nil, 1, nil)
	if errFrame := terminalErrorOrNil(redirectFrames); errFrame != nil {
		t.Fatalf("redirect request error = %#v", errFrame)
	}
	if got := redirectFrames[1].GetResponseStart().GetStatus(); got != http.StatusFound {
		t.Fatalf("redirect status = %d, want %d", got, http.StatusFound)
	}

	for range 2 {
		start := requestStart(target, directPolicy(true))
		start.FingerprintInstruction = chrome120FingerprintProfile
		frames := exec.Execute(context.Background(), start, nil, 1, nil)
		if errFrame := terminalErrorOrNil(frames); errFrame != nil {
			t.Fatalf("profiled request error = %#v", errFrame)
		}
	}
	if got := newConnections.Load(); got != 3 {
		t.Fatalf("new connections = %d, want 3 request-scoped connections", got)
	}
}

func TestProfileCancellationStopsPinnedDial(t *testing.T) {
	t.Parallel()

	dialStarted := make(chan struct{})
	var startedOnce sync.Once
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"cancel.profile.test": netip.MustParseAddr("127.0.0.1")},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			startedOnce.Do(func() { close(dialStarted) })
			<-ctx.Done()

			return nil, ctx.Err()
		},
	})
	start := requestStart("http://cancel.profile.test/", directPolicy(true))
	start.FingerprintInstruction = chrome120FingerprintProfile
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan []*strawpb.StreamFrame, 1)
	go func() { done <- exec.Execute(ctx, start, nil, 1, nil) }()

	select {
	case <-dialStarted:
	case <-time.After(time.Second):
		t.Fatal("profiled dialer did not start")
	}
	cancel()

	select {
	case frames := <-done:
		if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_CANCELLED {
			t.Fatalf("error code = %v, want cancelled", got)
		}
	case <-time.After(time.Second):
		t.Fatal("profiled cancellation did not stop the dial")
	}
}

func TestProfileDeadlineDuringPinnedDial(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"deadline.profile.test": netip.MustParseAddr("127.0.0.1")},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()

			return nil, ctx.Err()
		},
	})
	start := requestStart("http://deadline.profile.test/", directPolicy(true))
	start.FingerprintInstruction = chrome120FingerprintProfile
	start.DeadlineUnixMs = time.Now().Add(20 * time.Millisecond).UnixMilli()

	frames := exec.Execute(context.Background(), start, nil, 1, nil)
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED {
		t.Fatalf("error code = %v, want timeout_exceeded", got)
	}
}

func TestProfileErrorMappingUsesFHTTPStreamError(t *testing.T) {
	t.Parallel()

	failure, ok := mapHTTP2Error(&fhttphttp2.StreamError{Code: fhttphttp2.ErrCodeRefusedStream})
	if !ok {
		t.Fatal("mapHTTP2Error() did not recognize fhttp/http2.StreamError")
	}
	if failure.code != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET {
		t.Fatalf("mapped code = %v, want upstream_reset", failure.code)
	}
}

type profileOrderResolver struct {
	addr    netip.Addr
	lookups atomic.Int32
}

func (r *profileOrderResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.lookups.Add(1)

	return []net.IPAddr{{IP: net.ParseIP(r.addr.String())}}, nil
}

func (r *profileOrderResolver) LookupCNAME(_ context.Context, host string) ([]string, error) {
	return []string{host}, nil
}
