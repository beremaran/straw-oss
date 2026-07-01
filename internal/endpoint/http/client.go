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
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/internal/protocol"
)

// DefaultTimeout is the default timeout for HTTP requests.
const DefaultTimeout = 30 * time.Second

// DefaultMaxBodySize is the default maximum response body size (10 MB).
const DefaultMaxBodySize = 10 * 1024 * 1024

// TransportProvider provides HTTP transports for different hosts and presets.
type TransportProvider interface {
	GetTransport(host string, preset fingerprint.Preset) *fhttp.Transport
}

// Client makes HTTP requests with TLS fingerprinting support.
type Client struct {
	registry          *fingerprint.Registry
	transportProvider TransportProvider
	defaultTimeout    time.Duration
	maxBodySize       int64
	endpointID        string
	mu                sync.Mutex
	tlsClients        map[string]tls_client.HttpClient
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithDefaultTimeout sets the request timeout.
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

// WithEndpointID sets the endpoint identifier.
func WithEndpointID(id string) ClientOption {
	return func(c *Client) {
		c.endpointID = id
	}
}

// NewClient creates a new Client with the given registry and transport provider.
func NewClient(registry *fingerprint.Registry, transportProvider TransportProvider, opts ...ClientOption) *Client {
	c := &Client{
		registry:          registry,
		transportProvider: transportProvider,
		defaultTimeout:    DefaultTimeout,
		maxBodySize:       DefaultMaxBodySize,
		tlsClients:        make(map[string]tls_client.HttpClient),
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

type dummyCookieJar struct{}

func (d *dummyCookieJar) SetCookies(_ *url.URL, _ []*fhttp.Cookie) {}
func (d *dummyCookieJar) Cookies(_ *url.URL) []*fhttp.Cookie       { return nil }

var presetProfiles = map[string]profiles.ClientProfile{
	fingerprint.DefaultPresetID: profiles.Chrome_133,
	"chrome-131":                profiles.Chrome_131,
	"chrome-129":                profiles.Chrome_120,
	"firefox-133":               profiles.Firefox_133,
	"firefox-120":               profiles.Firefox_120,
	"safari-18":                 profiles.Safari_IOS_18_0,
	"safari-17":                 profiles.Safari_IOS_17_0,
	"edge-130":                  profiles.Chrome_133,
	"edge-106":                  profiles.Chrome_106,
}

func mapPresetToProfile(presetID string) (profiles.ClientProfile, bool) {
	profile, ok := presetProfiles[presetID]
	if !ok {
		return profiles.Chrome_133, false
	}

	return profile, true
}

// Do executes an HTTP request and returns the response.
func (c *Client) Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	startTime := time.Now()

	preset, err := resolveClientPreset(c)
	if err != nil {
		return nil, err
	}

	fhttpReq, err := BuildRequest(ctx, req, preset)
	if err != nil {
		return nil, err
	}

	host := requestHost(req.URL, fhttpReq)

	timeout := c.getTimeout(req)

	client, err := c.getTLSClient(preset.ID, timeout, clientDialContext(c, host, preset))
	if err != nil {
		return nil, fmt.Errorf("get tls client: %w", err)
	}

	var timing protocol.TimingInfo

	resp, err := client.Do(fhttpReq)
	if err != nil {
		timing.Total = time.Since(startTime)

		return upstreamErrorResponse(req, c.endpointID, timing, err), nil
	}

	defer func() { _ = resp.Body.Close() }()

	timing.Total = time.Since(startTime)

	return BuildResponseWithOptions(req.ID, resp, timing, responseOptions(c, req), c.endpointID)
}

func upstreamErrorResponse(
	req *protocol.Request,
	endpointID string,
	timing protocol.TimingInfo,
	err error,
) *protocol.Response {
	return &protocol.Response{
		RequestID:  req.ID,
		EndpointID: endpointID,
		Timing:     &timing,
		Error: &protocol.ErrorInfo{
			Code:      protocol.ErrCodeUpstreamError,
			Message:   err.Error(),
			Retryable: isRetryableError(err),
		},
	}
}

func responseOptions(c *Client, req *protocol.Request) ResponseOptions {
	opts := ResponseOptions{
		MaxBodySize: c.maxBodySize,
	}

	if req.MaxResponseSize > 0 {
		opts.MaxBodySize = req.MaxResponseSize
	}

	return opts
}

func resolveClientPreset(c *Client) (fingerprint.Preset, error) {
	preset, ok := c.registry.Get(fingerprint.DefaultPresetID)
	if ok {
		return preset, nil
	}

	return fingerprint.Preset{}, &ClientError{
		Code:    "FINGERPRINT_NOT_FOUND",
		Message: "fingerprint preset not found: " + fingerprint.DefaultPresetID,
	}
}

func requestHost(reqURL string, req *fhttp.Request) string {
	if req.URL.Host != "" {
		return req.URL.Host
	}

	u, err := url.Parse(reqURL)
	if err != nil {
		return ""
	}

	return u.Host
}

func clientDialContext(
	c *Client,
	host string,
	preset fingerprint.Preset,
) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if c.transportProvider == nil {
		return nil
	}

	transport := c.transportProvider.GetTransport(host, preset)
	if transport == nil {
		return nil
	}

	return transport.DialContext
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

	c.tlsClients = make(map[string]tls_client.HttpClient)

	return nil
}

func (c *Client) getTLSClient(presetID string, timeout time.Duration, dialContext func(ctx context.Context, network, addr string) (net.Conn, error)) (tls_client.HttpClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s_%s_%p", presetID, timeout, dialContext)
	if client, ok := c.tlsClients[key]; ok {
		return client, nil
	}

	profile, _ := mapPresetToProfile(presetID)

	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profile),
		tls_client.WithTimeoutSeconds(int(timeout.Seconds())),
		tls_client.WithCookieJar(&dummyCookieJar{}),
	}

	if dialContext != nil {
		options = append(options, tls_client.WithDialContext(dialContext))
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("creating tls client: %w", err)
	}

	c.tlsClients[key] = client

	return client, nil
}

func (c *Client) getTimeout(req *protocol.Request) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}

	return c.defaultTimeout
}

// NewRequest creates a new protocol request with a generated ID.
func NewRequest(method, url string, body []byte) *protocol.Request {
	return &protocol.Request{
		ID:      generateRequestID(),
		Method:  method,
		URL:     url,
		Body:    body,
		Headers: protocol.HeaderMap{},
	}
}

func generateRequestID() string {
	return time.Now().Format("20060102150405.000000000")
}
