package http

import (
	"time"

	"github.com/bogdanfinn/fhttp/httptrace"
	tls "github.com/bogdanfinn/utls"

	"github.com/beremaran/straw/internal/protocol/wirepb"
)

type requestTiming struct {
	trace       *httptrace.ClientTrace
	start       time.Time
	dnsStart    time.Time
	connStart   time.Time
	tlsStart    time.Time
	firstByteAt time.Time
	timing      *wirepb.TimingInfo
}

func newRequestTiming(timing *wirepb.TimingInfo) *requestTiming {
	rt := &requestTiming{
		start:  time.Now(),
		timing: timing,
	}
	rt.trace = &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			rt.dnsStart = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			if !rt.dnsStart.IsZero() {
				rt.timing.DnsLookupNanos = time.Since(rt.dnsStart).Nanoseconds()
			}
		},
		ConnectStart: func(_, _ string) {
			rt.connStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			if !rt.connStart.IsZero() {
				rt.timing.TcpConnectNanos = time.Since(rt.connStart).Nanoseconds()
			}
		},
		TLSHandshakeStart: func() {
			rt.tlsStart = time.Now()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			if !rt.tlsStart.IsZero() {
				rt.timing.TlsHandshakeNanos = time.Since(rt.tlsStart).Nanoseconds()
			}
		},
		GotFirstResponseByte: func() {
			rt.firstByteAt = time.Now()
			rt.timing.FirstByteNanos = rt.firstByteAt.Sub(rt.start).Nanoseconds()
		},
	}

	return rt
}

func (rt *requestTiming) finish() {
	rt.timing.TotalNanos = time.Since(rt.start).Nanoseconds()
}
