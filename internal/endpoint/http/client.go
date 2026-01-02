// Package http provides an HTTP client wrapper for the Endpoint that integrates
// TLS fingerprinting and proper header ordering using the fhttp library.
package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http/httptrace"
	"net/url"
	"time"

	fhttp "github.com/useflyent/fhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/metrics"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// DefaultTimeout is the default request timeout if not specified.
const DefaultTimeout = 30 * time.Second

// DefaultMaxBodySize is the maximum response body size (10MB).
const DefaultMaxBodySize = 10 * 1024 * 1024

// TransportProvider defines the interface for obtaining a transport for a specific host and fingerprint.
type TransportProvider interface {
	GetTransport(host string, preset fingerprint.Preset) *fhttp.Transport
}

// Client wraps fhttp.Client with fingerprint integration and proper header ordering.
type Client struct {
	registry          *fingerprint.Registry
	transportProvider TransportProvider
	defaultTimeout    time.Duration
	maxBodySize       int64
	endpointID        string
}

// ClientOption is a functional option for configuring the Client.
type ClientOption func(*Client)

// WithDefaultTimeout sets the default request timeout.
func WithDefaultTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.defaultTimeout = d
	}
}

// WithMaxBodySize sets the maximum response body size.
func WithMaxBodySize(size int64) ClientOption {
	return func(c *Client) {
		c.maxBodySize = size
	}
}

// WithEndpointID sets the endpoint ID for response metadata.
func WithEndpointID(id string) ClientOption {
	return func(c *Client) {
		c.endpointID = id
	}
}

// NewClient creates a new HTTP client with fingerprint integration.
func NewClient(registry *fingerprint.Registry, transportProvider TransportProvider, opts ...ClientOption) *Client {
	c := &Client{
		registry:          registry,
		transportProvider: transportProvider,
		defaultTimeout:    DefaultTimeout,
		maxBodySize:       DefaultMaxBodySize,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Do executes an HTTP request with the specified fingerprint and returns the response.
func (c *Client) Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	// Start tracing span
	ctx, span := otel.Tracer("internal/endpoint/http").Start(ctx, "upstream.request")
	defer span.End()

	startTime := time.Now()

	// Capture connection info
	var reusedConn bool
	traceCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			reusedConn = info.Reused
		},
	})

	// Add initial attributes
	span.SetAttributes(
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLFullKey.String(req.URL),
		attribute.String("endpoint.tls_fingerprint", req.Fingerprint),
	)

	// Get fingerprint preset
	preset, ok := c.registry.Get(req.Fingerprint)
	if ok {
		metrics.TLSFingerprintUsed.WithLabelValues(req.Fingerprint).Inc()
	} else {
		// Fall back to default chrome preset if fingerprint not found
		preset, ok = c.registry.Get("chrome-133")
		if !ok {
			err := &ClientError{
				Code:    "FINGERPRINT_NOT_FOUND",
				Message: "fingerprint preset not found: " + req.Fingerprint,
			}
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
	}

	// Build fhttp request
	fhttpReq, err := BuildRequest(traceCtx, req, preset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to build request")
		return nil, err
	}

	// Extract host for connection pooling
	host := fhttpReq.URL.Host
	if host == "" {
		// Fallback if URL is incomplete but normally BuildRequest handles this
		if u, err := url.Parse(req.URL); err == nil {
			host = u.Host
		}
	}
	span.SetAttributes(semconv.ServerAddressKey.String(host))

	// Get transport via provider
	transport := c.transportProvider.GetTransport(host, preset)

	// Create client with transport
	client := &fhttp.Client{
		Transport: transport,
		Timeout:   c.getTimeout(req),
	}

	// Execute request
	var timing protocol.TimingInfo
	resp, err := client.Do(fhttpReq)

	// Record connection reuse (captured by trace hook during Do)
	span.SetAttributes(attribute.Bool("endpoint.connection_reused", reusedConn))

	if err != nil {
		timing.Total = time.Since(startTime)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return &protocol.Response{
			RequestID:  req.ID,
			EndpointID: c.endpointID,
			SessionID:  req.SessionID,
			Timing:     &timing,
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   err.Error(),
				Retryable: isRetryableError(err),
			},
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	timing.Total = time.Since(startTime)

	// Record response attributes
	span.SetAttributes(
		semconv.HTTPResponseStatusCodeKey.Int(resp.StatusCode),
	)

	// Build protocol response
	protocolResp, err := BuildResponse(req.ID, resp, timing, c.maxBodySize, c.endpointID, req.SessionID)

	// Record metrics
	status := "unknown"
	if err == nil {
		status = fmt.Sprintf("%d", protocolResp.StatusCode)
	} else {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to build response")
	}
	metrics.UpstreamDuration.WithLabelValues(host, status).Observe(time.Since(startTime).Seconds())

	return protocolResp, err
}

// getTimeout returns the timeout for the request.
func (c *Client) getTimeout(req *protocol.Request) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}
	return c.defaultTimeout
}

// isRetryableError determines if an error is retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	return false
}

// ClientError represents an HTTP client error.
type ClientError struct {
	Code    string
	Message string
}

func (e *ClientError) Error() string {
	return e.Code + ": " + e.Message
}

// Close cleans up client resources.
// Note: PooledTransport should be closed separately as it is injected.
func (c *Client) Close() error {
	// Nothing to close here anymore as transport is managed externally
	return nil
}

// NewRequest creates a new protocol.Request with the given parameters.
// This is a convenience function for testing and simple use cases.
func NewRequest(method, url string, body []byte) *protocol.Request {
	return &protocol.Request{
		ID:      generateRequestID(),
		Method:  method,
		URL:     url,
		Body:    body,
		Headers: protocol.HeaderMap{},
	}
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return time.Now().Format("20060102150405.000000000")
}

// ensure bytes import is used
var _ = bytes.Buffer{}
