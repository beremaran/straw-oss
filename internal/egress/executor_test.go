package egress

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
)

const xProtoHeader = "X-Proto"

const unitTestHost = "unit.test"

const testTenantA = "ten_a"

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

func TestExecutorBaselineRegressionMatrixPreservesOriginStatuses(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusOK, http.StatusMultipleChoices, http.StatusBadRequest, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("baseline"))
			}))
			t.Cleanup(server.Close)

			target := rewriteHost(t, server.URL, "baseline.test")
			exec := NewExecutor(ExecutorOptions{Resolver: staticResolver{"baseline.test": loopbackIP(t, server.URL)}})
			start := requestStart(target, directPolicy(true))
			frames := exec.Execute(context.Background(), start, nil, 1, nil)
			if errFrame := terminalErrorOrNil(frames); errFrame != nil {
				t.Fatalf("baseline status %d error = %#v", status, errFrame)
			}
			if got := int(frames[1].GetResponseStart().GetStatus()); got != status {
				t.Fatalf("status = %d, want %d", got, status)
			}
			if got := frames[0].GetOutboundStart().GetExecutedFingerprintProfile(); got != baselineFingerprintProfile {
				t.Fatalf("executed profile = %q, want baseline", got)
			}
		})
	}
}

func TestExecutorBaselineUsesExistingRetryAndPoolBoundaries(t *testing.T) {
	t.Parallel()

	resolver := hostResolver{unitTestHost: netip.MustParseAddr("203.0.113.10")}
	exec := NewExecutor(ExecutorOptions{Resolver: resolver, Pool: UpstreamConnectionPoolOptions{Enabled: true}})
	base := requestStart("http://unit.test:8080", &strawpb.DestinationPolicy{
		AllowedCidrs:   []string{"203.0.113.0/24"},
		RedirectPolicy: strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		ResolutionMode: strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL,
	})
	target, failure := parseTarget(base)
	if failure != nil {
		t.Fatalf("parseTarget() failure = %v", failure)
	}
	emptyKey, failure := exec.poolKey(context.Background(), testTenantA, target, base)
	if failure != nil {
		t.Fatalf("empty baseline poolKey() failure = %v", failure)
	}
	defaultAlias := proto.Clone(base).(*strawpb.RequestStart)
	defaultAlias.FingerprintInstruction = ""
	defaultKey, failure := exec.poolKey(context.Background(), testTenantA, target, defaultAlias)
	if failure != nil {
		t.Fatalf("default baseline poolKey() failure = %v", failure)
	}
	if emptyKey != defaultKey {
		t.Fatalf("baseline aliases split pool key: empty=%+v default=%+v", emptyKey, defaultKey)
	}
	named := proto.Clone(base).(*strawpb.RequestStart)
	named.FingerprintInstruction = chrome120FingerprintProfile
	namedKey, failure := exec.poolKey(context.Background(), testTenantA, target, named)
	if failure != nil {
		t.Fatalf("named poolKey() failure = %v", failure)
	}
	if namedKey == emptyKey {
		t.Fatal("named and baseline requests share a pool key")
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

// TestExecutorSendsOutboundStartBeforeConnect verifies the docs/public/architecture.md
// step 19 ordering that docs/public/architecture.md depends on: with a send callback the
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

func TestProfileConformanceEmitsExecutedAfterLocalResolutionBeforeDNS(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	resolver := &profileOrderResolver{addr: loopbackIP(t, server.URL)}
	exec := NewExecutor(ExecutorOptions{Resolver: resolver})
	start := requestStart(rewriteHost(t, server.URL, "ordered.profile.test"), directPolicy(true))
	start.FingerprintInstruction = chrome120FingerprintProfile

	var sawOutbound bool
	frames := exec.Execute(context.Background(), start, nil, 1, func(frame *strawpb.StreamFrame) {
		if outbound := frame.GetOutboundStart(); outbound != nil {
			sawOutbound = true
			if got := outbound.GetExecutedFingerprintProfile(); got != chrome120FingerprintProfile {
				t.Errorf("executed fingerprint = %q, want %q", got, chrome120FingerprintProfile)
			}
			if got := resolver.lookups.Load(); got != 0 {
				t.Errorf("DNS lookups before OutboundStart = %d, want 0", got)
			}
		}
	})
	if errFrame := terminalErrorOrNil(frames); errFrame != nil {
		t.Fatalf("profiled request error = %#v", errFrame)
	}
	if !sawOutbound {
		t.Fatal("missing OutboundStart frame")
	}
}

func TestExecutorRejectsUnknownFingerprintBeforeOutboundOrDNS(t *testing.T) {
	t.Parallel()

	resolver := &profileOrderResolver{addr: netip.MustParseAddr("127.0.0.1")}
	exec := NewExecutor(ExecutorOptions{Resolver: resolver})
	start := requestStart("http://unsupported.profile.test/", directPolicy(true))
	start.FingerprintInstruction = "not_registered"

	sawOutbound := false
	frames := exec.Execute(context.Background(), start, nil, 1, func(frame *strawpb.StreamFrame) {
		if frame.GetOutboundStart() != nil {
			sawOutbound = true
		}
	})
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT {
		t.Fatalf("error code = %v, want unsupported_fingerprint", got)
	}
	if sawOutbound {
		t.Fatal("unsupported profile emitted OutboundStart")
	}
	if got := resolver.lookups.Load(); got != 0 {
		t.Fatalf("DNS lookups = %d, want 0", got)
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
			cnames:         []string{"edge.denied-cdn.example."},
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

func TestExecutorRejectsDeniedCNAMESuffixIntermediate(t *testing.T) {
	t.Parallel()

	policy := directPolicy(true)
	policy.DeniedCnameSuffixes = []string{"denied-intermediate.example"}

	exec := NewExecutor(ExecutorOptions{
		Resolver: cnameResolver{
			staticResolver: staticResolver{"cname.test": netip.MustParseAddr("127.0.0.1")},
			cnames:         []string{"hop1.denied-intermediate.example.", "clean-target.example."},
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

func TestExecutorUpstreamConnectionPoolReusesExactKey(t *testing.T) {
	t.Parallel()

	var newConns atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if host == unitTestHost {
				return nil, errors.New("pooled dialer used original hostname")
			}

			var d net.Dialer

			return d.DialContext(ctx, network, address)
		},
		Pool: UpstreamConnectionPoolOptions{
			Enabled:             true,
			MaxIdleConnsPerHost: 2,
			IdleTimeout:         time.Second,
			MaxLifetime:         time.Minute,
		},
	})

	for range 2 {
		frames := exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
		if terminalErrorOrNil(frames) != nil {
			t.Fatalf("ExecuteWithDeployment() frames = %#v, want success", frames)
		}
	}
	if got := newConns.Load(); got != 1 {
		t.Fatalf("new connections = %d, want one reused connection", got)
	}

	frames := exec.ExecuteWithDeployment(context.Background(), "ten_b", requestStart(target, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("ExecuteWithDeployment() tenant b frames = %#v, want success", frames)
	}
	if got := newConns.Load(); got != 2 {
		t.Fatalf("new connections after tenant change = %d, want 2", got)
	}

	otherHostTarget := rewriteHost(t, server.URL, "other.test")
	exec.resolver = hostResolver{
		unitTestHost: loopbackIP(t, server.URL),
		"other.test": loopbackIP(t, server.URL),
	}
	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(otherHostTarget, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("ExecuteWithDeployment() other host frames = %#v, want success", frames)
	}
	if got := newConns.Load(); got != 3 {
		t.Fatalf("new connections after host change = %d, want 3", got)
	}

	start := requestStart(target, directPolicy(true))
	start.FingerprintInstruction = "not_registered"
	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, start, nil, 1, nil)
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT {
		t.Fatalf("code = %v, want unsupported_fingerprint", got)
	}
	if got := newConns.Load(); got != 3 {
		t.Fatalf("new connections after unsupported fingerprint = %d, want 3", got)
	}
}

func TestExecutorUpstreamConnectionPoolValidatesBeforeReuse(t *testing.T) {
	t.Parallel()

	var newConns atomic.Int32
	var closedConns atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			newConns.Add(1)
		case http.StateClosed:
			closedConns.Add(1)
		case http.StateActive, http.StateIdle, http.StateHijacked:
		default:
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	resolver := &sequenceResolver{
		host: unitTestHost,
		ips: []netip.Addr{
			loopbackIP(t, server.URL),
			netip.MustParseAddr("203.0.113.77"),
		},
	}
	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{
		Resolver: resolver,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if host == "203.0.113.77" {
				return nil, syscall.ECONNREFUSED
			}

			var d net.Dialer

			return d.DialContext(ctx, network, address)
		},
		Pool: UpstreamConnectionPoolOptions{
			Enabled:             true,
			MaxIdleConnsPerHost: 2,
			IdleTimeout:         time.Second,
			MaxLifetime:         time.Minute,
		},
	})

	frames := exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("first ExecuteWithDeployment() frames = %#v, want success", frames)
	}

	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, &strawpb.DestinationPolicy{
		AllowedCidrs:   []string{"203.0.113.77/32"},
		RedirectPolicy: strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		ResolutionMode: strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL,
	}), nil, 1, nil)
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED {
		t.Fatalf("code = %v, want fresh dial failure after stale pooled IP rejected", got)
	}
	if got := newConns.Load(); got != 1 {
		t.Fatalf("new connections = %d, want stale pooled connection not reused", got)
	}
	if got := resolver.lookups.Load(); got != 2 {
		t.Fatalf("DNS lookups = %d, want validation before each request", got)
	}
	deadline := time.Now().Add(time.Second)
	for closedConns.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if closedConns.Load() == 0 {
		t.Fatal("stale pooled connection was not closed after DNS rebinding")
	}
}

func TestExecutorUpstreamConnectionPoolKeyIncludesIsolationFields(t *testing.T) {
	t.Parallel()

	resolver := hostResolver{
		unitTestHost: netip.MustParseAddr("203.0.113.10"),
		"other.test": netip.MustParseAddr("203.0.113.11"),
	}
	exec := NewExecutor(ExecutorOptions{Resolver: resolver})
	base := requestStart("http://unit.test:8080", &strawpb.DestinationPolicy{
		AllowedCidrs:   []string{"203.0.113.0/24"},
		RedirectPolicy: strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		ResolutionMode: strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL,
	})

	baseTarget, failure := parseTarget(base)
	if failure != nil {
		t.Fatalf("parseTarget() failure = %v", failure)
	}
	baseKey, failure := exec.poolKey(context.Background(), testTenantA, baseTarget, base)
	if failure != nil {
		t.Fatalf("poolKey() failure = %v", failure)
	}

	tests := map[string]struct {
		tenant string
		start  *strawpb.RequestStart
	}{
		"tenant":         {tenant: "ten_b", start: base},
		"different-host": {tenant: testTenantA, start: requestStart("http://other.test:8080", base.GetDestinationPolicy())},
		"port":           {tenant: testTenantA, start: requestStart("http://unit.test:8081", base.GetDestinationPolicy())},
		"scheme":         {tenant: testTenantA, start: requestStart("https://unit.test:8080", base.GetDestinationPolicy())},
		"fingerprint": {tenant: testTenantA, start: func() *strawpb.RequestStart {
			start := requestStart("http://unit.test:8080", base.GetDestinationPolicy())
			start.FingerprintInstruction = "chrome_120"

			return start
		}()},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			target, failure := parseTarget(tt.start)
			if failure != nil {
				t.Fatalf("parseTarget() failure = %v", failure)
			}

			key, failure := exec.poolKey(context.Background(), tt.tenant, target, tt.start)
			if failure != nil {
				t.Fatalf("poolKey() failure = %v", failure)
			}
			if key == baseKey {
				t.Fatalf("pool key for %s matched base key: %+v", name, key)
			}
		})
	}

	ipKey, failure := NewExecutor(ExecutorOptions{Resolver: hostResolver{unitTestHost: netip.MustParseAddr("203.0.113.12")}}).
		poolKey(context.Background(), testTenantA, baseTarget, base)
	if failure != nil {
		t.Fatalf("poolKey() cross-IP failure = %v", failure)
	}
	if ipKey == baseKey {
		t.Fatalf("pool key for IP change matched base key: %+v", ipKey)
	}
}

func TestExecutorUpstreamConnectionPoolEviction(t *testing.T) {
	t.Parallel()

	goroutinesBefore := runtime.NumGoroutine()
	var newConns atomic.Int32
	var closedConns atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-protocol":
			if h, ok := w.(http.Hijacker); ok {
				conn, rw, err := h.Hijack()
				if err == nil {
					_, _ = rw.WriteString("not http\r\n\r\n")
					_ = rw.Flush()
					_ = conn.Close()
				}
			}
		case "/short":
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write([]byte("x"))
			if h, ok := w.(http.Hijacker); ok {
				conn, _, err := h.Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			newConns.Add(1)
		case http.StateClosed:
			closedConns.Add(1)
		case http.StateActive, http.StateIdle, http.StateHijacked:
		default:
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)},
		Pool: UpstreamConnectionPoolOptions{
			Enabled:             true,
			MaxIdleConnsPerHost: 2,
			IdleTimeout:         10 * time.Millisecond,
			MaxLifetime:         20 * time.Millisecond,
		},
	})

	frames := exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("first ExecuteWithDeployment() frames = %#v, want success", frames)
	}

	time.Sleep(40 * time.Millisecond)
	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("second ExecuteWithDeployment() frames = %#v, want success", frames)
	}
	if got := newConns.Load(); got < 2 {
		t.Fatalf("new connections = %d, want idle/lifetime eviction to force a fresh connection", got)
	}

	shortTarget := target + "/short"
	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(shortTarget, directPolicy(true)), nil, 1, nil)
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET {
		t.Fatalf("code = %v, want upstream_reset", got)
	}
	before := newConns.Load()
	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("after body error frames = %#v, want success", frames)
	}
	if got := newConns.Load(); got <= before {
		t.Fatalf("new connections after body error = %d, want fresh connection beyond %d", got, before)
	}

	badProtocolTarget := target + "/bad-protocol"
	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(badProtocolTarget, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) == nil {
		t.Fatalf("bad protocol frames = %#v, want error", frames)
	}
	before = newConns.Load()
	frames = exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("after protocol error frames = %#v, want success", frames)
	}
	if got := newConns.Load(); got <= before {
		t.Fatalf("new connections after protocol error = %d, want fresh connection beyond %d", got, before)
	}

	beforeClose := closedConns.Load()
	exec.CloseIdleConnections()
	deadline := time.Now().Add(time.Second)
	for closedConns.Load() <= beforeClose && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if closedConns.Load() <= beforeClose {
		t.Fatal("CloseIdleConnections did not close any pooled connection")
	}

	deadline = time.Now().Add(time.Second)
	for runtime.NumGoroutine() > goroutinesBefore+20 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > goroutinesBefore+20 {
		t.Fatalf("goroutines after pool cleanup = %d, want <= %d", got, goroutinesBefore+20)
	}
}

func TestExecutorUpstreamConnectionPoolTLSFailureIsNotReused(t *testing.T) {
	t.Parallel()

	var newConns atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("tls"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)},
		Pool: UpstreamConnectionPoolOptions{
			Enabled:             true,
			MaxIdleConnsPerHost: 2,
			IdleTimeout:         time.Second,
			MaxLifetime:         time.Minute,
		},
	})

	for range 2 {
		frames := exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
		if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE {
			t.Fatalf("code = %v, want upstream_tls_failure", got)
		}
	}
	if got := newConns.Load(); got != 2 {
		t.Fatalf("new connections = %d, want TLS-failed connection not reused", got)
	}
}

func TestWorkerShutdownClosesExecutorPool(t *testing.T) {
	t.Parallel()

	var closedConns atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closedConns.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)},
		Pool: UpstreamConnectionPoolOptions{
			Enabled:             true,
			MaxIdleConnsPerHost: 2,
			IdleTimeout:         time.Second,
			MaxLifetime:         time.Minute,
		},
	})

	frames := exec.ExecuteWithDeployment(context.Background(), testTenantA, requestStart(target, directPolicy(true)), nil, 1, nil)
	if terminalErrorOrNil(frames) != nil {
		t.Fatalf("ExecuteWithDeployment() frames = %#v, want success", frames)
	}

	exec.CloseIdleConnections()

	deadline := time.Now().Add(time.Second)
	for closedConns.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if closedConns.Load() == 0 {
		t.Fatal("shutdown cleanup did not close pooled idle connection")
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

func (r staticResolver) LookupCNAME(_ context.Context, host string) ([]string, error) {
	return []string{host}, nil
}

// cnameResolver wraps staticResolver's IP lookups and returns a fixed
// canonical name regardless of the requested host, for denied_cname_suffixes
// tests.
type cnameResolver struct {
	staticResolver
	cnames []string
}

func (r cnameResolver) LookupCNAME(context.Context, string) ([]string, error) {
	return r.cnames, nil
}

type hostResolver map[string]netip.Addr

func (r hostResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addr, ok := r[host]
	if !ok {
		return nil, errors.New("host not found")
	}

	return []net.IPAddr{{IP: net.ParseIP(addr.String())}}, nil
}

func (r hostResolver) LookupCNAME(_ context.Context, host string) ([]string, error) {
	return []string{host}, nil
}

type sequenceResolver struct {
	host    string
	ips     []netip.Addr
	lookups atomic.Int32
}

func (r *sequenceResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if host != r.host {
		return nil, errors.New("unexpected host")
	}

	idx := int(r.lookups.Add(1)) - 1
	if idx >= len(r.ips) {
		idx = len(r.ips) - 1
	}

	return []net.IPAddr{{IP: net.ParseIP(r.ips[idx].String())}}, nil
}

func (r *sequenceResolver) LookupCNAME(_ context.Context, host string) ([]string, error) {
	return []string{host}, nil
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

func TestExecutorHTTP2Negotiation(t *testing.T) {
	t.Parallel()

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(xProtoHeader, r.Proto)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	t.Cleanup(ts.Close)

	certPool := x509.NewCertPool()
	certPool.AddCert(ts.Certificate())

	target := rewriteHost(t, ts.URL, "http2.test")
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{"http2.test": loopbackIP(t, ts.URL)},
		HTTP2Enabled:       true,
		RootCAs:            certPool,
		InsecureSkipVerify: true,
	})

	frames := exec.Execute(context.Background(), requestStart(target, directPolicy(true)), nil, 1, nil)
	if len(frames) < 3 {
		t.Fatalf("len(frames) = %d, want at least 3", len(frames))
	}
	respStart := frames[1].GetResponseStart()
	if respStart == nil {
		t.Fatalf("expected response start frame, got nil")
	}

	proto := ""
	for _, h := range respStart.GetHeaders() {
		if h.GetName() == xProtoHeader {
			proto = string(h.GetValue())
		}
	}
	if proto != "HTTP/2.0" {
		t.Fatalf("expected HTTP/2.0, got %q", proto)
	}
}

func TestExecutorHTTP2FallbackToHTTP11(t *testing.T) {
	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(xProtoHeader, r.Proto)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(ts.Close)

	certPool := x509.NewCertPool()
	certPool.AddCert(ts.Certificate())

	target := rewriteHost(t, ts.URL, "fallback.test")
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{"fallback.test": loopbackIP(t, ts.URL)},
		HTTP2Enabled:       true,
		RootCAs:            certPool,
		InsecureSkipVerify: true,
	})

	frames := exec.Execute(context.Background(), requestStart(target, directPolicy(true)), nil, 1, nil)
	if len(frames) < 3 {
		t.Fatalf("len(frames) = %d, want at least 3", len(frames))
	}
	respStart := frames[1].GetResponseStart()
	if respStart == nil {
		t.Fatalf("expected response start frame, got nil")
	}

	proto := ""
	for _, h := range respStart.GetHeaders() {
		if h.GetName() == xProtoHeader {
			proto = string(h.GetValue())
		}
	}
	if proto != "HTTP/1.1" {
		t.Fatalf("expected HTTP/1.1, got %q", proto)
	}

	if !exec.isHTTP11Only("fallback.test") {
		t.Fatalf("expected fallback.test to be cached as HTTP/1.1-only")
	}
}

func TestExecutorHTTP2HTTP11RequiredRetry(t *testing.T) {
	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(xProtoHeader, r.Proto)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("retry-ok"))
	}))
	t.Cleanup(ts.Close)

	certPool := x509.NewCertPool()
	certPool.AddCert(ts.Certificate())

	target := rewriteHost(t, ts.URL, "retry.test")

	var attempts atomic.Int32
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{"retry.test": loopbackIP(t, ts.URL)},
		HTTP2Enabled:       true,
		RootCAs:            certPool,
		InsecureSkipVerify: true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			attempt := attempts.Add(1)
			if attempt == 1 {
				return nil, http2.StreamError{
					StreamID: 1,
					Code:     http2.ErrCodeHTTP11Required,
				}
			}

			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	})

	policy := directPolicy(true)
	reqStart := requestStart(target, policy)
	reqStart.Replayable = true

	frames := exec.Execute(context.Background(), reqStart, nil, 1, nil)
	if len(frames) < 3 {
		t.Fatalf("len(frames) = %d, want at least 3", len(frames))
	}
	respStart := frames[1].GetResponseStart()
	if respStart == nil {
		t.Fatalf("expected response start frame, got nil")
	}

	proto := ""
	for _, h := range respStart.GetHeaders() {
		if h.GetName() == xProtoHeader {
			proto = string(h.GetValue())
		}
	}
	if proto != "HTTP/1.1" {
		t.Fatalf("expected HTTP/1.1 on second attempt, got %q", proto)
	}

	if attempts.Load() != 2 {
		t.Fatalf("expected 2 dial attempts, got %d", attempts.Load())
	}
}

func TestExecutorHTTP2HTTP11RequiredNotReplayable(t *testing.T) {
	t.Parallel()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	certPool := x509.NewCertPool()
	certPool.AddCert(ts.Certificate())

	target := rewriteHost(t, ts.URL, "noreplay.test")

	var attempts atomic.Int32
	exec := NewExecutor(ExecutorOptions{
		Resolver:           staticResolver{"noreplay.test": loopbackIP(t, ts.URL)},
		HTTP2Enabled:       true,
		RootCAs:            certPool,
		InsecureSkipVerify: true,
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			attempts.Add(1)

			return nil, http2.StreamError{
				StreamID: 1,
				Code:     http2.ErrCodeHTTP11Required,
			}
		},
	})

	policy := directPolicy(true)
	reqStart := requestStart(target, policy)
	reqStart.Replayable = false

	frames := exec.Execute(context.Background(), reqStart, nil, 1, nil)
	errFrame := terminalError(t, frames)
	if errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET || errFrame.GetDetails()["fact"] != "http_1_1_required" {
		t.Fatalf("unexpected error frame: %+v", errFrame)
	}

	if attempts.Load() != 1 {
		t.Fatalf("expected 1 dial attempt, got %d", attempts.Load())
	}
}
