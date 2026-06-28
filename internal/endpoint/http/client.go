//nolint:funcorder
package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http/httptrace"
	"net/url"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/internal/endpoint/metrics"
	"github.com/beremaran/straw/pkg/protocol"
)

const DefaultTimeout = 30 * time.Second

const DefaultMaxBodySize = 10 * 1024 * 1024

type TransportProvider interface {
	GetTransport(host string, preset fingerprint.Preset) *fhttp.Transport
}

type Client struct {
	registry          *fingerprint.Registry
	transportProvider TransportProvider
	defaultTimeout    time.Duration
	maxBodySize       int64
	endpointID        string

	mu         sync.Mutex
	tlsClients map[string]tls_client.HttpClient
}

type ClientOption func(*Client)

func WithDefaultTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.defaultTimeout = d
	}
}

func WithMaxBodySize(size int64) ClientOption {
	return func(c *Client) {
		c.maxBodySize = size
	}
}

func WithEndpointID(id string) ClientOption {
	return func(c *Client) {
		c.endpointID = id
	}
}

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

func (d *dummyCookieJar) SetCookies(u *url.URL, cookies []*fhttp.Cookie) {}
func (d *dummyCookieJar) Cookies(u *url.URL) []*fhttp.Cookie             { return nil }

var presetProfiles = map[string]profiles.ClientProfile{
	"chrome-133":  profiles.Chrome_133,
	"chrome-131":  profiles.Chrome_131,
	"chrome-129":  profiles.Chrome_120,
	"firefox-133": profiles.Firefox_133,
	"firefox-120": profiles.Firefox_120,
	"safari-18":   profiles.Safari_IOS_18_0,
	"safari-17":   profiles.Safari_IOS_17_0,
	"edge-130":    profiles.Chrome_133,
	"edge-106":    profiles.Chrome_106,
}

func mapPresetToProfile(presetID string) (profiles.ClientProfile, bool) {
	profile, ok := presetProfiles[presetID]
	if !ok {
		return profiles.Chrome_133, false
	}

	return profile, true
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
		return nil, err
	}

	c.tlsClients[key] = client

	return client, nil
}

//nolint:cyclop,funlen
func (c *Client) Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	ctx, span := otel.Tracer("internal/endpoint/http").Start(ctx, "upstream.request")
	defer span.End()

	startTime := time.Now()

	var reusedConn bool
	traceCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			reusedConn = info.Reused
		},
	})

	span.SetAttributes(
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLFullKey.String(req.URL),
		attribute.String("endpoint.tls_fingerprint", req.Fingerprint),
	)

	preset, ok := c.registry.Get(req.Fingerprint)
	if ok {
		metrics.TLSFingerprintUsed.WithLabelValues(req.Fingerprint).Inc()
	} else {
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

	fhttpReq, err := BuildRequest(traceCtx, req, preset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to build request")

		return nil, err
	}

	host := fhttpReq.URL.Host
	if host == "" {
		var u *url.URL
		u, err = url.Parse(req.URL)
		if err == nil {
			host = u.Host
		}
	}
	span.SetAttributes(semconv.ServerAddressKey.String(host))

	var dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	if c.transportProvider != nil {
		if t := c.transportProvider.GetTransport(host, preset); t != nil {
			dialContext = t.DialContext
		}
	}

	timeout := c.getTimeout(req)
	client, err := c.getTLSClient(preset.ID, timeout, dialContext)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get tls client: "+err.Error())

		return nil, err
	}

	var timing protocol.TimingInfo
	resp, err := client.Do(fhttpReq)

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

	span.SetAttributes(
		semconv.HTTPResponseStatusCodeKey.Int(resp.StatusCode),
	)

	opts := ResponseOptions{
		MaxBodySize:    c.maxBodySize,
		StreamResponse: req.StreamResponse,
	}

	if req.MaxResponseSize > 0 {
		opts.MaxBodySize = req.MaxResponseSize
	}
	protocolResp, err := BuildResponseWithOptions(req.ID, resp, timing, opts, c.endpointID, req.SessionID)

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

func (c *Client) getTimeout(req *protocol.Request) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}

	return c.defaultTimeout
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

type ClientError struct {
	Code    string
	Message string
}

func (e *ClientError) Error() string {
	return e.Code + ": " + e.Message
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, client := range c.tlsClients {
		client.CloseIdleConnections()
	}
	c.tlsClients = make(map[string]tls_client.HttpClient)

	return nil
}

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

var _ = bytes.Buffer{}
