package endpoint

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	endpointhttp "github.com/beremaran/straw/internal/endpoint/http"
	"github.com/beremaran/straw/pkg/protocol"
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

	c := NewConsumer(mb, httpClient, secret, "test-endpoint",
		WithStatsCallback(callback),
	)
	c.ctx = context.Background()

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

	_ = c.processTask(context.Background(), body)

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

	if statsResult.BytesReceived == 0 {
		t.Error("expected BytesReceived > 0 (even error response has wire size)")
	}
}
