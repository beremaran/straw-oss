package egress

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

type unsupportedProfileObserver struct {
	resolverCalls atomic.Int32
	policyDials   atomic.Int32
	tcpConnects   atomic.Int32
	tlsHandshakes atomic.Int32
	httpRequests  atomic.Int32
}

type unsupportedProfileResolver struct {
	observer *unsupportedProfileObserver
	addr     netip.Addr
}

func (r unsupportedProfileResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.observer.resolverCalls.Add(1)

	return []net.IPAddr{{IP: net.ParseIP(r.addr.String())}}, nil
}

func (r unsupportedProfileResolver) LookupCNAME(context.Context, string) ([]string, error) {
	return []string{"unsupported.profile.test"}, nil
}

func (o *unsupportedProfileObserver) assertZero(t *testing.T) {
	t.Helper()

	if got := o.resolverCalls.Load(); got != 0 {
		t.Fatalf("resolver calls = %d, want 0", got)
	}
	if got := o.policyDials.Load(); got != 0 {
		t.Fatalf("policy dial calls = %d, want 0", got)
	}
	if got := o.tcpConnects.Load(); got != 0 {
		t.Fatalf("tcp connects = %d, want 0", got)
	}
	if got := o.tlsHandshakes.Load(); got != 0 {
		t.Fatalf("tls handshakes = %d, want 0", got)
	}
	if got := o.httpRequests.Load(); got != 0 {
		t.Fatalf("http requests = %d, want 0", got)
	}
}

func newUnsupportedProfileExecutor(t *testing.T, instruction string) ([]*strawpb.StreamFrame, *unsupportedProfileObserver) {
	t.Helper()

	observer := &unsupportedProfileObserver{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		observer.httpRequests.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	exec := NewExecutor(ExecutorOptions{
		Resolver: unsupportedProfileResolver{observer: observer, addr: loopbackIP(t, server.URL)},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			observer.policyDials.Add(1)
			conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err == nil {
				observer.tcpConnects.Add(1)
			}

			return conn, err
		},
	})
	start := requestStart(rewriteHost(t, server.URL, "unsupported.profile.test"), directPolicy(true))
	start.FingerprintInstruction = instruction

	frames := exec.Execute(context.Background(), start, nil, 1, func(frame *strawpb.StreamFrame) {
		if frame.GetOutboundStart() != nil {
			observer.tlsHandshakes.Add(1)
		}
	})

	return frames, observer
}

func TestUnsupportedFingerprintUnknownInstructionNoUpstream(t *testing.T) {
	t.Parallel()

	frames, observer := newUnsupportedProfileExecutor(t, "not_registered")
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT {
		t.Fatalf("error code = %v, want unsupported_fingerprint", got)
	}
	observer.assertZero(t)
}

func TestUnsupportedFingerprintCapabilityDriftNoUpstream(t *testing.T) {
	t.Parallel()

	frames, observer := newUnsupportedProfileExecutor(t, "firefox_121")
	if got := terminalError(t, frames).GetCode(); got != strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT {
		t.Fatalf("error code = %v, want unsupported_fingerprint", got)
	}
	observer.assertZero(t)
}
