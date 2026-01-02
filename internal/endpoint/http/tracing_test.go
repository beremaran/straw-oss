package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	fhttp "github.com/useflyent/fhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTransportProvider for testing
type MockTransportProvider struct {
	mock.Mock
}

func (m *MockTransportProvider) GetTransport(host string, preset fingerprint.Preset) *fhttp.Transport {
	args := m.Called(host, preset)
	if t, ok := args.Get(0).(*fhttp.Transport); ok {
		return t
	}
	return nil
}

// MockRoundTripper to handle the request within fhttp.Client
type MockRoundTripper struct {
	mock.Mock
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	if resp, ok := args.Get(0).(*http.Response); ok {
		return resp, args.Error(1)
	}
	return nil, args.Error(1)
}

func TestClient_Do_Tracing(t *testing.T) {
	// Setup generic mock transport provider
	mockProvider := &MockTransportProvider{}
	mockProvider.On("GetTransport", mock.Anything, mock.Anything).Return(&fhttp.Transport{
		// Mock RoundTrip to intercept request
		// fhttp.Transport doesn't expose a simple RoundTripper hook easily if we want to mock just the execution
		// without making actual network calls, unless we set DialContext to something mockable.
		// However, fhttp.Client uses Transport.RoundTrip.
		// We can just return a transport that fails or succeeds if we can control it.
		// Actually, standard http.Transport has RegisterProtocol or we can just mock the implementation of TransportProvider
		// to return a *fhttp.Transport which we can't easily interface-mock because it's a struct.
		// BUT, fhttp.Client.Do calls Transport.RoundTrip.
		// If we can't easily mock *fhttp.Transport, we can rely on the fact that we can't fully mock executed request
		// without network, UNLESS we use a local listener.
		// For tracing verification, we just need ANY valid execution that triggers the span end.
		// Even an error is fine.
	})

	// Setup In-Memory Exporter
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	// Set global provider
	otel.SetTracerProvider(tp)

	// Reset after test
	defer func() {
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
	}()

	registry := fingerprint.DefaultRegistry()

	// We need a transport that doesn't actually dial out but returns something.
	// fhttp.Transport is a struct. We can't easily mock its internal behavior unless we configure it to use a custom DialContext
	// that connects to a local test server.

	// Start local test server
	server := http.NewServeMux()
	server.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	// We'll use a real HTTP server to verify the full flow including connection reuse if possible,
	// but for unit test, just basic span creation is enough.

	// To avoid actual network calls to internet, we mock the transport to dial a local listener.
	// But `GetTransport` returns `*fhttp.Transport`.

	// Let's create a real fhttp.Transport that dials a mockup listener.
	// We can use `fhttp.Transport{DialContext: ...}`

	// Create a dummy listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	go http.Serve(l, server)

	transport := &fhttp.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("tcp", l.Addr().String())
		},
		// Disable HTTP/2 for simplicity if needed, though fhttp might defaults.
	}

	provider := &MockTransportProvider{}
	provider.On("GetTransport", mock.Anything, mock.Anything).Return(transport)

	client := NewClient(registry, provider)

	req := &protocol.Request{
		ID:          "req-1",
		Method:      "GET",
		URL:         "http://example.com/foo",
		Fingerprint: "chrome-133",
	}

	ctx := context.Background()

	// Act
	resp, err := client.Do(ctx, req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)

	// Check Spans
	spans := exporter.GetSpans()
	assert.NotEmpty(t, spans)

	span := spans[0]
	assert.Equal(t, "upstream.request", span.Name)
	assert.Equal(t, "internal/endpoint/http", span.InstrumentationLibrary.Name)

	attrs := span.Attributes
	attrMap := make(map[string]interface{})
	for _, a := range attrs {
		attrMap[string(a.Key)] = a.Value.AsInterface()
	}

	assert.Equal(t, "GET", attrMap[string(semconv.HTTPRequestMethodKey)])
	assert.Equal(t, "http://example.com/foo", attrMap[string(semconv.URLFullKey)])
	assert.Equal(t, "example.com", attrMap[string(semconv.ServerAddressKey)])
	assert.Equal(t, int64(200), attrMap[string(semconv.HTTPResponseStatusCodeKey)])
	assert.Equal(t, "chrome-133", attrMap["endpoint.tls_fingerprint"])
	// Connection reused might be false for the first request
	assert.Equal(t, false, attrMap["endpoint.connection_reused"])
}

func TestClient_Do_Tracing_Error(t *testing.T) {
	// Setup In-Memory Exporter
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	otel.SetTracerProvider(tp)
	defer func() {
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
	}()

	registry := fingerprint.DefaultRegistry()

	// Transport that fails to dial
	transport := &fhttp.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		},
	}

	provider := &MockTransportProvider{}
	provider.On("GetTransport", mock.Anything, mock.Anything).Return(transport)

	client := NewClient(registry, provider)

	req := &protocol.Request{
		ID:          "req-2",
		Method:      "GET",
		URL:         "http://example.com/error",
		Fingerprint: "chrome-133",
	}

	// Act
	resp, err := client.Do(context.Background(), req)

	// Assert
	// The client returns a response with Error info, not a go error
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrCodeUpstreamError, resp.Error.Code)

	spans := exporter.GetSpans()
	assert.NotEmpty(t, spans)

	span := spans[0]
	assert.Equal(t, codes.Error, span.Status.Code)
	// We record the error message, but getting it from events needs checking Events
	assert.NotEmpty(t, span.Events)
	assert.Equal(t, "exception", span.Events[0].Name) // OpenTelemetry records errors as exception events
}
