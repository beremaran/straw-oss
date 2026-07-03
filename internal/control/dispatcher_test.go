package control

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"

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
	req.Headers = []HeaderPair{{Name: "Content-Type", Value: base64.StdEncoding.EncodeToString([]byte("text/plain"))}}

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

	resolver := dispatchResolver{"dispatch.test": loopbackDispatchIP(t, upstream.URL)}
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
		{RuleType: denyRuleTypeCIDR, Action: denyRuleActionAllow, Enabled: true, NormalizedCIDR: loopbackDispatchCIDR(t, upstream.URL)},
	}

	d := newTestDispatcherWithSnapshot(t, snapshot, dispatchCandidates{dispatchCandidate()})
	d.opts.NATS = controlConn
	d.opts.QuotaAdmission = NewQuotaAdmission(client, nil)
	d.opts.RateLimitAdmission = NewRateLimitAdmission(NewRateLimiter(client, DefaultRateLimitGuardrails(), nil))

	return &liveDispatchHarness{DefaultRequestDispatcher: d, upstreamURL: upstream.URL}, stop
}

func newTestDispatcher(t *testing.T, rules []config.RoutingRule, candidates dispatchCandidates) *DefaultRequestDispatcher {
	t.Helper()

	return newTestDispatcherWithSnapshot(t, dispatchSnapshot(rules), candidates)
}

func newTestDispatcherWithSnapshot(t *testing.T, snapshot config.TenantSnapshot, candidates dispatchCandidates) *DefaultRequestDispatcher {
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
		IngressModes:   []string{requestMetadataIngressType},
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

func dispatchInput(req *ValidatedRequest) DispatchInput {
	return DispatchInput{
		RequestID: "req_dispatch",
		Identity:  Identity{APIKeyID: dispatchTestKey, ScopeType: ScopeTenant, TenantID: dispatchTestTenant, Role: RoleRequester},
		Request:   req,
	}
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

func dispatchConnect(t *testing.T, url string) *natsx.Connection {
	t.Helper()

	conn, err := natsx.Connect(natsx.ConnectOptions{Servers: []string{url}})
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return conn
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
