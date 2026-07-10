package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
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
	"github.com/beremaran/straw/v2/internal/objectstore"
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

func TestDispatcherNamedFingerprintUsesCapableSessionAfterOrdinaryEligibility(t *testing.T) {
	t.Parallel()

	incapable := dispatchCandidate()
	incapable.WorkerID = "worker_dispatch_incapable"
	incapable.SessionID = "session_dispatch_incapable"
	incapable.AssignSubject = "straw.v1.executor." + incapable.WorkerID + "." + incapable.SessionID + ".assign"

	capable := dispatchCandidate()
	capable.WorkerID = "worker_dispatch_capable"
	capable.SessionID = "session_dispatch_capable"
	capable.AssignSubject = "straw.v1.executor." + capable.WorkerID + "." + capable.SessionID + ".assign"
	capable.SupportedFingerprintProfiles = []string{fingerprintProfileChrome120}

	snapshot := dispatchSnapshot([]config.RoutingRule{dispatchRule()})
	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{incapable, capable})
	req := validatedDispatchRequest(t, "https://example.com/")
	req.Fingerprint = fingerprintProfileChrome120

	outcome := d.route(dispatchInput(req), snapshot)
	if !outcome.OK || outcome.WorkerID != capable.WorkerID {
		t.Fatalf("route outcome = %+v, want capable worker %s", outcome, capable.WorkerID)
	}
}

func TestDispatcherNamedFingerprintPreservesOrdinaryRouteUnavailable(t *testing.T) {
	t.Parallel()

	incapable := dispatchCandidate()
	incapable.AvailableCap = 0
	incapable.SupportedFingerprintProfiles = []string{fingerprintProfileChrome120}

	d := newTestDispatcherWithSnapshot(t, dispatchSnapshot([]config.RoutingRule{dispatchRule()}), dispatchCandidates{incapable})
	req := validatedDispatchRequest(t, "https://example.com/")
	req.Fingerprint = fingerprintProfileChrome120

	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != RouteUnavailable {
		t.Fatalf("Dispatch error = %#v, want route_unavailable before unsupported_fingerprint when no ordinary candidate exists", perr)
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

func TestDispatcherTeesLargeResponseBodyToObjectStorage(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("response payload ", 64)
	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer stop()

	store := &fakeResponseBodyRefStore{}
	d.opts.MaxInlineResponseBodyBytes = 8
	d.opts.BodyTransport = config.ControlBodyTransportConfig{
		LargeBodyThresholdBytes: 8,
		ObjectStorage:           config.BodyObjectStorageConfig{Enabled: true},
	}.Normalized()
	d.opts.ResponseObjectStore = store

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	resp, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr != nil {
		t.Fatalf("Dispatch error = %#v", perr)
	}
	if resp.Body.Mode != "body_ref" || resp.Body.BodyRef == nil {
		t.Fatalf("response body = %#v, want body_ref download reference", resp.Body)
	}
	if resp.Body.DataBase64 != "" {
		t.Fatalf("body_ref response inlined data = %q, want empty", resp.Body.DataBase64)
	}
	if store.uploadCount() != 1 {
		t.Fatalf("upload count = %d, want exactly one tee", store.uploadCount())
	}
	if string(store.body) != body {
		t.Fatalf("teed body = %q..., want full upstream body", string(store.body[:min(16, len(store.body))]))
	}
	// Size/checksum validation: the surfaced download reference must match the
	// teed body exactly.
	ref := resp.Body.BodyRef
	sum := sha256.Sum256([]byte(body))
	if ref.SizeBytes != uint64(len(body)) || ref.Sha256Hex != hex.EncodeToString(sum[:]) {
		t.Fatalf("body_ref size/checksum = %d/%s, want %d/%s", ref.SizeBytes, ref.Sha256Hex, len(body), hex.EncodeToString(sum[:]))
	}
	if !strings.HasPrefix(ref.ObjectKey, "tenant/"+dispatchTestTenant+"/request/") || !strings.Contains(ref.ObjectKey, "/response/") || ref.SignedURL == "" {
		t.Fatalf("body_ref object = %#v, want response-scoped key and signed URL", ref)
	}
	if resp.ResponseSizeBytes != uint64(len(body)) {
		t.Fatalf("response size = %d, want %d", resp.ResponseSizeBytes, len(body))
	}
}

func TestDispatcherResponseTeeObjectStorageOutage(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 128)
	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer stop()

	store := &fakeResponseBodyRefStore{err: objectstore.Unavailable(nil)}
	d.opts.MaxInlineResponseBodyBytes = 8
	d.opts.BodyTransport = config.ControlBodyTransportConfig{
		LargeBodyThresholdBytes: 8,
		ObjectStorage:           config.BodyObjectStorageConfig{Enabled: true},
	}.Normalized()
	d.opts.ResponseObjectStore = store

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	_, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr == nil || perr.Code != BodyRefUnavailable {
		t.Fatalf("Dispatch error = %#v, want body_ref_unavailable on object storage outage", perr)
	}
	if perr.Details[errorDetailDirectionKey] != "response" {
		t.Fatalf("error details = %#v, want response direction", perr.Details)
	}
}

func TestDispatcherDoesNotTeeResponseWhenCancelled(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	wrote := make(chan struct{})

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("y", 128)))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(wrote)
		<-release
	}))
	defer stop()
	// Unblock the upstream handler before stop()/upstream.Close run (defers are
	// LIFO), so the blocked handler cannot deadlock httptest.Server.Close.
	defer close(release)

	store := &fakeResponseBodyRefStore{}
	d.opts.MaxInlineResponseBodyBytes = 8
	d.opts.BodyTransport = config.ControlBodyTransportConfig{
		LargeBodyThresholdBytes: 8,
		ObjectStorage:           config.BodyObjectStorageConfig{Enabled: true},
	}.Normalized()
	d.opts.ResponseObjectStore = store

	ctx, cancel := context.WithCancel(context.Background())
	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))

	done := make(chan *PipelineError, 1)
	go func() {
		_, perr := d.Dispatch(ctx, dispatchInput(req))
		done <- perr
	}()

	<-wrote
	cancel()

	select {
	case perr := <-done:
		if perr == nil || perr.Code != Cancelled {
			t.Fatalf("Dispatch error = %#v, want cancelled", perr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Dispatch did not return after cancellation")
	}

	// The upstream never sent End, so the tee never ran: no orphaned object.
	if store.uploadCount() != 0 {
		t.Fatalf("upload count = %d, want 0 (no tee before stream completion)", store.uploadCount())
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

func TestDispatcherNamedFingerprintBufferedRoundTrip(t *testing.T) {
	t.Parallel()

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("profiled body"))
	}))
	defer stop()

	capable := dispatchCandidate()
	capable.SupportedFingerprintProfiles = []string{fingerprintProfileChrome120}
	d.opts.Workers = dispatchCandidates{capable}

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	req.Fingerprint = fingerprintProfileChrome120

	resp, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr != nil {
		t.Fatalf("Dispatch error = %#v", perr)
	}
	if got := string(respBodyBytes(t, resp)); got != "profiled body" {
		t.Fatalf("body = %q, want profiled body", got)
	}
	if resp.SelectedFingerprintProfile != fingerprintProfileChrome120 || resp.ExecutedFingerprintProfile != fingerprintProfileChrome120 {
		t.Fatalf("profile evidence = selected %q executed %q, want chrome_120/chrome_120", resp.SelectedFingerprintProfile, resp.ExecutedFingerprintProfile)
	}
}

func TestDispatcherSendsLargeRequestBodyRef(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	conn := dispatchConnect(t, natsServer.URL())

	c2eSubject, err := natsx.StreamSubject("req_bodyref", dispatchTestWorker, dispatchTestSess, natsx.DirectionControlToExecutor)
	if err != nil {
		t.Fatalf("StreamSubject: %v", err)
	}

	frames := make(chan *strawpb.StreamFrame, 2)
	_, err = conn.Subscribe(c2eSubject, func(msg *nats.Msg) {
		frames <- decodeDispatchFrame(msg.Data)
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = conn.Flush()

	body := []byte("request body above threshold")
	sum := sha256.Sum256(body)
	store := &fakeRequestBodyRefStore{
		frame: &strawpb.BodyRefFrame{
			Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{
				ObjectKey:     "tenant/ten_dispatch/request/req_bodyref/request/nonce",
				SignedUrl:     "https://object.example/request",
				ExpiresUnixMs: time.Now().Add(time.Minute).UnixMilli(),
			}},
			ExpectedSizeBytes: uint64(len(body)),
			Sha256Hex:         hex.EncodeToString(sum[:]),
		},
	}

	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = conn
	d.opts.BodyTransport = config.ControlBodyTransportConfig{
		LargeBodyThresholdBytes: 3,
		ObjectStorage:           config.BodyObjectStorageConfig{Enabled: true},
	}.Normalized()
	d.opts.BodyObjectStore = store

	req := validatedDispatchRequest(t, "https://example.com/")
	req.Method = http.MethodPost
	req.BodyData = body

	_, err = d.sendRequestStart(context.Background(), c2eSubject, DispatchInput{
		RequestID: "req_bodyref",
		Identity:  Identity{TenantID: dispatchTestTenant},
		Request:   req,
	}, dispatchRoute(), &DestinationPolicyResult{Policy: &strawpb.DestinationPolicy{}}, 1, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("sendRequestStart() error = %v", err)
	}

	start := <-frames
	if start.GetRequestStart() == nil {
		t.Fatalf("first frame = %#v, want RequestStart", start)
	}

	ref := <-frames
	if got := ref.GetBodyRef(); got == nil || got.GetS3().GetObjectKey() != store.frame.GetS3().GetObjectKey() {
		t.Fatalf("second frame body_ref = %#v, want uploaded S3 ref", got)
	}
	if data := ref.GetData(); data != nil {
		t.Fatalf("large request sent DataFrame: %#v", data)
	}
	if store.tenantID != dispatchTestTenant || store.requestID != "req_bodyref" || string(store.body) != string(body) {
		t.Fatalf("upload scope/body = tenant %q request %q body %q", store.tenantID, store.requestID, string(store.body))
	}
}

func TestDispatcherStreamingRequestBodyWaitsForUploadCreditBeforeReading(t *testing.T) {
	t.Parallel()

	reader := &recordingChunkReader{
		chunks: [][]byte{[]byte("abc"), []byte("def")},
		reads:  make(chan struct{}, 2),
	}
	d := NewDefaultRequestDispatcher(RequestDispatcherOptions{
		MaxFrameDataBytes:        3,
		InitialUploadCreditBytes: 3,
	})
	frames := make(chan *strawpb.StreamFrame, 2)
	upload := &requestBodyUpload{
		gate: newTunnelUploadGate(3),
		c2e: &c2eStreamSender{
			publishFn: func(frame *strawpb.StreamFrame) {
				frames <- frame
			},
		},
	}
	in := DispatchInput{
		RequestID: "req_stream",
		Identity:  Identity{TenantID: dispatchTestTenant},
		Request: &ValidatedRequest{
			BodyReader:    reader,
			BodySizeBytes: 6,
		},
	}

	errs := make(chan error, 1)
	go func() { errs <- d.streamRequestBody(context.Background(), in, upload) }()

	<-reader.reads
	if got := (<-frames).GetData().GetData(); string(got) != "abc" {
		t.Fatalf("first frame = %q, want abc", got)
	}

	select {
	case <-reader.reads:
		t.Fatal("body reader was read again before upload credit was granted")
	case <-time.After(50 * time.Millisecond):
	}

	upload.gate.grant(3)

	<-reader.reads
	if got := (<-frames).GetData().GetData(); string(got) != "def" {
		t.Fatalf("second frame = %q, want def", got)
	}

	err := <-errs
	if err != nil {
		t.Fatalf("streamRequestBody() error = %v", err)
	}
}

type recordingChunkReader struct {
	chunks [][]byte
	reads  chan struct{}
}

func (r *recordingChunkReader) Read(p []byte) (int, error) {
	r.reads <- struct{}{}
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}

	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]

	return n, nil
}

func (r *recordingChunkReader) Close() error {
	return nil
}

func TestDispatcherCleansUploadedRequestBodyRefOnPublishFailure(t *testing.T) {
	t.Parallel()

	natsServer := testutil.NewFakeNATSServer(t, 2_000_000)
	conn := dispatchConnect(t, natsServer.URL())
	conn.Close()

	store := &fakeRequestBodyRefStore{
		frame: &strawpb.BodyRefFrame{
			Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{
				ObjectKey:     "tenant/ten_dispatch/request/req_cleanup/request/nonce",
				SignedUrl:     "https://object.example/request",
				ExpiresUnixMs: time.Now().Add(time.Minute).UnixMilli(),
			}},
			ExpectedSizeBytes: 4,
		},
	}
	d := newTestDispatcher(t, []config.RoutingRule{dispatchRule()}, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = conn
	d.opts.BodyTransport = config.ControlBodyTransportConfig{
		LargeBodyThresholdBytes: 3,
		ObjectStorage:           config.BodyObjectStorageConfig{Enabled: true},
	}.Normalized()
	d.opts.BodyObjectStore = store

	req := validatedDispatchRequest(t, "https://example.com/")
	req.Method = http.MethodPost
	req.BodyData = []byte("body")

	_, _, err := d.sendRequestBody(context.Background(), "straw.v1.req.req_cleanup.worker.session.c2e", DispatchInput{
		RequestID: "req_cleanup",
		Identity:  Identity{TenantID: dispatchTestTenant},
		Request:   req,
	}, time.Now().Add(time.Second), 1)
	if err == nil {
		t.Fatal("sendRequestBody() error = nil, want publish failure")
	}
	if store.deleted == nil || store.deleted.GetS3().GetObjectKey() != store.frame.GetS3().GetObjectKey() {
		t.Fatalf("deleted BodyRef = %#v, want uploaded ref", store.deleted)
	}
}

func TestDispatcherControlNATSEgressRoundTripLargeRequestBodyRef(t *testing.T) {
	t.Parallel()

	body := []byte("large body via object storage")
	sum := sha256.Sum256(body)
	object := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(object.Close)

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream request body: %v", err)
		}
		if string(raw) != string(body) {
			t.Fatalf("upstream body = %q, want %q", string(raw), string(body))
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	defer stop()

	d.opts.BodyTransport = config.ControlBodyTransportConfig{
		LargeBodyThresholdBytes: 3,
		ObjectStorage:           config.BodyObjectStorageConfig{Enabled: true},
	}.Normalized()
	d.opts.BodyObjectStore = &fakeRequestBodyRefStore{
		frame: &strawpb.BodyRefFrame{
			Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{
				ObjectKey:     "tenant/ten_dispatch/request/req_dispatch/request/nonce",
				SignedUrl:     object.URL,
				ExpiresUnixMs: time.Now().Add(time.Minute).UnixMilli(),
			}},
			ExpectedSizeBytes: uint64(len(body)),
			Sha256Hex:         hex.EncodeToString(sum[:]),
		},
	}

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	req.Method = http.MethodPost
	req.BodyData = body

	resp, perr := d.Dispatch(context.Background(), dispatchInput(req))
	if perr != nil {
		t.Fatalf("Dispatch error = %#v", perr)
	}
	if resp.Status != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.Status, http.StatusAccepted)
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

func TestDispatcherNamedFingerprintRawRoundTrip(t *testing.T) {
	t.Parallel()

	d, stop := newLiveDispatchHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("raw profiled body"))
	}))
	defer stop()

	capable := dispatchCandidate()
	capable.SupportedFingerprintProfiles = []string{fingerprintProfileChrome120}
	d.opts.Workers = dispatchCandidates{capable}

	req := validatedDispatchRequest(t, rewriteDispatchHost(t, d.upstreamURL, "dispatch.test"))
	req.Fingerprint = fingerprintProfileChrome120
	w := httptest.NewRecorder()

	resp, perr, wroteHeader := d.DispatchRaw(context.Background(), dispatchInput(req), w)
	if perr != nil {
		t.Fatalf("DispatchRaw error = %#v", perr)
	}
	if !wroteHeader || w.Code != http.StatusAccepted || w.Body.String() != "raw profiled body" {
		t.Fatalf("raw response = wroteHeader %v status %d body %q", wroteHeader, w.Code, w.Body.String())
	}
	if resp.SelectedFingerprintProfile != fingerprintProfileChrome120 || resp.ExecutedFingerprintProfile != fingerprintProfileChrome120 {
		t.Fatalf("profile evidence = selected %q executed %q, want chrome_120/chrome_120", resp.SelectedFingerprintProfile, resp.ExecutedFingerprintProfile)
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
			{Name: fingerprintProfileChrome120, ScopeType: "global", Enabled: true, SupportedByWorker: true},
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

type fakeRequestBodyRefStore struct {
	frame     *strawpb.BodyRefFrame
	err       error
	tenantID  string
	requestID string
	body      []byte
	deleted   *strawpb.BodyRefFrame
}

func (s *fakeRequestBodyRefStore) UploadRequestBody(_ context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error) {
	s.tenantID = tenantID
	s.requestID = requestID
	s.body = append([]byte(nil), body...)

	return s.frame, s.err
}

func (s *fakeRequestBodyRefStore) DeleteRequestBody(_ context.Context, frame *strawpb.BodyRefFrame) error {
	s.deleted = frame

	return nil
}

// fakeResponseBodyRefStore records response tees. When err is nil it echoes the
// received body back as a BodyRef frame with real size/checksum, so tests can
// assert Control surfaces exactly what was teed.
type fakeResponseBodyRefStore struct {
	mu       sync.Mutex
	err      error
	uploads  int
	body     []byte
	tenantID string
	frame    *strawpb.BodyRefFrame
}

func (s *fakeResponseBodyRefStore) UploadResponseBody(_ context.Context, tenantID, requestID string, body []byte) (*strawpb.BodyRefFrame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.uploads++
	if s.err != nil {
		return nil, s.err
	}

	s.tenantID = tenantID
	s.body = append([]byte(nil), body...)
	sum := sha256.Sum256(body)
	s.frame = &strawpb.BodyRefFrame{
		Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{
			ObjectKey:     "tenant/" + tenantID + "/request/" + requestID + "/response/nonce",
			SignedUrl:     "https://object.example/response",
			ExpiresUnixMs: time.Now().Add(time.Minute).UnixMilli(),
		}},
		ExpectedSizeBytes: uint64(len(body)),
		Sha256Hex:         hex.EncodeToString(sum[:]),
	}

	return s.frame, nil
}

func (s *fakeResponseBodyRefStore) DeleteResponseBody(_ context.Context, _ *strawpb.BodyRefFrame) error {
	return nil
}

func (s *fakeResponseBodyRefStore) uploadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.uploads
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

	_, perr := d.readResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2, nil)
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
	result, perr, wroteHeader := d.streamRawResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2, nil, w)
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

	result, perr := d.readResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2, nil)
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

	_, perr := d.readResponse(context.Background(), frames, dispatchRoute(), time.Now().Add(time.Second), "c2e", dispatchInput(validatedDispatchRequest(t, "https://example.com/")), 2, nil)
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
