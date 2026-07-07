package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/egress"
	"github.com/beremaran/straw/v2/internal/natsx"
	"github.com/beremaran/straw/v2/internal/testutil"
)

const (
	dispatchTestTenant = "ten_dispatch"
	dispatchTestKey    = "key_dispatch"
	dispatchTestWorker = "worker_dispatch"
	dispatchTestSess   = "session_dispatch"
	dispatchTestPool   = "pool_dispatch"
	dispatchPartial    = "partial"
)

func TestDispatcherRouteNoMatch(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(t, nil, dispatchCandidates{})
	req := validatedDispatchRequest(t, "https://example.com/")

	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != RouteNoMatch {
		t.Fatalf("Dispatch error = %#v, want route_no_match", perr)
	}
}

func TestDispatcherRouteUnavailable(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, dispatchCandidates{})
	req := validatedDispatchRequest(t, "https://example.com/")

	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != RouteUnavailable {
		t.Fatalf("Dispatch error = %#v, want route_unavailable", perr)
	}
}

// TestDispatcherFailureCarriesPartialTiming verifies a PipelineError from a
// post-routing failure still carries the routing phase timing it actually
// measured (docs/tasks/p0/32), instead of leaving request_events with all-zero
// timings on failure.
func TestDispatcherFailureCarriesPartialTiming(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, dispatchCandidates{})
	req := validatedDispatchRequest(t, "https://example.com/")

	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != RouteUnavailable {
		t.Fatalf("Dispatch error = %#v, want route_unavailable", perr)
	}
	if perr.TotalMs < 0 {
		t.Fatalf("TotalMs = %d, want >= 0", perr.TotalMs)
	}
	if perr.AssignmentMs != 0 {
		t.Fatalf("AssignmentMs = %d, want 0 (failure happened before assignment)", perr.AssignmentMs)
	}
}

// TestDispatcherRoutePoolPolicyFromSnapshot verifies degraded-pool policy is
// sourced from the tenant snapshot's executor pools (docs/tasks/p0/30)
// instead of the previous NewStaticPoolPolicyProvider(nil) that always
// rejected degraded workers.
func TestDispatcherRoutePoolPolicyFromSnapshot(t *testing.T) {
	t.Parallel()

	degradedCandidate := dispatchCandidate()
	degradedCandidate.Degraded = true

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{degradedCandidate})
	req := validatedDispatchRequest(t, "https://example.com/")

	// No executor pool configured: unknown-pool policy defaults to
	// AllowDegradedWorkers=false, so the only (degraded) candidate is rejected.
	outcome := d.route(dispatchInput(req), snapshot)
	if outcome.OK {
		t.Fatalf("route outcome = %+v, want unavailable with no pool policy", outcome)
	}

	// A pool with allow_degraded_workers=true in the snapshot admits the
	// degraded candidate.
	snapshot.ExecutorPools = []config.ExecutorPool{
		{ID: dispatchTestPool, Enabled: true, AllowDegradedWorkers: true},
	}

	outcome = d.route(dispatchInput(req), snapshot)
	if !outcome.OK || outcome.WorkerID != dispatchTestWorker {
		t.Fatalf("route outcome = %+v, want OK with worker %s", outcome, dispatchTestWorker)
	}
}

// TestDispatcherRoutePoolCapabilityRestriction verifies a pool's
// allowed_countries restriction (docs/planning/26, docs/tasks/p0/42) excludes
// a candidate whose claimed country is outside the restriction and admits a
// matching candidate.
func TestDispatcherRoutePoolCapabilityRestriction(t *testing.T) {
	t.Parallel()

	outside := dispatchCandidate()
	outside.Countries = []string{"FR"}

	matching := dispatchCandidate()
	matching.WorkerID = "worker_dispatch_match"
	matching.SessionID = "session_dispatch_match"
	matching.AssignSubject = "straw.v1.executor." + matching.WorkerID + "." + matching.SessionID + ".assign"
	matching.Countries = []string{"US"}

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	snapshot.ExecutorPools = []config.ExecutorPool{
		{ID: dispatchTestPool, Enabled: true, AllowedCountries: []string{"US"}},
	}

	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{outside, matching})
	req := validatedDispatchRequest(t, "https://example.com/")

	outcome := d.route(dispatchInput(req), snapshot)
	if !outcome.OK || outcome.WorkerID != matching.WorkerID {
		t.Fatalf("route outcome = %+v, want OK with worker %s (outside-restriction candidate excluded)", outcome, matching.WorkerID)
	}
}

func TestDispatcherRoutesUsingRequestIngressType(t *testing.T) {
	t.Parallel()

	snapshot := dispatchSnapshot([]config.RoutingRule{
		{
			ID: "rule_proxy", Priority: 1, Enabled: true, TargetPoolID: dispatchTestPool,
			Match: config.MatchConditions{IngressType: IngressTypeHTTPProxy},
		},
		{
			ID: "rule_rest", Priority: 2, Enabled: true, TargetPoolID: dispatchTestPool,
			Match: config.MatchConditions{IngressType: IngressTypeREST},
		},
	})

	proxyCandidate := dispatchCandidate()
	proxyCandidate.IngressModes = []string{IngressTypeHTTPProxy}

	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{proxyCandidate})
	req := validatedDispatchRequest(t, "https://example.com/")
	req.IngressType = IngressTypeHTTPProxy

	outcome := d.route(dispatchInput(req), snapshot)
	if !outcome.OK || outcome.RuleID != "rule_proxy" || outcome.WorkerID != proxyCandidate.WorkerID {
		t.Fatalf("route outcome = %+v, want http_proxy rule and candidate", outcome)
	}
}

func TestDispatcherRateLimitRetryAfter(t *testing.T) {
	client := newTestRedisClient(t)

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	snapshot.RateLimits = []config.RateLimitRule{
		{Dimension: string(RateLimitDimTenant), WindowSeconds: 60, MaxRequests: 1, FailPolicy: string(RateLimitFailOpen)},
	}

	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{dispatchCandidate()})
	req := validatedDispatchRequest(t, "https://example.com/")
	d.opts.RateLimitAdmission = NewRateLimitAdmission(NewRateLimiter(client, DefaultRateLimitGuardrails(), nil))

	_, first := d.Dispatch(context.Background(), dispatchInput(req))
	if first == nil || first.Code != TransportUnavailable {
		t.Fatalf("first Dispatch error = %#v, want transport_unavailable after admission", first)
	}

	_, second := d.Dispatch(context.Background(), dispatchInput(req))
	if second == nil || second.Code != RateLimitExceeded || second.RetryAfterMs <= 0 {
		t.Fatalf("second Dispatch error = %#v, want rate_limit_exceeded with retry_after", second)
	}
}

func TestDispatcherResponseBodyTooLarge(t *testing.T) {
	t.Parallel()

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("too large"))
	}))
	defer stop()
	d.opts.MaxInlineResponseBodyBytes = 3

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != BodyTooLarge {
		t.Fatalf("Dispatch error = %#v, want body_too_large", perr)
	}
}

func TestDispatcherResponseBodyRefUnavailableWhenSelectedBackendUnwired(t *testing.T) {
	t.Parallel()

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("too large"))
	}))
	defer stop()
	d.opts.MaxInlineResponseBodyBytes = 3
	d.opts.BodyTransport = config.ControlBodyTransportConfig{
		LargeBodyThresholdBytes: 3,
		ObjectStorage:           config.BodyObjectStorageConfig{Enabled: true},
	}.Normalized()

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != BodyRefUnavailable {
		t.Fatalf("Dispatch error = %#v, want body_ref_unavailable", perr)
	}
	if perr.Details[errorDetailDirectionKey] != "response" || perr.Details["transport"] != string(BodyTransportS3BodyRef) {
		t.Fatalf("error details = %#v, want response s3 body ref", perr.Details)
	}
}

func TestDispatcherControlNATSEgressRoundTrip(t *testing.T) {
	t.Parallel()

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Upstream", "real")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("real body"))
	}))
	defer stop()

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	req.Method = http.MethodPost
	req.BodyData = []byte("request body")
	req.Headers = []HeaderPair{{Name: headerCanonicalContentType, Value: base64.StdEncoding.EncodeToString([]byte(mediaTypeTextPlain))}}

	resp, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr != nil {
		t.Fatalf("Dispatch error = %#v", perr)
	}
	if resp.Status != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", resp.Status, http.StatusTeapot)
	}
	if got, _ := base64.StdEncoding.DecodeString(resp.Body.DataBase64); string(got) != "real body" {
		t.Fatalf("body = %q, want real body", got)
	}
	if len(resp.Headers) == 0 {
		t.Fatal("headers empty, want upstream headers")
	}
}

func TestDispatcherControlNATSEgressRoundTripIDNAHost(t *testing.T) {
	t.Parallel()

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("idna body"))
	}))
	defer stop()

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "bücher.example"))
	resp, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr != nil {
		t.Fatalf("Dispatch error = %#v", perr)
	}
	if resp.Status != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", resp.Status, http.StatusTeapot)
	}
	if got, _ := base64.StdEncoding.DecodeString(resp.Body.DataBase64); string(got) != "idna body" {
		t.Fatalf("body = %q, want idna body", got)
	}
}

// TestDispatcherEgressPhaseTiming reproduces the 2026-07-05 live-stack gap
// (docs/tasks/p0/41): a successful dispatch against an upstream with a real
// delay must record egress_ms reflecting that delay, not 0, and the phase
// timings must sum consistently toward total_ms.
func TestDispatcherEgressPhaseTiming(t *testing.T) {
	t.Parallel()

	const upstreamDelay = 100 * time.Millisecond

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(upstreamDelay)
		_, _ = w.Write([]byte("delayed"))
	}))
	defer stop()

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))

	resp, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr != nil {
		t.Fatalf("Dispatch error = %#v", perr)
	}

	timing := resp.Timing
	if timing.EgressMs < int64(upstreamDelay/time.Millisecond)/2 {
		t.Fatalf("EgressMs = %d, want >= %d (upstream slept %v)", timing.EgressMs, int64(upstreamDelay/time.Millisecond)/2, upstreamDelay)
	}
	if timing.TotalMs < timing.EgressMs {
		t.Fatalf("TotalMs = %d < EgressMs = %d", timing.TotalMs, timing.EgressMs)
	}
	if timing.RoutingMs+timing.AssignmentMs+timing.EgressMs > timing.TotalMs {
		t.Fatalf("phase sum %d+%d+%d exceeds TotalMs %d", timing.RoutingMs, timing.AssignmentMs, timing.EgressMs, timing.TotalMs)
	}
}

func TestDispatcherRawResponseStreamsPastInlineLimit(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", (1<<20)+16384)
	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerCanonicalContentType, mediaTypeTextPlain)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(body))
	}))
	defer stop()

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	w := httptest.NewRecorder()

	resp, perr, wroteHeader := d.DispatchRaw(context.Background(), dispatchInput(req), w)
	if perr != nil {
		t.Fatalf("DispatchRaw error = %#v", perr)
	}
	if !wroteHeader {
		t.Fatal("DispatchRaw wroteHeader = false, want true")
	}
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if got := w.Body.Len(); got != len(body) {
		t.Fatalf("body len = %d, want %d", got, len(body))
	}
	if resp.ResponseSizeBytes != uint64(len(body)) {
		t.Fatalf("response size = %d, want %d", resp.ResponseSizeBytes, len(body))
	}
}

func TestRawResponseHeaderAndTrailerFiltering(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	writeRawResponseStart(w, http.StatusAccepted, []*strawpb.Header{
		{Name: headerCanonicalContentType, Value: []byte(mediaTypeTextPlain)},
		{Name: headerCanonicalContentLength, Value: []byte("10")},
		{Name: "X-Straw-Debug", Value: []byte("drop")},
	})
	_, _ = w.Write([]byte("ok"))
	writeRawTrailers(w, []*strawpb.Header{
		{Name: "X-Trailer", Value: []byte("yes")},
		{Name: headerCanonicalConnection, Value: []byte("drop")},
	})

	res := w.Result()
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusAccepted)
	}
	if got := res.Header.Get(headerCanonicalContentType); got != mediaTypeTextPlain {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	for _, name := range []string{headerCanonicalContentLength, "X-Straw-Debug", headerCanonicalConnection} {
		if got := res.Header.Get(name); got != "" {
			t.Fatalf("%s = %q, want stripped", name, got)
		}
	}
	if got := res.Trailer.Get("X-Trailer"); got != "yes" {
		t.Fatalf("X-Trailer = %q, want yes", got)
	}
}

func TestDispatcherRawCancellationPublishesCancelFrame(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	workerConn := dispatchConnect(t, natsServer.URL())
	cancelReason := make(chan string, 1)

	assignSubject, err := natsx.AssignmentSubject(dispatchTestWorker, dispatchTestSess)
	if err != nil {
		t.Fatalf("AssignmentSubject: %v", err)
	}

	_, err = workerConn.Subscribe(assignSubject, func(msg *nats.Msg) {
		env, decodeErr := natsx.UnmarshalEnvelope(msg.Data)
		if decodeErr != nil {
			return
		}

		c2eSubj, _ := natsx.StreamSubject(env.GetRequestId(), dispatchTestWorker, dispatchTestSess, natsx.DirectionControlToExecutor)
		_, _ = workerConn.Subscribe(c2eSubj, func(msg *nats.Msg) {
			frame := decodeDispatchFrame(msg.Data)
			if cancel := frame.GetCancel(); cancel != nil {
				cancelReason <- cancel.GetReason()
			}
		})
		_ = workerConn.Flush()

		ack := &strawpb.Envelope{
			RequestId:     env.GetRequestId(),
			TenantId:      env.GetTenantId(),
			ProtocolMajor: ProtocolMajor,
			Attempt:       env.GetAttempt(),
			Payload:       &strawpb.Envelope_AssignAck{AssignAck: &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED}},
		}
		raw, _ := natsx.MarshalEnvelope(ack)
		_ = msg.Respond(raw)

		e2cSubj, _ := natsx.StreamSubject(env.GetRequestId(), dispatchTestWorker, dispatchTestSess, natsx.DirectionExecutorToControl)
		publishDispatchFrames(workerConn, env, e2cSubj,
			&strawpb.StreamFrame{StreamSeq: 1, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusOK}}},
			&strawpb.StreamFrame{StreamSeq: 2, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Data: []byte(dispatchPartial)}}},
		)
	})
	if err != nil {
		t.Fatalf("Subscribe assignment: %v", err)
	}
	_ = workerConn.Flush()

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = controlConn

	ctx, cancel := context.WithCancel(context.Background())
	req := validatedDispatchRequest(t, "https://example.com/")
	w := httptest.NewRecorder()

	done := make(chan *PipelineError, 1)
	go func() {
		_, perr, _ := d.DispatchRaw(ctx, dispatchInput(req), w)
		done <- perr
	}()

	for !bytes.Contains(w.Body.Bytes(), []byte(dispatchPartial)) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	perr := <-done
	if perr == nil || perr.Code != Cancelled {
		t.Fatalf("DispatchRaw error = %#v, want cancelled", perr)
	}

	select {
	case got := <-cancelReason:
		if got != "client_cancelled" {
			t.Fatalf("cancel reason = %q, want client_cancelled", got)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not receive CancelFrame")
	}
}

func TestDispatcherRawPostHeaderErrorKeepsPartialResponse(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	workerConn := dispatchConnect(t, natsServer.URL())

	assignSubject, err := natsx.AssignmentSubject(dispatchTestWorker, dispatchTestSess)
	if err != nil {
		t.Fatalf("AssignmentSubject: %v", err)
	}

	_, err = workerConn.Subscribe(assignSubject, func(msg *nats.Msg) {
		env, decodeErr := natsx.UnmarshalEnvelope(msg.Data)
		if decodeErr != nil {
			return
		}

		c2eSubj, _ := natsx.StreamSubject(env.GetRequestId(), dispatchTestWorker, dispatchTestSess, natsx.DirectionControlToExecutor)
		_, _ = workerConn.Subscribe(c2eSubj, func(*nats.Msg) {})
		_ = workerConn.Flush()

		ack := &strawpb.Envelope{
			RequestId:     env.GetRequestId(),
			TenantId:      env.GetTenantId(),
			ProtocolMajor: ProtocolMajor,
			Attempt:       env.GetAttempt(),
			Payload:       &strawpb.Envelope_AssignAck{AssignAck: &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED}},
		}
		raw, _ := natsx.MarshalEnvelope(ack)
		_ = msg.Respond(raw)

		e2cSubj, _ := natsx.StreamSubject(env.GetRequestId(), dispatchTestWorker, dispatchTestSess, natsx.DirectionExecutorToControl)
		publishDispatchFrames(workerConn, env, e2cSubj,
			&strawpb.StreamFrame{StreamSeq: 1, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusOK}}},
			&strawpb.StreamFrame{StreamSeq: 2, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Data: []byte(dispatchPartial)}}},
			&strawpb.StreamFrame{StreamSeq: 3, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_Error{Error: &strawpb.ErrorFrame{Code: strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET}}},
		)
	})
	if err != nil {
		t.Fatalf("Subscribe assignment: %v", err)
	}
	_ = workerConn.Flush()

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = controlConn

	req := validatedDispatchRequest(t, "https://example.com/")
	w := httptest.NewRecorder()

	_, perr, wroteHeader := d.DispatchRaw(context.Background(), dispatchInput(req), w)
	if perr == nil || perr.Code != UpstreamReset {
		t.Fatalf("DispatchRaw error = %#v, want upstream_reset", perr)
	}
	if !wroteHeader {
		t.Fatal("DispatchRaw wroteHeader = false, want true")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want upstream status 200", w.Code)
	}
	if got := w.Body.String(); got != dispatchPartial {
		t.Fatalf("body = %q, want partial", got)
	}
}

type liveDispatchHarness struct {
	*DefaultRequestDispatcher
	upstreamURL string
}

func newLiveDispatchHarness(t *testing.T, upstreamHandler http.Handler) (*liveDispatchHarness, func()) {
	t.Helper()

	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	workerConn := dispatchConnect(t, natsServer.URL())

	resolver := dispatchResolver{
		"dispatch.test":         loopbackDispatchIP(t, upstream.URL),
		"xn--bcher-kva.example": loopbackDispatchIP(t, upstream.URL),
	}
	worker, err := egress.NewWorker(workerConn, egress.Identity{WorkerID: dispatchTestWorker}, egress.NewExecutor(egress.ExecutorOptions{Resolver: resolver}), dispatchTestSess, 4)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	stopWorker := make(chan struct{})
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = worker.Serve(stopWorker)
	}()
	stop := sync.OnceFunc(func() {
		close(stopWorker)
		<-served
	})
	t.Cleanup(stop)

	client := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr})
	t.Cleanup(func() { _ = client.Close() })

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	snapshot.DenyRules = []config.DenyRule{
		{RuleType: denyRuleTypeCIDR, Action: denyRuleActionAllowOverride, Enabled: true, NormalizedCIDR: loopbackDispatchCIDR(t, upstream.URL)},
	}

	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = controlConn
	d.opts.QuotaAdmission = NewQuotaAdmission(client, nil)
	d.opts.RateLimitAdmission = NewRateLimitAdmission(NewRateLimiter(client, DefaultRateLimitGuardrails(), nil))

	return &liveDispatchHarness{DefaultRequestDispatcher: d, upstreamURL: upstream.URL}, stop
}

func newTestDispatcher(t *testing.T, rules []config.RoutingRule, candidates CandidateSource) *DefaultRequestDispatcher {
	t.Helper()

	return newTestDispatcherWithSnapshot(t, dispatchSnapshot(rules), candidates)
}

func newTestDispatcherWithSnapshot(t *testing.T, snapshot config.TenantSnapshot, candidates CandidateSource) *DefaultRequestDispatcher {
	t.Helper()

	store := NewInMemorySnapshotStore()
	_, err := store.SaveTenantSnapshot(context.Background(), snapshot, 0)
	if err != nil {
		t.Fatalf("SaveTenantSnapshot() error = %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr})
	t.Cleanup(func() { _ = client.Close() })

	return NewDefaultRequestDispatcher(RequestDispatcherOptions{
		ConfigCache:                NewConfigCache(store, nil),
		Workers:                    candidates,
		Sticky:                     NewStickyStore(nil),
		NATS:                       nil,
		RateLimitAdmission:         NewRateLimitAdmission(NewRateLimiter(client, DefaultRateLimitGuardrails(), nil)),
		QuotaAdmission:             NewQuotaAdmission(client, nil),
		MaxInlineResponseBodyBytes: 1 << 20,
		MaxFrameDataBytes:          1 << 20,
		MaxTimeoutMs:               5000,
	})
}

func TestDispatcherDefaultTimeoutUsesTenantPolicyClampedByStaticMax(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	d := NewDefaultRequestDispatcher(RequestDispatcherOptions{
		MaxTimeoutMs: 5000,
		Now:          func() time.Time { return now },
	})

	got := d.deadline(&ValidatedRequest{}, config.TenantSnapshot{DefaultTimeoutMs: 7000}).Sub(now)
	if got != 5*time.Second {
		t.Fatalf("deadline with static clamp = %s, want 5s", got)
	}

	got = d.deadline(&ValidatedRequest{}, config.TenantSnapshot{DefaultTimeoutMs: 3000}).Sub(now)
	if got != 3*time.Second {
		t.Fatalf("deadline with tenant default = %s, want 3s", got)
	}
}

func dispatchSnapshot(rules []config.RoutingRule) config.TenantSnapshot {
	return config.TenantSnapshot{
		TenantID:      dispatchTestTenant,
		ConfigVersion: 1,
		RoutingRules:  rules,
		FingerprintProfiles: []config.FingerprintProfile{
			{Name: "default", ScopeType: "global", Enabled: true, SupportedByWorker: true},
		},
		Quota: config.QuotaConfig{Enabled: false, CountOnAdmission: true, FailPolicy: "open"},
	}
}

func dispatchRule() config.RoutingRule {
	return config.RoutingRule{ID: "rule_dispatch", Priority: 1, Enabled: true, TargetPoolID: dispatchTestPool}
}

func dispatchCandidate() PoolCandidate {
	return PoolCandidate{
		WorkerID:       dispatchTestWorker,
		SessionID:      dispatchTestSess,
		AssignSubject:  "straw.v1.executor." + dispatchTestWorker + "." + dispatchTestSess + ".assign",
		IngressModes:   []string{IngressTypeREST},
		MaxConcurrency: 4,
		AvailableCap:   4,
	}
}

type dispatchCandidates []PoolCandidate

func (c dispatchCandidates) CandidatesForPool(_, poolID string) []PoolCandidate {
	if poolID != dispatchTestPool {
		return nil
	}

	return append([]PoolCandidate(nil), c...)
}

type recordingDispatchCandidates struct {
	dispatchCandidates
	count int
}

func (c *recordingDispatchCandidates) RecordFailure(workerID string) {
	if workerID == dispatchTestWorker {
		c.count++
	}
}

func dispatchRoute() RouteOutcome {
	candidate := dispatchCandidate()

	return RouteOutcome{
		OK:            true,
		RuleID:        "rule_dispatch",
		PoolID:        dispatchTestPool,
		WorkerID:      dispatchTestWorker,
		SessionID:     dispatchTestSess,
		AssignSubject: candidate.AssignSubject,
	}
}

func dispatchInput(req *ValidatedRequest) DispatchInput {
	return DispatchInput{
		RequestID: "req_dispatch",
		Identity:  Identity{APIKeyID: dispatchTestKey, ScopeType: ScopeTenant, TenantID: dispatchTestTenant, Role: RoleRequester},
		Request:   req,
	}
}

func respBodyBytes(t *testing.T, resp SuccessResponse) []byte {
	t.Helper()

	body, err := base64.StdEncoding.DecodeString(resp.Body.DataBase64)
	if err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	return body
}

func validatedDispatchRequest(t *testing.T, rawURL string) *ValidatedRequest {
	t.Helper()

	body := `{"method":"GET","url":"` + rawURL + `","timeout_ms":5000}`
	req, err := ValidateRequest([]byte(body), 1<<20, 5000)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}

	return req
}

type dispatchResolver map[string]netip.Addr

func (r dispatchResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addr, ok := r[host]
	if !ok {
		return nil, nil
	}

	return []net.IPAddr{{IP: net.ParseIP(addr.String())}}, nil
}

func (r dispatchResolver) LookupCNAME(_ context.Context, host string) ([]string, error) {
	return []string{host}, nil
}

func dispatchConnect(t *testing.T, url string) *natsx.Connection {
	t.Helper()

	conn, err := natsx.Connect(natsx.ConnectOptions{Servers: []string{url}})
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return conn
}

func publishDispatchFrames(conn *natsx.Connection, env *strawpb.Envelope, subject string, frames ...*strawpb.StreamFrame) {
	for _, frame := range frames {
		out := &strawpb.Envelope{
			RequestId:      env.GetRequestId(),
			TenantId:       env.GetTenantId(),
			DeadlineUnixMs: env.GetDeadlineUnixMs(),
			ProtocolMajor:  ProtocolMajor,
			Attempt:        env.GetAttempt(),
			Payload:        &strawpb.Envelope_StreamFrame{StreamFrame: frame},
		}
		raw, _ := natsx.MarshalEnvelope(out)
		_ = conn.Publish(subject, raw)
	}
	_ = conn.Flush()
}

func rewriteDispatchHost(t *testing.T, raw, host string) string {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}

	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split upstream host: %v", err)
	}

	u.Host = net.JoinHostPort(host, port)

	return u.String()
}

func loopbackDispatchIP(t *testing.T, raw string) netip.Addr {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split upstream host: %v", err)
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		t.Fatalf("parse upstream addr: %v", err)
	}

	return addr
}

func loopbackDispatchCIDR(t *testing.T, raw string) string {
	t.Helper()

	addr := loopbackDispatchIP(t, raw)
	if strings.Contains(addr.String(), ":") {
		return addr.String() + "/128"
	}

	return addr.String() + "/32"
}

// TestDispatcherNATSUnavailable verifies the docs/planning/29 outage row:
// when the NATS transport is unavailable, new request dispatch fails with
// transport_unavailable (after routing and admission succeed).
func TestDispatcherNATSUnavailable(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = nil // no transport available

	req := validatedDispatchRequest(t, "https://example.com/")

	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != TransportUnavailable {
		t.Fatalf("Dispatch error = %#v, want transport_unavailable", perr)
	}
}

// TestDispatcherAssignmentTimeout verifies that when no egress worker is
// listening on the assignment subject, Dispatch returns AssignmentTimeout.
func TestDispatcherAssignmentTimeout(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = controlConn
	d.opts.AssignmentAckTimeout = 200 * time.Millisecond

	req := validatedDispatchRequest(t, "https://example.com/")
	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != AssignmentTimeout {
		t.Fatalf("Dispatch error = %#v, want assignment_timeout", perr)
	}
}

func TestDispatcherAssignmentNATSOutageSynthesizesTransportUnavailableAndMetric(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	controlConn.Close()

	reg := prometheus.NewRegistry()
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = controlConn
	d.opts.Metrics = NewMetrics(reg)

	req := validatedDispatchRequest(t, "https://example.com/")
	in := dispatchInput(req)
	_, perr := d.requestAssign(dispatchCandidate().AssignSubject, in, d.assignRequest(in, dispatchRoute(), 1, time.Now().Add(time.Second)), time.Now().Add(time.Second))
	if perr == nil || perr.Code != TransportUnavailable {
		t.Fatalf("requestAssign error = %#v, want transport_unavailable", perr)
	}
	errMF := gatherFamily(t, reg, "straw_nats_errors_total")
	if got := counterValue(errMF, map[string]string{metricLabelErrorCode: errorCodeTransportUnavailable}); got != 1 {
		t.Fatalf("nats_errors_total = %v, want 1", got)
	}
}

func TestDispatcherWorkerLossBeforeRequestStartFallsBackToAlternateWorker(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	workerConn := dispatchConnect(t, natsServer.URL())

	alternate := dispatchCandidate()
	alternate.WorkerID = "worker_dispatch_alt"
	alternate.SessionID = "session_dispatch_alt"
	alternate.AssignSubject = "straw.v1.executor." + alternate.WorkerID + "." + alternate.SessionID + ".assign"

	// The first selected worker has no assignment subscriber, which is the
	// observable Core NATS shape for worker loss before RequestStart.
	_, err := workerConn.Subscribe(alternate.AssignSubject, func(msg *nats.Msg) {
		env, decodeErr := natsx.UnmarshalEnvelope(msg.Data)
		if decodeErr != nil {
			return
		}

		c2eSubj, _ := natsx.StreamSubject(env.GetRequestId(), alternate.WorkerID, alternate.SessionID, natsx.DirectionControlToExecutor)
		_, _ = workerConn.Subscribe(c2eSubj, func(*nats.Msg) {})
		_ = workerConn.Flush()

		ack := &strawpb.Envelope{
			RequestId:     env.GetRequestId(),
			TenantId:      env.GetTenantId(),
			ProtocolMajor: ProtocolMajor,
			Attempt:       env.GetAttempt(),
			Payload:       &strawpb.Envelope_AssignAck{AssignAck: &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED}},
		}
		raw, _ := natsx.MarshalEnvelope(ack)
		_ = msg.Respond(raw)

		e2cSubj, _ := natsx.StreamSubject(env.GetRequestId(), alternate.WorkerID, alternate.SessionID, natsx.DirectionExecutorToControl)
		publishDispatchFrames(workerConn, env, e2cSubj,
			&strawpb.StreamFrame{StreamSeq: 1, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusAccepted}}},
			&strawpb.StreamFrame{StreamSeq: 2, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Data: []byte("fallback")}}},
			&strawpb.StreamFrame{StreamSeq: 3, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_End{End: &strawpb.EndFrame{}}},
		)
	})
	if err != nil {
		t.Fatalf("Subscribe alternate assignment: %v", err)
	}
	_ = workerConn.Flush()

	failures := &recordingDispatchCandidates{dispatchCandidates: dispatchCandidates{dispatchCandidate(), alternate}}
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, failures)
	d.opts.NATS = controlConn
	d.opts.AssignmentAckTimeout = 100 * time.Millisecond

	req := validatedDispatchRequest(t, "https://example.com/")
	resp, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr != nil {
		t.Fatalf("Dispatch error = %#v, want fallback success", perr)
	}
	if resp.Status != http.StatusAccepted || string(respBodyBytes(t, resp)) != "fallback" {
		t.Fatalf("fallback response = status %d body %q, want 202 fallback", resp.Status, string(respBodyBytes(t, resp)))
	}
	if failures.count != 1 {
		t.Fatalf("first worker failures = %d, want 1", failures.count)
	}
}

// TestDispatcherStreamProtocolError verifies that a sequence gap in e2c frames
// returns ProtocolError.
func TestDispatcherStreamProtocolError(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	workerConn := dispatchConnect(t, natsServer.URL())

	// A stub upstream that is never reached: the worker replies ACCEPTED and
	// then sends a frame with stream_seq=99 (a sequence gap).
	assignSubject, err := natsx.AssignmentSubject(dispatchTestWorker, dispatchTestSess)
	if err != nil {
		t.Fatalf("AssignmentSubject: %v", err)
	}

	_, err = workerConn.Subscribe(assignSubject, func(msg *nats.Msg) {
		env, decodeErr := natsx.UnmarshalEnvelope(msg.Data)
		if decodeErr != nil {
			return
		}
		// Subscribe to c2e so the flush from sendRequestStart doesn't block.
		c2eSubj, _ := natsx.StreamSubject(env.GetRequestId(), dispatchTestWorker, dispatchTestSess, natsx.DirectionControlToExecutor)
		_, _ = workerConn.Subscribe(c2eSubj, func(*nats.Msg) {})
		_ = workerConn.Flush()

		// Reply with ACCEPTED.
		ack := &strawpb.Envelope{
			RequestId:     env.GetRequestId(),
			TenantId:      env.GetTenantId(),
			ProtocolMajor: ProtocolMajor,
			Attempt:       env.GetAttempt(),
			Payload:       &strawpb.Envelope_AssignAck{AssignAck: &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED}},
		}
		raw, _ := natsx.MarshalEnvelope(ack)
		_ = msg.Respond(raw)

		// Send a frame with a sequence gap (seq=99 instead of seq=1).
		e2cSubj, _ := natsx.StreamSubject(env.GetRequestId(), dispatchTestWorker, dispatchTestSess, natsx.DirectionExecutorToControl)
		bad := &strawpb.Envelope{
			RequestId:     env.GetRequestId(),
			TenantId:      env.GetTenantId(),
			ProtocolMajor: ProtocolMajor,
			Attempt:       env.GetAttempt(),
			Payload: &strawpb.Envelope_StreamFrame{StreamFrame: &strawpb.StreamFrame{
				StreamSeq: 99,
				Attempt:   defaultRequestAttempt,
				Payload:   &strawpb.StreamFrame_End{End: &strawpb.EndFrame{}},
			}},
		}
		badRaw, _ := natsx.MarshalEnvelope(bad)
		_ = workerConn.Publish(e2cSubj, badRaw)
		_ = workerConn.Flush()
	})
	if err != nil {
		t.Fatalf("Subscribe assignment: %v", err)
	}
	_ = workerConn.Flush()

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = controlConn

	req := validatedDispatchRequest(t, "https://example.com/")
	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != ProtocolError {
		t.Fatalf("Dispatch error = %#v, want protocol_error", perr)
	}
}

func TestDispatcherWorkerLossAfterRequestStartSynthesizesWorkerDisconnected(t *testing.T) {
	t.Parallel()

	failures := &recordingDispatchCandidates{dispatchCandidates: dispatchCandidates{dispatchCandidate()}}
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, failures)
	frames := make(chan *strawpb.StreamFrame)
	close(frames)

	_, perr := d.readResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2)
	if perr == nil || perr.Code != WorkerDisconnected {
		t.Fatalf("readResponse error = %#v, want worker_disconnected", perr)
	}
	if failures.count != 1 {
		t.Fatalf("worker failures = %d, want 1", failures.count)
	}
}

func TestDispatcherWorkerLossAfterPartialResponseSynthesizesWorkerDisconnected(t *testing.T) {
	t.Parallel()

	failures := &recordingDispatchCandidates{dispatchCandidates: dispatchCandidates{dispatchCandidate()}}
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, failures)
	frames := make(chan *strawpb.StreamFrame, 3)
	frames <- &strawpb.StreamFrame{StreamSeq: 1, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{}}}
	frames <- &strawpb.StreamFrame{StreamSeq: 2, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusOK}}}
	frames <- &strawpb.StreamFrame{StreamSeq: 3, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Data: []byte(dispatchPartial)}}}
	close(frames)

	w := httptest.NewRecorder()
	result, perr, wroteHeader := d.streamRawResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2, w)
	if perr == nil || perr.Code != WorkerDisconnected {
		t.Fatalf("streamRawResponse error = %#v, want worker_disconnected", perr)
	}
	if !wroteHeader || result.size != uint64(len(dispatchPartial)) || w.Body.String() != dispatchPartial {
		t.Fatalf("partial raw response = wroteHeader %v size %d body %q, want partial response preserved", wroteHeader, result.size, w.Body.String())
	}
	if failures.count != 1 {
		t.Fatalf("worker failures = %d, want 1", failures.count)
	}
}

func TestDispatcherReplenishesDownloadCreditForDecodedResponse(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	c2eSub, err := controlConn.SubscribeSync("c2e")
	if err != nil {
		t.Fatalf("SubscribeSync c2e: %v", err)
	}
	err = controlConn.Flush()
	if err != nil {
		t.Fatalf("Flush c2e subscription: %v", err)
	}

	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, dispatchCandidates{})
	d.opts.NATS = controlConn
	d.opts.InitialDownloadCreditBytes = 2
	d.opts.MaxInflightDownloadBytes = 1

	frames := make(chan *strawpb.StreamFrame, 5)
	frames <- &strawpb.StreamFrame{StreamSeq: 1, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{}}}
	frames <- &strawpb.StreamFrame{StreamSeq: 2, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusOK}}}
	frames <- &strawpb.StreamFrame{StreamSeq: 3, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 0, Data: []byte("a")}}}
	frames <- &strawpb.StreamFrame{StreamSeq: 4, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 1, Data: []byte("b")}}}
	frames <- &strawpb.StreamFrame{StreamSeq: 5, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}}}

	result, perr := d.readResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2)
	if perr != nil {
		t.Fatalf("readResponse error = %#v", perr)
	}
	if string(result.body) != "ab" {
		t.Fatalf("body = %q, want ab", result.body)
	}

	msg, err := c2eSub.NextMsg(time.Second)
	if err != nil {
		t.Fatalf("NextMsg credit: %v", err)
	}
	env, err := natsx.UnmarshalEnvelope(msg.Data)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope credit: %v", err)
	}
	credit := env.GetStreamFrame().GetCredit()
	if credit == nil || credit.GetDownloadCreditBytes() != 1 {
		t.Fatalf("credit frame = %#v, want 1 download byte", env.GetStreamFrame())
	}
}

func TestDispatcherInFlightNATSDisconnectSynthesizesTransportUnavailable(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	controlConn := dispatchConnect(t, natsServer.URL())
	controlConn.Close()

	reg := prometheus.NewRegistry()
	failures := &recordingDispatchCandidates{dispatchCandidates: dispatchCandidates{dispatchCandidate()}}
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, failures)
	d.opts.NATS = controlConn
	d.opts.Metrics = NewMetrics(reg)
	frames := make(chan *strawpb.StreamFrame)

	_, perr := d.readResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2)
	if perr == nil || perr.Code != TransportUnavailable {
		t.Fatalf("readResponse error = %#v, want transport_unavailable", perr)
	}
	if failures.count != 1 {
		t.Fatalf("worker failures = %d, want 1", failures.count)
	}
	errMF := gatherFamily(t, reg, "straw_nats_errors_total")
	if got := counterValue(errMF, map[string]string{metricLabelErrorCode: errorCodeTransportUnavailable}); got != 1 {
		t.Fatalf("nats_errors_total = %v, want 1", got)
	}
}

// TestDispatcherCancellation verifies that cancelling the context returns
// Cancelled and sends a CancelFrame to the egress worker.
func TestDispatcherCancellation(t *testing.T) {
	t.Parallel()

	// Slow upstream: blocks until the test finishes.
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
		w.WriteHeader(http.StatusNoContent)
	})
	slowUpstream := httptest.NewServer(slowHandler)
	t.Cleanup(slowUpstream.Close)

	d, stop := newLiveDispatchHarness(t, slowHandler)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))

	done := make(chan *PipelineError, 1)
	go func() {
		_, perr := d.Dispatch(ctx, dispatchInput(req))
		done <- perr
	}()

	// Give the request time to reach the upstream then cancel.
	time.Sleep(150 * time.Millisecond)
	cancel()

	perr := <-done
	if perr == nil || perr.Code != Cancelled {
		t.Fatalf("Dispatch error = %#v, want cancelled", perr)
	}
}

// TestDispatcherAdminCancelEndToEnd verifies the docs/tasks/p0/27 wiring:
// registering the request with InFlightRegistry lets an admin-initiated
// cancel (the same path CancelRequest drives) terminate a running Dispatch
// with the canonical cancelled outcome and publish a CancelFrame the
// executor receives, using the same live NATS/egress harness as
// TestDispatcherCancellation.
func TestDispatcherAdminCancelEndToEnd(t *testing.T) {
	t.Parallel()

	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
		w.WriteHeader(http.StatusNoContent)
	})

	d, stop := newLiveDispatchHarness(t, slowHandler)
	defer stop()

	registry := NewInFlightRegistry()
	d.opts.InFlight = registry

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	in := dispatchInput(req)

	done := make(chan *PipelineError, 1)
	go func() {
		_, perr := d.Dispatch(context.Background(), in)
		done <- perr
	}()

	// Give the request time to reach the upstream, then cancel the way
	// CancelRequest does: through the registry, authorized as system_admin.
	time.Sleep(150 * time.Millisecond)

	admin := Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}

	err := registry.Cancel(context.Background(), admin, in.RequestID)
	if err != nil {
		t.Fatalf("registry.Cancel() error = %v", err)
	}

	perr := <-done
	if perr == nil || perr.Code != Cancelled {
		t.Fatalf("Dispatch error = %#v, want cancelled", perr)
	}
}

// TestDispatcherAdminCancelForeignTenantRejected verifies that a tenant-scoped
// admin cancel for a foreign tenant's in-flight request is rejected by the
// registry and does not cancel the dispatch.
func TestDispatcherAdminCancelForeignTenantRejected(t *testing.T) {
	t.Parallel()

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer stop()

	registry := NewInFlightRegistry()
	d.opts.InFlight = registry

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	in := dispatchInput(req)

	done := make(chan *PipelineError, 1)
	go func() {
		_, perr := d.Dispatch(context.Background(), in)
		done <- perr
	}()

	time.Sleep(150 * time.Millisecond)

	foreign := Identity{ScopeType: ScopeTenant, TenantID: "ten_other", Role: RoleTenantAdmin}

	err := registry.Cancel(context.Background(), foreign, in.RequestID)
	if !errors.Is(err, ErrInsufficientPermissions) {
		t.Fatalf("registry.Cancel() error = %v, want ErrInsufficientPermissions", err)
	}

	select {
	case perr := <-done:
		t.Fatalf("Dispatch finished early with %#v, want it still running after rejected cancel", perr)
	case <-time.After(300 * time.Millisecond):
	}
}
