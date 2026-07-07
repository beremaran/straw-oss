// Package objectstore is the P2 object-storage foundation for BodyRef and
// payload-capture body references (docs/planning/18-large-body-transport-p2.md,
// docs/planning/29-operational-behavior.md). It presigns scoped, single-object
// operations against an S3-compatible endpoint using SigV4 query signing from
// the standard library — no bucket listing is ever exposed, and every presigned
// URL is bound to one object key, one HTTP method, and a short expiry.
//
// This package is the foundation only. The request/response upload and download
// flows that construct a Client and call it live in
// docs/tasks/p2/07-bodyref-request-body-flow.md and
// docs/tasks/p2/08-bodyref-response-body-flow.md; payload-capture storage lives
// in docs/tasks/p2/11-payload-capture-storage.md.
package objectstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Direction labels a body object as a request or response body. It becomes a
// path segment in the object key, so the set is closed.
type Direction string

// Body directions used in object keys.
const (
	DirectionRequest  Direction = "request"
	DirectionResponse Direction = "response"
)

// Internal SigV4/object-key constants.
const (
	// nonceBytes is the entropy per object key. 128 bits makes keys unguessable
	// (docs/planning/18 Object Storage Security).
	nonceBytes = 16

	sigV4Service = "s3"
	sigV4Algo    = "AWS4-HMAC-SHA256"
	hostHeader   = "host"

	// sseHeader / sseValue enforce server-side encryption on uploads. The header
	// is signed into presigned PUT URLs so the object cannot be written without
	// it (docs/planning/18 Object Storage Security).
	sseHeader = "x-amz-server-side-encryption"
	sseValue  = "AES256"

	hoursPerDay       = 24
	errorBodyMaxBytes = 4096
)

// DefaultPresignExpiry and MaxPresignExpiry keep signed URLs short-lived.
const (
	DefaultPresignExpiry = 5 * time.Minute
	MaxPresignExpiry     = 15 * time.Minute
)

// DefaultRetentionDays and MaxRetentionDays bound body-object retention
// (docs/planning/18 Retention).
const (
	DefaultRetentionDays = 1
	MaxRetentionDays     = 3
)

// ErrUnavailable is the sentinel the BodyRef/capture flows (tasks 07/08/11) wrap
// object-storage transport failures with. Control maps it to the
// body_ref_unavailable error, matching the Section 29 outage row ("P2 BodyRef
// requests fail or fall back to direct streaming only if enabled and safe").
var ErrUnavailable = errors.New("object storage unavailable")

// Unavailable wraps err as an object-storage outage so callers can detect it
// with IsUnavailable and map it to body_ref_unavailable.
func Unavailable(err error) error {
	if err == nil {
		return ErrUnavailable
	}

	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

// IsUnavailable reports whether err is an object-storage outage.
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable)
}

var (
	errDisabled       = errors.New("object storage is not enabled")
	errEndpointEmpty  = errors.New("object storage endpoint is required")
	errEndpointScheme = errors.New("object storage endpoint must be an absolute http(s) URL")
	errBucketEmpty    = errors.New("object storage bucket is required")
	errRegionEmpty    = errors.New("object storage region is required")
	errAccessKeyEmpty = errors.New("object storage access key env is unset")
	errSecretKeyEmpty = errors.New("object storage secret key env is unset")
	errRetentionRange = errors.New("object storage retention days must be between 1 and 3")
	errIdentifier     = errors.New("tenant_id and request_id must be non-empty and free of '/'")
	errDirection      = errors.New("direction must be request or response")
	errEmptyKey       = errors.New("object key is required")
	errLifecycleApply = errors.New("put bucket lifecycle failed")
)

// Options configures a Client. Callers map their config into it; credentials are
// resolved from the named environment variables, never taken inline, so secrets
// stay out of config files (docs/planning/24-static-configuration.md).
type Options struct {
	Enabled       bool
	Endpoint      string
	Bucket        string
	Region        string
	AccessKeyEnv  string
	SecretKeyEnv  string
	RetentionDays int
}

// Client presigns scoped single-object operations. It holds resolved
// credentials and never offers a bucket-listing operation.
type Client struct {
	endpoint  *url.URL
	bucket    string
	region    string
	accessKey string
	secretKey string
	retention time.Duration
	now       func() time.Time
}

// New resolves credentials and validates cfg, returning a ready Client. It
// returns errDisabled when object storage is not enabled so callers can treat
// "disabled" distinctly from "misconfigured".
func New(opts Options) (*Client, error) {
	endpoint, err := validateOptions(opts)
	if err != nil {
		return nil, err
	}

	accessKey := os.Getenv(opts.AccessKeyEnv)
	if opts.AccessKeyEnv == "" || accessKey == "" {
		return nil, errAccessKeyEmpty
	}

	secretKey := os.Getenv(opts.SecretKeyEnv)
	if opts.SecretKeyEnv == "" || secretKey == "" {
		return nil, errSecretKeyEmpty
	}

	return &Client{
		endpoint:  endpoint,
		bucket:    opts.Bucket,
		region:    opts.Region,
		accessKey: accessKey,
		secretKey: secretKey,
		retention: time.Duration(opts.RetentionDays) * 24 * time.Hour,
		now:       time.Now,
	}, nil
}

// validateOptions checks non-credential fields and returns the parsed endpoint.
func validateOptions(opts Options) (*url.URL, error) {
	if !opts.Enabled {
		return nil, errDisabled
	}

	endpoint, err := parseEndpoint(opts.Endpoint)
	if err != nil {
		return nil, err
	}

	if opts.Bucket == "" {
		return nil, errBucketEmpty
	}

	if opts.Region == "" {
		return nil, errRegionEmpty
	}

	if opts.RetentionDays < DefaultRetentionDays || opts.RetentionDays > MaxRetentionDays {
		return nil, errRetentionRange
	}

	return endpoint, nil
}

func parseEndpoint(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errEndpointEmpty
	}

	endpoint, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse object storage endpoint: %w", err)
	}

	if (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, errEndpointScheme
	}

	return endpoint, nil
}

// Retention returns the configured body-object retention.
func (c *Client) Retention() time.Duration { return c.retention }

// ApplyLifecycleRetention installs the bucket backstop that expires BodyRef
// objects left behind when Control crashes after upload but before cleanup.
func (c *Client) ApplyLifecycleRetention(ctx context.Context) error {
	body, err := xml.Marshal(lifecycleConfiguration{
		XMLName: xml.Name{Local: "LifecycleConfiguration"},
		Rules: []lifecycleRule{{
			ID:     "straw-body-object-retention",
			Status: "Enabled",
			Filter: lifecycleFilter{Prefix: "tenant/"},
			Expiration: lifecycleExpiration{
				Days: int(c.retention / (hoursPerDay * time.Hour)),
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("marshal lifecycle retention: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint.Scheme+"://"+c.endpoint.Host+"/"+awsURIEncode(c.bucket, false)+"?lifecycle", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build lifecycle request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml")
	c.signRequest(req, body)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Unavailable(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyMaxBytes))

		return Unavailable(fmt.Errorf("%w: status %d: %s", errLifecycleApply, resp.StatusCode, strings.TrimSpace(string(msg))))
	}

	return nil
}

type lifecycleConfiguration struct {
	XMLName xml.Name        `xml:"LifecycleConfiguration"`
	Rules   []lifecycleRule `xml:"Rule"`
}

type lifecycleRule struct {
	ID         string              `xml:"ID"`
	Status     string              `xml:"Status"`
	Filter     lifecycleFilter     `xml:"Filter"`
	Expiration lifecycleExpiration `xml:"Expiration"`
}

type lifecycleFilter struct {
	Prefix string `xml:"Prefix"`
}

type lifecycleExpiration struct {
	Days int `xml:"Days"`
}

// ObjectKey builds the tenant/request-scoped, high-entropy object key from
// docs/planning/18: tenant/<tenant_id>/request/<request_id>/<direction>/<nonce>.
// Identifiers containing '/' are rejected so a caller cannot escape its tenant
// prefix.
func (c *Client) ObjectKey(tenantID, requestID string, dir Direction) (string, error) {
	if !safeIdentifier(tenantID) || !safeIdentifier(requestID) {
		return "", errIdentifier
	}

	if dir != DirectionRequest && dir != DirectionResponse {
		return "", errDirection
	}

	nonce := make([]byte, nonceBytes)

	_, err := rand.Read(nonce)
	if err != nil {
		return "", fmt.Errorf("generate object key nonce: %w", err)
	}

	return fmt.Sprintf("tenant/%s/request/%s/%s/%s", tenantID, requestID, dir, hex.EncodeToString(nonce)), nil
}

func safeIdentifier(s string) bool {
	return s != "" && !strings.Contains(s, "/")
}

// PresignedPut is a scoped upload URL plus the headers the uploader must send.
// Headers carries the signed SSE header, so the object cannot be written without
// server-side encryption.
type PresignedPut struct {
	URL     string
	Headers map[string]string
}

// PresignedDelete is a scoped cleanup URL.
type PresignedDelete struct {
	URL string
}

// PresignGet returns a short-lived SigV4 URL that grants GET on exactly one
// object key. The signature is bound to the key, so it grants no access to any
// other object and reveals no bucket-listing capability.
func (c *Client) PresignGet(key string, expiry time.Duration) (string, error) {
	return c.presign("GET", key, expiry, nil)
}

// PresignPut returns a short-lived SigV4 URL that grants PUT on exactly one
// object key, with SSE signed in. The returned Headers must accompany the PUT.
func (c *Client) PresignPut(key string, expiry time.Duration) (PresignedPut, error) {
	signed, err := c.presign("PUT", key, expiry, map[string]string{sseHeader: sseValue})
	if err != nil {
		return PresignedPut{}, err
	}

	return PresignedPut{URL: signed, Headers: map[string]string{sseHeader: sseValue}}, nil
}

// PresignDelete returns a short-lived SigV4 URL that grants DELETE on exactly
// one object key. Control uses this for best-effort cleanup after upload
// cancellation or request-stream publication failure.
func (c *Client) PresignDelete(key string, expiry time.Duration) (PresignedDelete, error) {
	signed, err := c.presign("DELETE", key, expiry, nil)
	if err != nil {
		return PresignedDelete{}, err
	}

	return PresignedDelete{URL: signed}, nil
}

// presign implements S3 SigV4 query (presigned URL) signing over the standard
// library. extraSigned headers (beyond host) are folded into SignedHeaders so
// the request must carry them verbatim.
func (c *Client) presign(method, key string, expiry time.Duration, extraSigned map[string]string) (string, error) {
	if key == "" {
		return "", errEmptyKey
	}

	expiry = clampExpiry(expiry)
	now := c.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	scope := strings.Join([]string{date, c.region, sigV4Service, "aws4_request"}, "/")

	signedHeaderNames, canonicalHeaders := c.canonicalHeaders(extraSigned)

	query := map[string]string{
		"X-Amz-Algorithm":     sigV4Algo,
		"X-Amz-Credential":    c.accessKey + "/" + scope,
		"X-Amz-Date":          amzDate,
		"X-Amz-Expires":       strconv.Itoa(int(expiry.Seconds())),
		"X-Amz-SignedHeaders": signedHeaderNames,
	}

	canonicalURI := "/" + awsURIEncode(c.bucket, false) + "/" + awsURIEncode(key, false)
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQueryString(query),
		canonicalHeaders,
		signedHeaderNames,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		sigV4Algo,
		amzDate,
		scope,
		hex.EncodeToString(canonicalHash[:]),
	}, "\n")

	signature := hmacSHA256Hex(awsSigningKey(c.secretKey, date, c.region, sigV4Service), []byte(stringToSign))
	query["X-Amz-Signature"] = signature

	return c.endpoint.Scheme + "://" + c.endpoint.Host + canonicalURI + "?" + canonicalQueryString(query), nil
}

func clampExpiry(expiry time.Duration) time.Duration {
	if expiry <= 0 {
		return DefaultPresignExpiry
	}

	if expiry > MaxPresignExpiry {
		return MaxPresignExpiry
	}

	return expiry
}

func (c *Client) canonicalHeaders(extra map[string]string) (string, string) {
	headers := map[string]string{hostHeader: c.endpoint.Host}
	for name, value := range extra {
		headers[strings.ToLower(name)] = strings.TrimSpace(value)
	}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}

	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(headers[name])
		b.WriteByte('\n')
	}

	return strings.Join(names, ";"), b.String()
}

func (c *Client) signRequest(req *http.Request, payload []byte) {
	now := c.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	payloadHash := sha256.Sum256(payload)
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	req.Header.Set("Host", c.endpoint.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHashHex)

	headers := map[string]string{
		hostHeader:             c.endpoint.Host,
		"x-amz-content-sha256": payloadHashHex,
		"x-amz-date":           amzDate,
	}

	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if lower == "authorization" || len(values) == 0 {
			continue
		}

		headers[lower] = strings.TrimSpace(strings.Join(values, ","))
	}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}

	sort.Strings(names)

	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(headers[name])
		canonicalHeaders.WriteByte('\n')
	}

	scope := strings.Join([]string{date, c.region, sigV4Service, "aws4_request"}, "/")
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalRawQuery(req.URL.RawQuery),
		canonicalHeaders.String(),
		strings.Join(names, ";"),
		payloadHashHex,
	}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		sigV4Algo,
		amzDate,
		scope,
		hex.EncodeToString(canonicalHash[:]),
	}, "\n")
	signature := hmacSHA256Hex(awsSigningKey(c.secretKey, date, c.region, sigV4Service), []byte(stringToSign))

	req.Header.Set("Authorization", sigV4Algo+" Credential="+c.accessKey+"/"+scope+", SignedHeaders="+strings.Join(names, ";")+", Signature="+signature)
}

func canonicalRawQuery(raw string) string {
	if raw == "" {
		return ""
	}

	if !strings.Contains(raw, "=") {
		return awsURIEncode(raw, true) + "="
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}

	parts := make([]string, 0)

	for key, vals := range values {
		if len(vals) == 0 {
			parts = append(parts, awsURIEncode(key, true)+"=")

			continue
		}

		for _, val := range vals {
			parts = append(parts, awsURIEncode(key, true)+"="+awsURIEncode(val, true))
		}
	}

	sort.Strings(parts)

	return strings.Join(parts, "&")
}

func canonicalQueryString(query map[string]string) string {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, awsURIEncode(k, true)+"="+awsURIEncode(query[k], true))
	}

	return strings.Join(parts, "&")
}

// awsURIEncode encodes s per the SigV4 rules: every byte except the unreserved
// set is percent-encoded; '/' is preserved when encodeSlash is false (paths).
func awsURIEncode(s string, encodeSlash bool) string {
	var b strings.Builder

	for i := range len(s) {
		ch := s[i]
		switch {
		case isUnreserved(ch), ch == '/' && !encodeSlash:
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}

	return b.String()
}

func isUnreserved(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') ||
		ch == '-' || ch == '.' || ch == '_' || ch == '~'
}

func awsSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))

	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, raw []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)

	return mac.Sum(nil)
}

func hmacSHA256Hex(key, raw []byte) string {
	return hex.EncodeToString(hmacSHA256(key, raw))
}
