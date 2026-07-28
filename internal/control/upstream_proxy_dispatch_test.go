package control

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func TestResolveRouteExecutionDirectAndProxy(t *testing.T) {
	t.Parallel()

	target, err := url.Parse("https://www.example.com/products")
	if err != nil {
		t.Fatal(err)
	}
	in := DispatchInput{
		Identity: Identity{DeploymentID: testDeploymentID},
		Request: &ValidatedRequest{
			Method: http.MethodGet,
			URL:    target,
			Routing: RoutingHints{
				Country: "AU", Region: testProxyRegion, IPType: routingIPType, StickySessionID: testStickySessionID,
			},
		},
	}
	dispatcher := NewDefaultRequestDispatcher(RequestDispatcherOptions{})
	snapshot := config.NewSnapshot(1)

	direct, perr := dispatcher.resolveRouteExecution(in, snapshot, RouteOutcome{PoolID: "direct"}, false)
	if perr != nil {
		t.Fatalf("direct execution: %v", perr)
	}
	if direct.upstreamProxy != nil || direct.policy.Policy.GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL {
		t.Fatalf("direct execution = %+v", direct)
	}

	route := RouteOutcome{
		PoolID: "proxy-pool", UpstreamProxyID: "provider-profile", TrustedRemoteResolution: true,
		StickySessionTTLSeconds: 900, ProtocolMinor: 2,
	}
	proxied, perr := dispatcher.resolveRouteExecution(in, snapshot, route, false)
	if perr != nil {
		t.Fatalf("proxy execution: %v", perr)
	}
	if proxied.policy.Policy.GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE {
		t.Fatalf("resolution mode = %v", proxied.policy.Policy.GetResolutionMode())
	}
	instruction := proxied.upstreamProxy
	if instruction.GetUpstreamProxyId() != route.UpstreamProxyID || instruction.GetCountry() != "AU" || instruction.GetRegion() != testProxyRegion || instruction.GetIpType() != routingIPType {
		t.Fatalf("instruction = %+v", instruction)
	}
	wantSession := deriveProviderSessionID(testDeploymentID, route, "AU", testProxyRegion, routingIPType, testStickySessionID)
	if instruction.GetProviderSessionId() != wantSession || len(wantSession) != 32 {
		t.Fatalf("provider session = %q, want %q", instruction.GetProviderSessionId(), wantSession)
	}

	otherRoute := route
	otherRoute.PoolID = "other-pool"
	other, perr := dispatcher.resolveRouteExecution(in, snapshot, otherRoute, true)
	if perr != nil {
		t.Fatalf("fallback execution: %v", perr)
	}
	if other.upstreamProxy.GetProviderSessionId() == instruction.GetProviderSessionId() {
		t.Fatal("different-pool fallback retained provider session")
	}
	if other.policy.FingerprintProfile != "" || len(other.policy.InjectionOperations) != 0 {
		t.Fatal("raw tunnel execution retained decoded HTTP policy")
	}
}

func TestMinor2RouteResponseFrameContract(t *testing.T) {
	t.Parallel()

	dispatcher := NewDefaultRequestDispatcher(RequestDispatcherOptions{})
	route := RouteOutcome{WorkerID: "worker-1", UpstreamProxyID: "proxy-1", ProtocolMinor: 2}
	outbound, response := false, false

	if perr := dispatcher.validateRouteResponseFrame(route, responseStartTestFrame(), &outbound, &response); perr == nil || perr.Code != ProtocolError {
		t.Fatalf("response before outbound = %#v", perr)
	}

	outbound, response = false, false
	wrong := outboundStartTestFrame("wrong")
	if perr := dispatcher.validateRouteResponseFrame(route, wrong, &outbound, &response); perr == nil || perr.Code != ProtocolError {
		t.Fatalf("wrong proxy outbound = %#v", perr)
	}

	outbound, response = false, false
	if perr := dispatcher.validateRouteResponseFrame(route, outboundStartTestFrame("proxy-1"), &outbound, &response); perr != nil {
		t.Fatalf("matching outbound: %v", perr)
	}
	if perr := dispatcher.validateRouteResponseFrame(route, responseStartTestFrame(), &outbound, &response); perr != nil {
		t.Fatalf("response after outbound: %v", perr)
	}
	if perr := dispatcher.validateRouteResponseFrame(route, responseStartTestFrame(), &outbound, &response); perr == nil || perr.Code != ProtocolError {
		t.Fatalf("duplicate response start = %#v", perr)
	}
	if perr := dispatcher.validateRouteResponseFrame(route, outboundStartTestFrame("proxy-1"), &outbound, &response); perr == nil || perr.Code != ProtocolError {
		t.Fatalf("duplicate outbound = %#v", perr)
	}

	outbound, response = false, false
	errorFrame := &strawpb.StreamFrame{Payload: &strawpb.StreamFrame_Error{Error: &strawpb.ErrorFrame{Code: strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR}}}
	if perr := dispatcher.validateRouteResponseFrame(route, errorFrame, &outbound, &response); perr != nil {
		t.Fatalf("terminal error before outbound: %v", perr)
	}
	endFrame := &strawpb.StreamFrame{Payload: &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}}}
	if perr := dispatcher.validateRouteResponseFrame(route, endFrame, &outbound, &response); perr == nil || perr.Code != ProtocolError {
		t.Fatalf("successful end before outbound = %#v", perr)
	}
	outbound = true
	if perr := dispatcher.validateRouteResponseFrame(route, endFrame, &outbound, &response); perr == nil || perr.Code != ProtocolError {
		t.Fatalf("successful end before response = %#v", perr)
	}
	response = true
	if perr := dispatcher.validateRouteResponseFrame(route, endFrame, &outbound, &response); perr != nil {
		t.Fatalf("successful end after response: %v", perr)
	}

	direct := RouteOutcome{ProtocolMinor: 2}
	outbound, response = false, false
	if perr := dispatcher.validateRouteResponseFrame(direct, outboundStartTestFrame(""), &outbound, &response); perr != nil {
		t.Fatalf("direct outbound: %v", perr)
	}
}

func TestDispatchEnvelopeMinorValidation(t *testing.T) {
	t.Parallel()

	env := &strawpb.Envelope{
		ProtocolMajor: ProtocolMajor, ProtocolMinor: 1,
		Payload: &strawpb.Envelope_StreamFrame{StreamFrame: &strawpb.StreamFrame{StreamSeq: 1, Attempt: 1, Payload: &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}}}},
	}
	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	if decodeDispatchFrame(raw, 1) == nil {
		t.Fatal("matching minor response rejected")
	}
	if decodeDispatchFrame(raw, 2) != nil {
		t.Fatal("mismatched minor response accepted")
	}

	env.ProtocolMinor = 0
	raw, err = natsx.MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	if decodeDispatchFrame(raw, 1) == nil {
		t.Fatal("published legacy minor-1 response rejected")
	}
	if decodeDispatchFrame(raw, 2) != nil {
		t.Fatal("legacy omitted minor accepted for minor-2 route")
	}
}

func TestUpstreamStatusPublicErrorPresence(t *testing.T) {
	t.Parallel()

	status := uint32(407)
	perr := errorFramePipelineError(strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE, &strawpb.ErrorFrame{
		Code: strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE, UpstreamStatus: &status,
		Details: map[string]string{"fact": "upstream_proxy_authentication_failed"},
	})
	response := pipelineErrorResponse("request-1", perr)
	if response.UpstreamStatus == nil || *response.UpstreamStatus != status {
		t.Fatalf("upstream status = %v", response.UpstreamStatus)
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	err = json.Unmarshal(raw, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if payload["upstream_status"] != float64(status) {
		t.Fatalf("public error = %s", raw)
	}

	absent := pipelineErrorResponse("request-2", &PipelineError{Code: UpstreamProxyFailure})
	if absent.UpstreamStatus != nil {
		t.Fatal("absent upstream status became present")
	}
}

func outboundStartTestFrame(proxyID string) *strawpb.StreamFrame {
	return &strawpb.StreamFrame{Payload: &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{UpstreamProxyId: proxyID}}}
}

func responseStartTestFrame() *strawpb.StreamFrame {
	return &strawpb.StreamFrame{Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusOK}}}
}
