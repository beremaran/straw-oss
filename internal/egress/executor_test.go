package egress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const unitTestHost = "unit.test"

func TestExecutorEmitsSuccessfulHTTPFramesAndAppliesInjection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Set") != "two" {
			t.Fatalf("X-Set = %q, want two", r.Header.Get("X-Set"))
		}
		if got := r.Header.Values("X-Append"); strings.Join(got, ",") != "one,two" {
			t.Fatalf("X-Append = %q, want one,two", got)
		}
		if r.Header.Get("X-Remove") != "" {
			t.Fatalf("X-Remove = %q, want empty", r.Header.Get("X-Remove"))
		}
		if r.Header.Get("Cookie") != "session=abc" {
			t.Fatalf("Cookie = %q, want session=abc", r.Header.Get("Cookie"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "payload" {
			t.Fatalf("body = %q, want payload", body)
		}

		w.Header().Set("X-Origin", "ok")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)}})
	frames := exec.Execute(context.Background(), requestStart(target, directPolicy(true)), []byte("payload"), 3, nil)

	if len(frames) != 4 {
		t.Fatalf("len(frames) = %d, want 4", len(frames))
	}
	if got := frames[0].GetOutboundStart(); got == nil || got.GetTargetHost() != unitTestHost || got.GetAttempt() != 3 {
		t.Fatalf("outbound start = %#v", got)
	}
	if got := frames[1].GetResponseStart(); got == nil || got.GetStatus() != http.StatusTeapot {
		t.Fatalf("response start = %#v", got)
	}
	if got := string(frames[2].GetData().GetData()); got != "ok" {
		t.Fatalf("data = %q, want ok", got)
	}
	if got := frames[3].GetEnd(); got == nil || !got.GetSuccess() {
		t.Fatalf("end = %#v", got)
	}
}

func TestExecutorOpenTunnelUsesDestinationPolicyDial(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"tunnel.test": netip.MustParseAddr("203.0.113.10")},
		DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
			if address != "203.0.113.10:443" {
				t.Fatalf("dial address = %q, want 203.0.113.10:443", address)
			}

			return client, nil
		},
	})

	conn, target, failure := exec.openTunnel(context.Background(), tunnelStart("connect://tunnel.test:443", &strawpb.DestinationPolicy{
		AllowedCidrs:   []string{"203.0.113.10/32"},
		RedirectPolicy: strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		ResolutionMode: strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL,
	}))
	if failure != nil {
		t.Fatalf("openTunnel() failure = %v", failure)
	}
	defer func() { _ = conn.Close() }()

	if target.host != "tunnel.test" || target.port != 443 {
		t.Fatalf("target = %+v, want tunnel.test:443", target)
	}
}

func TestExecutorOpenTunnelAppliesDeniedIPPolicy(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{"blocked.test": netip.MustParseAddr("127.0.0.1")}})

	_, _, failure := exec.openTunnel(context.Background(), tunnelStart("connect://blocked.test:443", directPolicy(false)))
	if failure == nil || failure.code != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
		t.Fatalf("openTunnel() failure = %v, want destination denied", failure)
	}
}

// TestExecutorSendsOutboundStartBeforeConnect verifies the docs/planning/09
// step 19 ordering that docs/tasks/p0/41 depends on: with a send callback the
// OutboundStartFrame is delivered before the upstream request is made, and is
// excluded from the returned batch.
func TestExecutorSendsOutboundStartBeforeConnect(t *testing.T) {
	t.Parallel()

	upstreamHit := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(upstreamHit)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	var sent []*strawpb.StreamFrame

	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)}})
	frames := exec.Execute(context.Background(), requestStart(target, directPolicy(true)), nil, 1, func(frame *strawpb.StreamFrame) {
		if frame.GetOutboundStart() != nil {
			select {
			case <-upstreamHit:
				t.Error("OutboundStart sent after upstream was contacted")
			default:
			}
		}

		sent = append(sent, frame)
	})

	if len(sent) == 0 || sent[0].GetOutboundStart() == nil {
		t.Fatalf("sent = %#v, want first callback frame to be OutboundStart", sent)
	}
	for _, frame := range frames {
		if frame.GetOutboundStart() != nil {
			t.Fatalf("returned batch still contains OutboundStart: %#v", frame)
		}
	}
	if len(frames) != 1 {
		t.Fatalf("returned frames = %#v, want terminal frame only", frames)
	}
	if last := frames[0].GetEnd(); last == nil || !last.GetSuccess() {
		t.Fatalf("terminal frame = %#v, want successful EndFrame", frames[len(frames)-1])
	}
}

func TestExecutorStreamsResponseFramesBeforeUpstreamCompletes(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	firstData := make(chan struct{})
	body := bytes.Repeat([]byte("x"), responseFrameDataBytes+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)}})
	done := make(chan []*strawpb.StreamFrame, 1)

	go func() {
		done <- exec.Execute(context.Background(), requestStart(target, directPolicy(true)), nil, 1, func(frame *strawpb.StreamFrame) {
			if frame.GetData() != nil {
				select {
				case <-firstData:
				default:
					close(firstData)
				}
			}
		})
	}()

	select {
	case <-firstData:
	case <-time.After(time.Second):
		t.Fatal("executor did not send response data before upstream handler completed")
	}

	close(release)

	frames := <-done
	if len(frames) != 1 || frames[0].GetEnd() == nil {
		t.Fatalf("returned frames = %#v, want terminal EndFrame only", frames)
	}
}

func TestExecutorRejectsResolvedDeniedIPAndRedactsDetails(t *testing.T) {
	t.Parallel()

	dialed := false
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"metadata.test": netip.MustParseAddr("169.254.169.254")},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialed = true

			return nil, errors.New("unexpected dial")
		},
	})
	start := requestStart("http://metadata.test/secret?token=abc", directPolicy(false))
	start.Headers = []*strawpb.Header{{Name: "Authorization", Value: []byte("secret")}}

	frames := exec.Execute(context.Background(), start, nil, 1, nil)

	if dialed {
		t.Fatal("executor dialed a denied destination")
	}
	errFrame := terminalError(t, frames)
	if errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
		t.Fatalf("code = %v, want destination_denied", errFrame.GetCode())
	}
	if got := errFrame.GetDetails(); len(got) != 1 || got["fact"] != "dns_denied_ip" {
		t.Fatalf("details = %#v, want only fact=dns_denied_ip", got)
	}
}

func TestExecutorDeniesPrivateAndMetadataIPsByDefault(t *testing.T) {
	t.Parallel()

	tests := map[string]netip.Addr{
		"private":  netip.MustParseAddr("10.0.0.1"),
		"metadata": netip.MustParseAddr("100.100.100.200"),
	}
	for name, addr := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{"denied.test": addr}})
			frames := exec.Execute(context.Background(), requestStart("http://denied.test", directPolicy(false)), nil, 1, nil)
			if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
				t.Fatalf("code = %v, want destination_denied", got)
			}
		})
	}
}

func TestExecutorAllowedCIDRsOverridePrivateAndMetadataDenials(t *testing.T) {
	t.Parallel()

	tests := map[string]netip.Addr{
		"private":  netip.MustParseAddr("10.0.0.1"),
		"loopback": netip.MustParseAddr("127.0.0.1"),
		"metadata": netip.MustParseAddr("169.254.169.254"),
	}
	for name, addr := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			policy := directPolicy(false)
			policy.AllowedCidrs = []string{addr.String() + "/32"}

			dialed := false
			exec := NewExecutor(ExecutorOptions{
				Resolver: staticResolver{"override.test": addr},
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dialed = true

					return nil, errors.New("no real upstream in this test")
				},
			})

			frames := exec.Execute(context.Background(), requestStart("http://override.test", policy), nil, 1, nil)
			if !dialed {
				t.Fatal("executor did not dial the allowed_cidrs-overridden address")
			}
			errFrame := terminalError(t, frames)
			if errFrame.GetDetails()["fact"] == dnsDeniedIPFact {
				t.Fatalf("details = %#v, want no destination-denied rejection", errFrame.GetDetails())
			}
		})
	}
}

func TestExecutorIs4In6NeverOverriddenByAllowedCIDRs(t *testing.T) {
	t.Parallel()

	addr := netip.MustParseAddr("::ffff:169.254.169.254")
	policy := directPolicy(false)
	policy.AllowedCidrs = []string{"::ffff:0:0/96"}

	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"mapped.test": addr},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("executor dialed an Is4In6 address")

			return nil, errors.New("unexpected dial")
		},
	})

	frames := exec.Execute(context.Background(), requestStart("http://mapped.test", policy), nil, 1, nil)
	errFrame := terminalError(t, frames)
	if errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
		t.Fatalf("code = %v, want destination_denied", errFrame.GetCode())
	}
}

func TestExecutorRejectsDeniedHostSuffix(t *testing.T) {
	t.Parallel()

	policy := directPolicy(true)
	policy.DeniedHostSuffixes = []string{"blocked.example"}

	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"sub.blocked.example": netip.MustParseAddr("127.0.0.1")},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("executor dialed a denied-host-suffix target")

			return nil, errors.New("unexpected dial")
		},
	})

	frames := exec.Execute(context.Background(), requestStart("http://sub.blocked.example", policy), nil, 1, nil)
	errFrame := terminalError(t, frames)
	if errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
		t.Fatalf("code = %v, want destination_denied", errFrame.GetCode())
	}
	if got := errFrame.GetDetails()["fact"]; got != "host_denied_suffix" {
		t.Fatalf("fact = %q, want host_denied_suffix", got)
	}
}

func TestExecutorRejectsDeniedCNAMESuffix(t *testing.T) {
	t.Parallel()

	policy := directPolicy(true)
	policy.DeniedCnameSuffixes = []string{"denied-cdn.example"}

	exec := NewExecutor(ExecutorOptions{
		Resolver: cnameResolver{
			staticResolver: staticResolver{"cname.test": netip.MustParseAddr("127.0.0.1")},
			cname:          "edge.denied-cdn.example.",
		},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("executor dialed a denied-cname-suffix target")

			return nil, errors.New("unexpected dial")
		},
	})

	frames := exec.Execute(context.Background(), requestStart("http://cname.test", policy), nil, 1, nil)
	errFrame := terminalError(t, frames)
	if errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
		t.Fatalf("code = %v, want destination_denied", errFrame.GetCode())
	}
	if got := errFrame.GetDetails()["fact"]; got != "cname_denied_suffix" {
		t.Fatalf("fact = %q, want cname_denied_suffix", got)
	}
}

func TestExecutorBlocksDNSRebindingByDialingValidatedIP(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	var dialAddress string
	dialer := &net.Dialer{}
	target := rewriteHost(t, server.URL, "rebind.test")
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{"rebind.test": loopbackIP(t, server.URL)},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialAddress = address

			return dialer.DialContext(ctx, network, address)
		},
	})

	frames := exec.Execute(context.Background(), requestStart(target, directPolicy(true)), nil, 1, nil)

	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("unexpected error frame: %#v", terminalErrorOrNil(frames))
	}
	host, _, err := net.SplitHostPort(dialAddress)
	if err != nil {
		t.Fatalf("dial address %q: %v", dialAddress, err)
	}
	if host != loopbackIP(t, server.URL).String() {
		t.Fatalf("dial host = %q, want validated IP %q", host, loopbackIP(t, server.URL))
	}
}

func TestExecutorRejectsUnsafeHeaderInjection(t *testing.T) {
	t.Parallel()

	tests := map[string]*strawpb.InjectionOperation{
		"host":      {Op: opSet, HeaderName: "Host", Value: []byte("evil.test")},
		"crlf":      {Op: opAppend, HeaderName: "X-Test", Value: []byte("bad\r\nvalue")},
		"duplicate": {Op: opSet, HeaderName: "X-Dupe", Value: []byte("one")},
	}
	for name, op := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			start := requestStart("http://header.test", directPolicy(true))
			start.InjectionOperations = []*strawpb.InjectionOperation{op}
			if name == "duplicate" {
				start.InjectionOperations = append(start.InjectionOperations, &strawpb.InjectionOperation{Op: opSet, HeaderName: "x-dupe", Value: []byte("two")})
			}
			exec := NewExecutor(ExecutorOptions{
				Resolver: staticResolver{"header.test": netip.MustParseAddr("127.0.0.1")},
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					t.Fatal("executor dialed after unsafe injection")

					return nil, errors.New("unexpected dial")
				},
			})

			errFrame := terminalError(t, exec.Execute(context.Background(), start, nil, 1, nil))
			if errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR {
				t.Fatalf("code = %v, want executor_internal_error", errFrame.GetCode())
			}
			if got := errFrame.GetDetails()["fact"]; got != "header_injection_failed" {
				t.Fatalf("fact = %q, want header_injection_failed", got)
			}
		})
	}
}

func TestExecutorEnforcesTotalDeadline(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, "slow.test")
	start := requestStart(target, directPolicy(true))
	start.DeadlineUnixMs = time.Now().Add(20 * time.Millisecond).UnixMilli()
	exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{"slow.test": loopbackIP(t, server.URL)}})

	errFrame := terminalError(t, exec.Execute(context.Background(), start, nil, 1, nil))
	if errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED {
		t.Fatalf("code = %v, want timeout_exceeded", errFrame.GetCode())
	}
	if errFrame.GetTimeoutType() != strawpb.TimeoutType_TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT {
		t.Fatalf("timeout type = %v, want total deadline", errFrame.GetTimeoutType())
	}
}

func TestExecutorDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/next" {
			w.WriteHeader(http.StatusOK)

			return
		}
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, "redirect.test")
	exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{"redirect.test": loopbackIP(t, server.URL)}})
	frames := exec.Execute(context.Background(), requestStart(target, directPolicy(true)), nil, 1, nil)

	if got := frames[1].GetResponseStart().GetStatus(); got != http.StatusFound {
		t.Fatalf("status = %d, want redirect passthrough %d", got, http.StatusFound)
	}
}

func TestP0TransportDefaults(t *testing.T) {
	t.Parallel()

	tr := NewP0Transport(func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("unused") })
	if !tr.DisableKeepAlives {
		t.Fatal("DisableKeepAlives = false, want true")
	}
	if tr.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = true, want false")
	}
	if len(tr.TLSNextProto) != 0 {
		t.Fatalf("TLSNextProto len = %d, want 0", len(tr.TLSNextProto))
	}
}

type staticResolver map[string]netip.Addr

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	out := make([]net.IPAddr, 0, len(r))
	for _, addr := range r {
		out = append(out, net.IPAddr{IP: net.ParseIP(addr.String())})
	}

	return out, nil
}

func (r staticResolver) LookupCNAME(_ context.Context, host string) (string, error) {
	return host, nil
}

// cnameResolver wraps staticResolver's IP lookups and returns a fixed
// canonical name regardless of the requested host, for denied_cname_suffixes
// tests.
type cnameResolver struct {
	staticResolver
	cname string
}

func (r cnameResolver) LookupCNAME(context.Context, string) (string, error) {
	return r.cname, nil
}

func requestStart(rawURL string, policy *strawpb.DestinationPolicy) *strawpb.RequestStart {
	return &strawpb.RequestStart{
		Mode:           strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP,
		Method:         http.MethodPost,
		Url:            rawURL,
		DeadlineUnixMs: time.Now().Add(5 * time.Second).UnixMilli(),
		Headers:        []*strawpb.Header{{Name: "X-Append", Value: []byte("one")}, {Name: "X-Remove", Value: []byte("gone")}},
		InjectionOperations: []*strawpb.InjectionOperation{
			{Op: opSet, HeaderName: "X-Set", Value: []byte("two")},
			{Op: opSet, HeaderName: "Cookie", Value: []byte("session=abc")},
			{Op: opAppend, HeaderName: "X-Append", Value: []byte("two")},
			{Op: opRemove, HeaderName: "X-Remove"},
		},
		DestinationPolicy: policy,
	}
}

func tunnelStart(rawURL string, policy *strawpb.DestinationPolicy) *strawpb.RequestStart {
	return &strawpb.RequestStart{
		Mode:              strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL,
		Method:            http.MethodConnect,
		Url:               rawURL,
		DeadlineUnixMs:    time.Now().Add(5 * time.Second).UnixMilli(),
		RedirectPolicy:    strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		DestinationPolicy: policy,
	}
}

func directPolicy(allowLoopback bool) *strawpb.DestinationPolicy {
	return &strawpb.DestinationPolicy{
		AllowLoopback:  allowLoopback,
		RedirectPolicy: strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		ResolutionMode: strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL,
	}
}

func terminalError(t *testing.T, frames []*strawpb.StreamFrame) *strawpb.ErrorFrame {
	t.Helper()

	errFrame := terminalErrorOrNil(frames)
	if errFrame == nil {
		t.Fatalf("terminal frame = %#v, want error", frames[len(frames)-1])
	}

	return errFrame
}

func terminalErrorOrNil(frames []*strawpb.StreamFrame) *strawpb.ErrorFrame {
	if len(frames) == 0 {
		return nil
	}

	return frames[len(frames)-1].GetError()
}

func rewriteHost(t *testing.T, raw, host string) string {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split server URL host: %v", err)
	}
	u.Host = net.JoinHostPort(host, port)

	return u.String()
}

func loopbackIP(t *testing.T, raw string) netip.Addr {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split server URL host: %v", err)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		t.Fatalf("parse server IP: %v", err)
	}

	return addr
}
