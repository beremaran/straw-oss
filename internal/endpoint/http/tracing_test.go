package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

	mockProvider := &MockTransportProvider{}
	mockProvider.On("GetTransport", mock.Anything, mock.Anything).Return(&fhttp.Transport{})

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	otel.SetTracerProvider(tp)

	defer func() {
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
	}()

	registry := fingerprint.DefaultRegistry()

	server := http.NewServeMux()
	server.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

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

	resp, err := client.Do(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)

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

	assert.Equal(t, false, attrMap["endpoint.connection_reused"])
}

func TestClient_Do_Tracing_Error(t *testing.T) {

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)

	otel.SetTracerProvider(tp)
	defer func() {
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
	}()

	registry := fingerprint.DefaultRegistry()

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

	resp, err := client.Do(context.Background(), req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrCodeUpstreamError, resp.Error.Code)

	spans := exporter.GetSpans()
	assert.NotEmpty(t, spans)

	span := spans[0]
	assert.Equal(t, codes.Error, span.Status.Code)

	assert.NotEmpty(t, span.Events)
	assert.Equal(t, "exception", span.Events[0].Name)
}
