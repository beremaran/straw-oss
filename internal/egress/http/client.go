// Package http provides an HTTP client for making upstream requests with TLS fingerprinting support.
package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/httptrace"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"github.com/beremaran/straw/internal/protocol"
	"github.com/beremaran/straw/internal/protocol/wirepb"
)

// DefaultTimeout is the default timeout for HTTP requests.
const DefaultTimeout = 30 * time.Second

// DefaultMaxBodySize is the default maximum response body size (10 MB).
const DefaultMaxBodySize = 10 * 1024 * 1024

// Client makes HTTP requests with a Chrome TLS profile.
type Client struct {
	defaultTimeout time.Duration
	maxBodySize    int64
	egressID       string
	mu             sync.Mutex
	tlsClients     map[time.Duration]tls_client.HttpClient
}

// NewClient creates a new Client.
func NewClient(egressID string) *Client {
	return &Client{
		defaultTimeout: DefaultTimeout,
		maxBodySize:    DefaultMaxBodySize,
		egressID:       egressID,
		tlsClients:     make(map[time.Duration]tls_client.HttpClient),
	}
}

type dummyCookieJar struct{}

func (d *dummyCookieJar) SetCookies(_ *url.URL, _ []*fhttp.Cookie) {}
func (d *dummyCookieJar) Cookies(_ *url.URL) []*fhttp.Cookie       { return nil }

// Do executes an HTTP request and returns the response.
func (c *Client) Do(ctx context.Context, req *wirepb.Request) (*wirepb.Response, error) {
	fhttpReq, err := BuildRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	timeout := c.getTimeout(req)

	client, err := c.getTLSClient(timeout)
	if err != nil {
		return nil, fmt.Errorf("get tls client: %w", err)
	}

	timing := &wirepb.TimingInfo{}
	recorder := newRequestTiming(timing)
	fhttpReq = fhttpReq.WithContext(httptrace.WithClientTrace(ctx, recorder.trace))

	resp, err := client.Do(fhttpReq)

	recorder.finish()

	if err != nil {
		return upstreamErrorResponse(req, c.egressID, timing, err), nil
	}

	defer func() { _ = resp.Body.Close() }()

	return BuildResponse(req.GetId(), resp, timing, c.maxResponseSize(req), c.egressID)
}

func upstreamErrorResponse(
	req *wirepb.Request,
	egressID string,
	timing *wirepb.TimingInfo,
	err error,
) *wirepb.Response {
	return &wirepb.Response{
		RequestId: req.GetId(),
		EgressId:  egressID,
		Timing:    timing,
		Error: &wirepb.ErrorInfo{
			Code:      protocol.ErrCodeUpstreamError,
			Message:   err.Error(),
			Retryable: isRetryableError(err),
		},
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

// ClientError represents a client-side error.
type ClientError struct {
	Code    string
	Message string
}

func (e *ClientError) Error() string {
	return e.Code + ": " + e.Message
}

// Close closes idle connections in the client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, client := range c.tlsClients {
		client.CloseIdleConnections()
	}

	c.tlsClients = make(map[time.Duration]tls_client.HttpClient)

	return nil
}

func (c *Client) maxResponseSize(req *wirepb.Request) int64 {
	if req.GetMaxResponseSize() > 0 {
		return req.GetMaxResponseSize()
	}

	return c.maxBodySize
}

func (c *Client) getTLSClient(timeout time.Duration) (tls_client.HttpClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if client, ok := c.tlsClients[timeout]; ok {
		return client, nil
	}

	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profiles.Chrome_133),
		tls_client.WithTimeoutSeconds(int(timeout.Seconds())),
		tls_client.WithCookieJar(&dummyCookieJar{}),
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("creating tls client: %w", err)
	}

	c.tlsClients[timeout] = client

	return client, nil
}

func (c *Client) getTimeout(req *wirepb.Request) time.Duration {
	if req.GetTimeoutNanos() > 0 {
		return time.Duration(req.GetTimeoutNanos())
	}

	return c.defaultTimeout
}
