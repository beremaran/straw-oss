package http

import (
	"testing"
	"time"

	"github.com/bogdanfinn/fhttp/httptrace"
	tls "github.com/bogdanfinn/utls"

	"github.com/beremaran/straw/internal/protocol/wirepb"
)

func TestRequestTimingTraceRecordsStages(t *testing.T) {
	timing := &wirepb.TimingInfo{}
	recorder := newRequestTiming(timing)

	recorder.trace.DNSStart(httptrace.DNSStartInfo{})
	time.Sleep(time.Nanosecond)
	recorder.trace.DNSDone(httptrace.DNSDoneInfo{})

	recorder.trace.ConnectStart("", "")
	time.Sleep(time.Nanosecond)
	recorder.trace.ConnectDone("", "", nil)

	recorder.trace.TLSHandshakeStart()
	time.Sleep(time.Nanosecond)
	recorder.trace.TLSHandshakeDone(tls.ConnectionState{}, nil)

	recorder.trace.GotFirstResponseByte()

	if timing.GetDnsLookupNanos() <= 0 {
		t.Fatalf("DNSLookupNanos = %d, want > 0", timing.GetDnsLookupNanos())
	}
	if timing.GetTcpConnectNanos() <= 0 {
		t.Fatalf("TcpConnectNanos = %d, want > 0", timing.GetTcpConnectNanos())
	}
	if timing.GetTlsHandshakeNanos() <= 0 {
		t.Fatalf("TlsHandshakeNanos = %d, want > 0", timing.GetTlsHandshakeNanos())
	}
	if timing.GetFirstByteNanos() <= 0 {
		t.Fatalf("FirstByteNanos = %d, want > 0", timing.GetFirstByteNanos())
	}
}
