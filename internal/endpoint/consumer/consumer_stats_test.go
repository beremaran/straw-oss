package consumer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	endpointhttp "github.com/kwilabs/straw-proxy-server/internal/endpoint/http"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

func TestConsumer_StatsCallback(t *testing.T) {
	mb := &mockBroker{}
	registry := fingerprint.NewRegistry()
	provider := &mockTransportProvider{}
	httpClient := endpointhttp.NewClient(registry, provider)
	secret := []byte("test-secret")

	var statsResult TaskResult
	callback := func(res TaskResult) {
		statsResult = res
	}

	c := New(mb, httpClient, secret, "test-endpoint",
		WithStatsCallback(callback),
	)
	c.ctx = context.Background()

	// Create a valid request
	req := &protocol.Request{
		ID:     "test-req-stats",
		Method: "POST",
		URL:    "https://example.com/api",
		Body:   []byte(`{"key":"value"}`),
	}

	signedTask, err := protocol.NewSignedTask(req, secret)
	if err != nil {
		t.Fatalf("failed to create signed task: %v", err)
	}

	body, err := json.Marshal(signedTask)
	if err != nil {
		t.Fatalf("failed to marshal signed task: %v", err)
	}

	// Process the task
	// This will fail the HTTP request (fingerprint not found), but should still calculate stats
	_ = c.processTask(context.Background(), body)

	// Check BytesSent - should match EstimateWireSize
	expectedBytesSent := req.EstimateWireSize()
	if statsResult.BytesSent != expectedBytesSent {
		t.Errorf("expected BytesSent %d, got %d", expectedBytesSent, statsResult.BytesSent)
	}

	if statsResult.BytesSent == 0 {
		t.Error("expected BytesSent > 0")
	}

	if statsResult.RequestID != "test-req-stats" {
		t.Errorf("expected RequestID 'test-req-stats', got %s", statsResult.RequestID)
	}

	// BytesReceived should be > 0 even on error (minimal response structure)
	if statsResult.BytesReceived == 0 {
		t.Error("expected BytesReceived > 0 (even error response has wire size)")
	}
}
