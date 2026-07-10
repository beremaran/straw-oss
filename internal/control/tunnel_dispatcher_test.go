package control

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/testutil"
)

func TestTunnelClientEOFSendsCancel(t *testing.T) {
	t.Parallel()

	var sent []*strawpb.StreamFrame
	state := tunnelStreamState{
		c2e: &c2eStreamSender{
			seq:       7,
			publishFn: func(frame *strawpb.StreamFrame) { sent = append(sent, frame) },
		},
	}

	done, perr := state.clientEvent(io.EOF)
	if !done || perr == nil || perr.Code != Cancelled {
		t.Fatalf("clientEvent(io.EOF) = (%v, %+v), want cancelled terminal", done, perr)
	}
	if len(sent) != 1 {
		t.Fatalf("sent frames = %d, want 1", len(sent))
	}
	cancel := sent[0].GetCancel()
	if cancel == nil || cancel.GetReason() != "client_disconnect" {
		t.Fatalf("sent frame = %#v, want client_disconnect cancel", sent[0])
	}
	if sent[0].GetStreamSeq() != 7 {
		t.Fatalf("cancel seq = %d, want 7", sent[0].GetStreamSeq())
	}
}

func TestTunnelUploadGateWaitsForCreditGrant(t *testing.T) {
	t.Parallel()

	gate := newTunnelUploadGate(1)
	defer gate.close()

	done := make(chan bool, 1)
	go func() {
		done <- gate.take(2)
	}()

	select {
	case <-done:
		t.Fatal("take returned before enough upload credit was granted")
	case <-time.After(20 * time.Millisecond):
	}

	gate.grant(1)

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("take returned false after grant")
		}
	case <-time.After(time.Second):
		t.Fatal("take did not resume after upload credit grant")
	}
}

func TestTunnelIdleTimeoutSendsCancel(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	validator := natsx.NewStreamValidator(defaultRequestAttempt, 1, time.Millisecond, func() time.Time {
		return now
	})
	now = now.Add(2 * time.Millisecond)

	var sent []*strawpb.StreamFrame
	state := tunnelStreamState{
		c2e: &c2eStreamSender{
			seq:       3,
			publishFn: func(frame *strawpb.StreamFrame) { sent = append(sent, frame) },
		},
	}

	done, perr := state.idleEvent(validator)
	if !done || perr == nil || perr.Code != TimeoutExceeded || perr.TimeoutType != timeoutTypeIdle {
		t.Fatalf("idleEvent() = (%v, %+v), want idle timeout", done, perr)
	}
	if len(sent) != 1 {
		t.Fatalf("sent frames = %d, want 1", len(sent))
	}
	cancel := sent[0].GetCancel()
	if cancel == nil || cancel.GetReason() != "idle_timeout" {
		t.Fatalf("sent frame = %#v, want idle_timeout cancel", sent[0])
	}
}

func TestTunnelWorkerLossSynthesizesWorkerDisconnected(t *testing.T) {
	t.Parallel()

	failures := &recordingDispatchCandidates{dispatchCandidates: dispatchCandidates{dispatchCandidate()}}
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, failures)
	frames := make(chan *strawpb.StreamFrame)
	close(frames)

	state := tunnelStreamState{
		dispatcher: d,
		route:      dispatchRoute(),
		c2e:        &c2eStreamSender{},
	}
	validator := natsx.NewStreamValidator(defaultRequestAttempt, 1, time.Second, nil)

	done, perr := state.nextTunnelEvent(context.Background(), time.Now().Add(time.Second), nil, frames, validator)
	if !done || perr == nil || perr.Code != WorkerDisconnected {
		t.Fatalf("nextTunnelEvent() = (%v, %+v), want worker_disconnected", done, perr)
	}
	if failures.count != 1 {
		t.Fatalf("worker failures = %d, want 1", failures.count)
	}
}

func TestTunnelNATSDisconnectSynthesizesTransportUnavailable(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	controlConn.Close()

	reg := prometheus.NewRegistry()
	failures := &recordingDispatchCandidates{dispatchCandidates: dispatchCandidates{dispatchCandidate()}}
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, failures)
	d.opts.NATS = controlConn
	d.opts.Metrics = NewMetrics(reg)
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()

	state := tunnelStreamState{
		dispatcher: d,
		route:      dispatchRoute(),
		c2e:        &c2eStreamSender{},
	}
	validator := natsx.NewStreamValidator(defaultRequestAttempt, 1, time.Second, nil)

	done, perr := state.nextTunnelEvent(context.Background(), time.Now().Add(time.Second), ticks, make(chan *strawpb.StreamFrame), validator)
	if !done || perr == nil || perr.Code != TransportUnavailable {
		t.Fatalf("nextTunnelEvent() = (%v, %+v), want transport_unavailable", done, perr)
	}
	if failures.count != 1 {
		t.Fatalf("worker failures = %d, want 1", failures.count)
	}
	errMF := gatherFamily(t, reg, "straw_nats_errors_total")
	if got := counterValue(errMF, map[string]string{metricLabelErrorCode: errorCodeTransportUnavailable}); got != 1 {
		t.Fatalf("nats_errors_total = %v, want 1", got)
	}
}

func TestFingerprintProfileTunnelPreparationCarriesNamedProfileThroughPolicy(t *testing.T) {
	t.Parallel()

	candidate := dispatchCandidate()
	candidate.IngressModes = []string{IngressTypeConnect}
	candidate.SupportedFingerprintProfiles = []string{fingerprintProfileChrome120}
	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{candidate})
	req := validatedDispatchRequest(t, "https://example.com/")
	req.IngressType = IngressTypeConnect
	req.Fingerprint = fingerprintProfileChrome120

	prep, perr := d.prepareTunnelDispatch(context.Background(), dispatchInput(req), time.Now())
	if perr != nil {
		t.Fatalf("prepareTunnelDispatch() error = %#v", perr)
	}
	if prep.policy == nil || prep.policy.FingerprintProfile != fingerprintProfileChrome120 {
		t.Fatalf("prepared fingerprint = %#v, want chrome_120", prep.policy)
	}
}
