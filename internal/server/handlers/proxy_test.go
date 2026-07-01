package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
)

func TestProxyHandlerRequiresProxyAuthorization(t *testing.T) {
	handler, err := NewProxyHandler(&controlMockBroker{}, config.ControlConfig{
		EgressID:             testEgressID,
		AuthToken:            "secret",
		MaxConcurrentTunnels: 1,
		TunnelChunkSize:      1024,
	})
	if err != nil {
		t.Fatalf("NewProxyHandler() error = %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://example.com:443", nil)
	req.Host = "example.com:443"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusProxyAuthRequired)
	}
}

func TestProxyHandlerRejectsBadConnectTarget(t *testing.T) {
	handler, err := NewProxyHandler(&controlMockBroker{}, config.ControlConfig{
		EgressID:             testEgressID,
		AuthToken:            "secret",
		MaxConcurrentTunnels: 1,
		TunnelChunkSize:      1024,
	})
	if err != nil {
		t.Fatalf("NewProxyHandler() error = %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://example.com", nil)
	req.Host = "example.com"
	req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("mode=tunnel:secret")))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func (m *controlMockBroker) CoreRequest(context.Context, string, []byte) ([]byte, error) {
	return nil, nil
}

func (m *controlMockBroker) CorePublish(context.Context, string, []byte) error {
	return nil
}

func (m *controlMockBroker) CoreSubscribe(context.Context, string, broker.Handler) (broker.Subscription, error) {
	return noopSubscription{}, nil
}

type noopSubscription struct{}

func (noopSubscription) Unsubscribe() error { return nil }
