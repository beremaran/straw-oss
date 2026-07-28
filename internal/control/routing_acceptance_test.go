package control

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw-oss/internal/config"
)

// ingressRouteAcceptanceDispatcher keeps the real DefaultRequestDispatcher
// route evaluator while providing bounded success responses for the three
// public ingress adapters. This exercises request parsing and ingress
// normalization without requiring an outbound destination.
type ingressRouteAcceptanceDispatcher struct {
	routing   *DefaultRequestDispatcher
	snapshot  config.Snapshot
	decisions []RouteOutcome
	ingresses []string
}

func (d *ingressRouteAcceptanceDispatcher) Dispatch(_ context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	if outcome := d.decide(in); !outcome.OK {
		return SuccessResponse{}, &PipelineError{Code: RouteUnavailable}
	}

	return SuccessResponse{Status: http.StatusOK}, nil
}

func (d *ingressRouteAcceptanceDispatcher) DispatchRaw(_ context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	if outcome := d.decide(in); !outcome.OK {
		return SuccessResponse{}, &PipelineError{Code: RouteUnavailable}, false
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")

	return SuccessResponse{Status: http.StatusOK}, nil, true
}

func (d *ingressRouteAcceptanceDispatcher) DispatchTunnel(_ context.Context, in DispatchInput, rw io.ReadWriter) (SuccessResponse, *PipelineError) {
	if outcome := d.decide(in); !outcome.OK {
		return SuccessResponse{}, &PipelineError{Code: RouteUnavailable}
	}

	_, _ = io.WriteString(rw, "HTTP/1.1 200 Connection Established\r\n\r\n")

	return SuccessResponse{Status: http.StatusOK}, nil
}

func (d *ingressRouteAcceptanceDispatcher) decide(in DispatchInput) RouteOutcome {
	d.ingresses = append(d.ingresses, in.Request.IngressType)
	outcome := d.routing.route(in, d.snapshot)
	d.decisions = append(d.decisions, outcome)

	return outcome
}

func TestIngressAdaptersUseEquivalentRoutingDecisions(t *testing.T) {
	snapshot := config.NewSnapshot(7)
	snapshot.ExecutorPools = []config.ExecutorPool{{ID: routingSharedPoolID, ExecutorType: errorCategoryEgress, Enabled: true}}
	snapshot.RoutingRules = []config.RoutingRule{{
		ID: routingSharedRouteID, Priority: 1, Enabled: true, TargetPoolID: routingSharedPoolID,
		Match: config.MatchConditions{Country: "AU", Region: routingRegion, IPType: routingIPType},
	}}
	candidate := routingCandidate("shared-worker")
	candidate.Tags = []string{routingIPType}
	candidate.Countries = []string{"AU"}
	candidate.Regions = []string{routingRegion}
	candidate.IPTypes = []string{routingIPType}
	candidate.IngressModes = []string{IngressTypeREST, IngressTypeHTTPProxy, IngressTypeConnect}
	workers := testRoutingCandidates{routingSharedPoolID: {candidate}}
	routing := NewDefaultRequestDispatcher(RequestDispatcherOptions{Workers: workers, Sticky: NewStickyStore(nil)})
	dispatcher := &ingressRouteAcceptanceDispatcher{routing: routing, snapshot: snapshot}
	auth := NewDeploymentAuthenticator("secret")

	rest := NewRequestHandler(1<<20, 5000, auth)
	rest.SetDispatcher(dispatcher)
	restRequest := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", bytes.NewBufferString(`{"method":"GET","url":"https://example.com/","routing":{"tags":["residential"],"country":"au","region":"ap-southeast-2","ip_type":"residential","sticky_session_id":"checkout-42"}}`))
	restRequest.Header.Set("Authorization", "Bearer secret")
	restResponse := httptest.NewRecorder()
	rest.ServeHTTP(restResponse, restRequest)
	if restResponse.Code != http.StatusOK {
		t.Fatalf("REST response = %d %q", restResponse.Code, restResponse.Body.String())
	}

	proxy := NewProxyHandler(1<<20, auth, dispatcher, dispatcher)
	proxyRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
	proxyRequest.Header.Set("Proxy-Authorization", "Bearer secret")
	proxyRequest.Header.Set("X-Straw-Route-Tags", routingIPType)
	proxyRequest.Header.Set("X-Straw-Route-Country", "au")
	proxyRequest.Header.Set("X-Straw-Route-Region", routingRegion)
	proxyRequest.Header.Set("X-Straw-Route-IP-Type", routingIPType)
	proxyRequest.Header.Set("X-Straw-Route-Sticky-Session", testStickySessionID)
	proxyResponse := httptest.NewRecorder()
	proxy.ServeHTTP(proxyResponse, proxyRequest)
	if proxyResponse.Code != http.StatusOK || proxyResponse.Body.String() != "ok" {
		t.Fatalf("absolute-form proxy response = %d %q", proxyResponse.Code, proxyResponse.Body.String())
	}

	server, client := net.Pipe()
	defer func() { _ = client.Close() }()
	connectRequest := httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://example.com:443", nil)
	connectRequest.Host = "example.com:443"
	connectRequest.Header.Set("Proxy-Authorization", "Bearer secret")
	connectRequest.Header.Set("X-Straw-Route-Tags", routingIPType)
	connectRequest.Header.Set("X-Straw-Route-Country", "au")
	connectRequest.Header.Set("X-Straw-Route-Region", routingRegion)
	connectRequest.Header.Set("X-Straw-Route-IP-Type", routingIPType)
	connectRequest.Header.Set("X-Straw-Route-Sticky-Session", testStickySessionID)
	connectWriter := &hijackResponseWriter{conn: server, header: make(http.Header)}
	done := make(chan struct{})
	go func() {
		proxy.ServeHTTP(connectWriter, connectRequest)
		close(done)
	}()
	handshake := make([]byte, len("HTTP/1.1 200 Connection Established\r\n\r\n"))
	_, err := io.ReadFull(client, handshake)
	if err != nil {
		t.Fatalf("read CONNECT handshake: %v", err)
	}
	<-done

	if got, want := dispatcher.ingresses, []string{IngressTypeREST, IngressTypeHTTPProxy, IngressTypeConnect}; !equalStrings(got, want) {
		t.Fatalf("ingresses = %v, want %v", got, want)
	}
	if len(dispatcher.decisions) != 3 {
		t.Fatalf("routing decisions = %d, want 3", len(dispatcher.decisions))
	}
	first := dispatcher.decisions[0]
	for _, outcome := range dispatcher.decisions {
		if !outcome.OK || !outcome.Sticky || outcome.RuleID != first.RuleID || outcome.PoolID != first.PoolID || outcome.WorkerID != first.WorkerID {
			t.Fatalf("routing outcomes = %+v, want equivalent sticky decisions from %+v", dispatcher.decisions, first)
		}
	}
}

func TestDestinationPolicyEnforcedAcrossIngressDispatchers(t *testing.T) {
	snapshot := config.NewSnapshot(8)
	snapshot.ExecutorPools = []config.ExecutorPool{{ID: routingSharedPoolID, ExecutorType: errorCategoryEgress, Enabled: true}}
	snapshot.RoutingRules = []config.RoutingRule{{ID: routingSharedRouteID, Priority: 1, Enabled: true, TargetPoolID: routingSharedPoolID}}
	snapshot.DenyRules = []config.DenyRule{{RuleType: denyRuleTypeHost, Action: denyRuleActionDeny, Enabled: true, NormalizedHost: "blocked.example"}}
	candidate := routingCandidate("shared-worker")
	candidate.IngressModes = []string{IngressTypeREST, IngressTypeHTTPProxy, IngressTypeConnect}
	dispatcher := NewDefaultRequestDispatcher(RequestDispatcherOptions{
		ConfigCache: NewConfigCache(snapshot),
		Workers:     testRoutingCandidates{routingSharedPoolID: {candidate}},
	})

	request := func(ingress, rawURL string) *ValidatedRequest {
		return &ValidatedRequest{Method: http.MethodGet, URL: mustURL(t, rawURL), IngressType: ingress}
	}
	for _, testCase := range []struct {
		name    string
		ingress string
		rawURL  string
	}{
		{name: "REST", ingress: IngressTypeREST, rawURL: "https://blocked.example/"},
		{name: "absolute-form proxy", ingress: IngressTypeHTTPProxy, rawURL: "http://blocked.example/"},
		{name: "CONNECT", ingress: IngressTypeConnect, rawURL: "connect://blocked.example:443"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := DispatchInput{RequestID: testCase.name, Identity: Identity{DeploymentID: config.DefaultDeploymentID}, Request: request(testCase.ingress, testCase.rawURL)}
			var perr *PipelineError
			switch testCase.ingress {
			case IngressTypeREST:
				_, perr = dispatcher.Dispatch(context.Background(), input)
			case IngressTypeHTTPProxy:
				_, perr, _ = dispatcher.DispatchRaw(context.Background(), input, httptest.NewRecorder())
			case IngressTypeConnect:
				_, perr = dispatcher.DispatchTunnel(context.Background(), input, &bytes.Buffer{})
			}
			if perr == nil || perr.Code != DestinationDenied {
				t.Fatalf("pipeline error = %+v, want destination_denied", perr)
			}
		})
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}
