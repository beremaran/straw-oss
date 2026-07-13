package control

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/natsx"
	"github.com/beremaran/straw-oss/internal/testutil"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	proxyTestWorkerA = "worker-a"
	proxyTestWorkerB = "worker-b"
	proxyTestPool    = "proxy"
)

func TestProxyHTTPAssignmentFallbackPreservesHintsAndStickySelection(t *testing.T) {
	conn, dispatcher, attempts := newProxyDispatchE2E(t, false, IngressTypeHTTPProxy)
	defer conn.Close()

	handler := NewProxyHandler(1024, NewDeploymentAuthenticator("secret"), dispatcher, dispatcher)
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
		req.Header.Set("Proxy-Authorization", "Bearer secret")
		req.Header.Set("X-Straw-Route-Tags", routingIPType)
		req.Header.Set("X-Straw-Route-Country", "au")
		req.Header.Set("X-Straw-Route-Sticky-Session", "checkout-42")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		return rec
	}

	if rec := request(); rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("first proxy response = %d %q, want 200 ok", rec.Code, rec.Body.String())
	}
	if rec := request(); rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("sticky proxy response = %d %q, want 200 ok", rec.Code, rec.Body.String())
	}

	got := []string{<-attempts, <-attempts, <-attempts}
	want := []string{proxyTestWorkerA, proxyTestWorkerB, proxyTestWorkerB}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("assignment attempts = %v, want %v", got, want)
		}
	}
}

func TestProxyHTTPNeverFallsBackAfterResponseBytes(t *testing.T) {
	conn, dispatcher, attempts := newProxyDispatchE2E(t, true, IngressTypeHTTPProxy)
	defer conn.Close()

	handler := NewProxyHandler(1024, NewDeploymentAuthenticator("secret"), dispatcher, dispatcher)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)
	req.Header.Set("Proxy-Authorization", "Bearer secret")
	req.Header.Set("X-Straw-Route-Country", "AU")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("response = %d, want committed upstream status 502", rec.Code)
	}
	select {
	case got := <-attempts:
		if got != proxyTestWorkerA {
			t.Fatalf("attempt worker = %q, want %s", got, proxyTestWorkerA)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for assignment")
	}
	select {
	case got := <-attempts:
		t.Fatalf("post-response fallback assignment = %q, want none", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestProxyConnectAssignmentFallbackAndOpaqueCommitBoundary(t *testing.T) {
	t.Run("assignment fallback", func(t *testing.T) {
		conn, dispatcher, attempts := newProxyDispatchE2E(t, false, IngressTypeConnect)
		defer conn.Close()

		request := &ValidatedRequest{Method: http.MethodConnect, URL: &url.URL{Scheme: IngressTypeConnect, Host: proxyTestAuthority}, Routing: RoutingHints{Country: "AU"}, IngressType: IngressTypeConnect}
		var tunnel bytes.Buffer
		response, perr := dispatcher.DispatchTunnel(context.Background(), DispatchInput{RequestID: "connect-fallback", Identity: Identity{DeploymentID: config.DefaultDeploymentID}, Request: request}, &tunnel)
		if perr != nil || response.Status != http.StatusOK || tunnel.String() != "HTTP/1.1 200 Connection Established\r\n\r\nok" {
			t.Fatalf("connect fallback response=%+v err=%v bytes=%q", response, perr, tunnel.String())
		}
		if got := []string{<-attempts, <-attempts}; got[0] != proxyTestWorkerA || got[1] != proxyTestWorkerB {
			t.Fatalf("connect assignment attempts = %v, want [%s %s]", got, proxyTestWorkerA, proxyTestWorkerB)
		}
	})

	t.Run("no replay after established", func(t *testing.T) {
		conn, dispatcher, attempts := newProxyDispatchE2E(t, true, IngressTypeConnect)
		defer conn.Close()

		request := &ValidatedRequest{Method: http.MethodConnect, URL: &url.URL{Scheme: IngressTypeConnect, Host: proxyTestAuthority}, Routing: RoutingHints{Country: "AU"}, IngressType: IngressTypeConnect}
		var tunnel bytes.Buffer
		response, perr := dispatcher.DispatchTunnel(context.Background(), DispatchInput{RequestID: "connect-committed", Identity: Identity{DeploymentID: config.DefaultDeploymentID}, Request: request}, io.ReadWriter(&tunnel))
		if perr == nil || response.Status != http.StatusOK || !bytes.HasPrefix(tunnel.Bytes(), []byte("HTTP/1.1 200 Connection Established\r\n\r\n")) {
			t.Fatalf("connect committed response=%+v err=%v bytes=%q", response, perr, tunnel.String())
		}
		select {
		case got := <-attempts:
			if got != proxyTestWorkerA {
				t.Fatalf("committed connect worker = %q, want %s", got, proxyTestWorkerA)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for committed connect assignment")
		}
		select {
		case got := <-attempts:
			t.Fatalf("post-establishment fallback assignment = %q, want none", got)
		case <-time.After(100 * time.Millisecond):
		}
	})
}

func newProxyDispatchE2E(t *testing.T, committedFailure bool, ingress string) (*natsx.Connection, *DefaultRequestDispatcher, <-chan string) {
	t.Helper()

	server := testutil.NewFakeNATSServer(t, 1<<20)
	conn, err := natsx.Connect(natsx.ConnectOptions{Servers: []string{server.URL()}})
	if err != nil {
		t.Fatalf("connect fake NATS: %v", err)
	}

	candidate := func(id, session string) PoolCandidate {
		return PoolCandidate{
			WorkerID: id, SessionID: session, AssignSubject: "assign." + id, ExecutorType: errorCategoryEgress,
			Countries: []string{"AU"}, Tags: []string{routingIPType}, IngressModes: []string{ingress},
			MaxConcurrency: 1, AvailableCap: 1,
		}
	}

	snapshot := config.NewSnapshot(1)
	snapshot.DefaultTimeoutMs = 2000
	snapshot.MaxTimeoutMs = 5000
	snapshot.ExecutorPools = []config.ExecutorPool{{ID: proxyTestPool, ExecutorType: errorCategoryEgress, Enabled: true}}
	snapshot.RoutingRules = []config.RoutingRule{{
		ID: "proxy-route", Priority: 1, Enabled: true, TargetPoolID: proxyTestPool,
		Match: config.MatchConditions{IngressType: ingress, Country: "AU"}, StickySessionTTLSeconds: 60, AllowStickyFallback: true,
	}}

	wrongCapability := candidate("worker-0", "session-0")
	wrongCapability.Countries = []string{"US"}
	workers := testRoutingCandidates{proxyTestPool: {wrongCapability, candidate(proxyTestWorkerA, "session-a"), candidate(proxyTestWorkerB, "session-b")}}
	dispatcher := NewDefaultRequestDispatcher(RequestDispatcherOptions{
		ConfigCache:                NewConfigCache(snapshot),
		Workers:                    workers,
		NATS:                       conn,
		MaxInlineResponseBodyBytes: 1024,
		MaxTimeoutMs:               5000,
		FrameIdleTimeout:           time.Second,
		Now:                        time.Now,
	})

	return conn, dispatcher, subscribeProxyAssignments(t, conn, committedFailure, ingress)
}

func subscribeProxyAssignments(t *testing.T, conn *natsx.Connection, committedFailure bool, ingress string) <-chan string {
	t.Helper()

	attempts := make(chan string, 8)
	for _, worker := range []struct{ id, session string }{{proxyTestWorkerA, "session-a"}, {proxyTestWorkerB, "session-b"}} {
		_, err := conn.Subscribe("assign."+worker.id, func(msg *nats.Msg) {
			env, err := natsx.UnmarshalEnvelope(msg.Data)
			if err != nil || env.GetAssignRequest() == nil {
				return
			}
			attempts <- worker.id

			code := strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED
			if worker.id == proxyTestWorkerA && !committedFailure {
				code = strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_CAPACITY
			}
			ack := &strawpb.Envelope{
				RequestId: env.GetRequestId(), ProtocolMajor: ProtocolMajor, Attempt: env.GetAttempt(),
				Payload: &strawpb.Envelope_AssignAck{AssignAck: &strawpb.AssignAck{Code: code}},
			}
			raw, err := natsx.MarshalEnvelope(ack)
			if err != nil {
				return
			}
			respondErr := msg.Respond(raw)
			if respondErr != nil || code != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
				return
			}

			if worker.id == proxyTestWorkerA && committedFailure {
				publishProxyFrame(conn, env.GetRequestId(), worker.id, worker.session, 1, &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{}})
				status := http.StatusBadGateway
				if ingress == IngressTypeConnect {
					status = http.StatusOK
				}
				publishProxyFrame(conn, env.GetRequestId(), worker.id, worker.session, 2, &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: uint32(status)}})
				publishProxyFrame(conn, env.GetRequestId(), worker.id, worker.session, 3, &strawpb.StreamFrame_Error{Error: &strawpb.ErrorFrame{Code: strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET}})

				return
			}

			publishProxyFrame(conn, env.GetRequestId(), worker.id, worker.session, 1, &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{}})
			publishProxyFrame(conn, env.GetRequestId(), worker.id, worker.session, 2, &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{Status: http.StatusOK}})
			publishProxyFrame(conn, env.GetRequestId(), worker.id, worker.session, 3, &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: 0, Data: []byte("ok")}})
			publishProxyFrame(conn, env.GetRequestId(), worker.id, worker.session, 4, &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}})
		})
		if err != nil {
			t.Fatalf("subscribe assignment %s: %v", worker.id, err)
		}
	}
	err := conn.Flush()
	if err != nil {
		t.Fatalf("flush assignment subscriptions: %v", err)
	}

	return attempts
}

func publishProxyFrame(conn *natsx.Connection, requestID, worker, session string, seq uint64, payload any) {
	subject, err := natsx.StreamSubject(requestID, worker, session, natsx.DirectionExecutorToControl)
	if err != nil {
		return
	}
	var frame *strawpb.StreamFrame
	switch payload := payload.(type) {
	case *strawpb.StreamFrame_OutboundStart:
		frame = &strawpb.StreamFrame{StreamSeq: seq, Attempt: 1, Payload: payload}
	case *strawpb.StreamFrame_ResponseStart:
		frame = &strawpb.StreamFrame{StreamSeq: seq, Attempt: 1, Payload: payload}
	case *strawpb.StreamFrame_Data:
		frame = &strawpb.StreamFrame{StreamSeq: seq, Attempt: 1, Payload: payload}
	case *strawpb.StreamFrame_Error:
		frame = &strawpb.StreamFrame{StreamSeq: seq, Attempt: 1, Payload: payload}
	case *strawpb.StreamFrame_End:
		frame = &strawpb.StreamFrame{StreamSeq: seq, Attempt: 1, Payload: payload}
	default:
		return
	}
	env := &strawpb.Envelope{
		RequestId: requestID, DeploymentId: config.DefaultDeploymentID, ProtocolMajor: ProtocolMajor, Attempt: 1,
		Payload: &strawpb.Envelope_StreamFrame{StreamFrame: frame},
	}
	raw, err := natsx.MarshalEnvelope(env)
	if err == nil {
		_ = conn.Publish(subject, raw)
	}
}
