package objectstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	defaultS3Region     = "us-east-1"
	maxS3ErrorBodyBytes = 4 << 10
)

// S3 is a dependency-free, path-style S3-compatible Store. Endpoint should
// include the scheme and host; Bucket is appended as the first path segment.
type S3 struct {
	Endpoint, Bucket, Region, AccessKey, SecretKey, SessionToken string
	ServerSideEncryption, KMSKeyID                               string
	Transport                                                    http.RoundTripper
	Now                                                          func() time.Time
}

// Put writes an object using an authenticated S3 PUT request.
func (s S3) Put(ctx context.Context, key string, body io.Reader, size int64, metadata map[string]string) error {
	req, err := s.request(ctx, http.MethodPut, key, nil, body)
	if err != nil {
		return err
	}

	if size >= 0 {
		req.ContentLength = size
	}

	for name, value := range metadata {
		req.Header.Set("x-amz-meta-"+strings.ToLower(name), value)
	}

	if s.ServerSideEncryption != "" {
		req.Header.Set("x-amz-server-side-encryption", s.ServerSideEncryption)
	}

	if s.KMSKeyID != "" {
		req.Header.Set("x-amz-server-side-encryption-aws-kms-key-id", s.KMSKeyID)
	}

	s.sign(req, s.now())

	return s.doNoBody(req)
}

// Open opens an object using an authenticated S3 GET request.
func (s S3) Open(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	req, err := s.request(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, Object{}, err
	}

	resp, err := s.do(req)
	if err != nil {
		return nil, Object{}, err
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()

		return nil, Object{}, ErrNotFound
	}

	if !successStatus(resp.StatusCode) {
		defer func() { _ = resp.Body.Close() }()

		return nil, Object{}, s.statusError(req.Method, resp)
	}

	object := Object{Key: key, Size: resp.ContentLength, Metadata: map[string]string{}}

	modified, parseErr := http.ParseTime(resp.Header.Get("Last-Modified"))
	if parseErr == nil {
		object.LastModified = modified
	}

	for name, values := range resp.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-amz-meta-") && len(values) > 0 {
			object.Metadata[strings.TrimPrefix(lower, "x-amz-meta-")] = values[0]
		}
	}

	return resp.Body, object, nil
}

// Delete removes an S3 object. Missing objects are ignored.
func (s S3) Delete(ctx context.Context, key string) error {
	req, err := s.request(ctx, http.MethodDelete, key, nil, nil)
	if err != nil {
		return err
	}

	return s.doNoBody(req)
}

// List returns S3 objects with the requested key prefix.
func (s S3) List(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object

	continuation := ""
	for {
		page, err := s.listPage(ctx, prefix, continuation)
		if err != nil {
			return nil, err
		}

		for _, item := range page.Contents {
			modified, _ := time.Parse(time.RFC3339, item.LastModified)
			out = append(out, Object{Key: item.Key, Size: item.Size, LastModified: modified})
		}

		if !page.IsTruncated || page.NextContinuationToken == "" {
			return out, nil
		}

		continuation = page.NextContinuationToken
	}
}

type s3ListResult struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
	} `xml:"Contents"`
}

const maxS3ListResponseBytes = 4 << 20

func (s S3) listPage(ctx context.Context, prefix, continuation string) (s3ListResult, error) {
	query := url.Values{"list-type": {"2"}, "prefix": {prefix}}
	if continuation != "" {
		query.Set("continuation-token", continuation)
	}

	req, err := s.request(ctx, http.MethodGet, "", query, nil)
	if err != nil {
		return s3ListResult{}, err
	}

	resp, err := s.do(req)
	if err != nil {
		return s3ListResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if !successStatus(resp.StatusCode) {
		return s3ListResult{}, s.statusError("LIST", resp)
	}

	page, err := decodeS3List(resp.Body)
	if err != nil {
		return s3ListResult{}, fmt.Errorf("decode S3 object list: %w", err)
	}

	return page, nil
}

func decodeS3List(reader io.Reader) (s3ListResult, error) {
	var page s3ListResult

	err := xml.NewDecoder(io.LimitReader(reader, maxS3ListResponseBytes)).Decode(&page)
	if err != nil {
		return s3ListResult{}, fmt.Errorf("decode bounded S3 list: %w", err)
	}

	return page, nil
}

func (s S3) request(ctx context.Context, method, key string, query url.Values, body io.Reader) (*http.Request, error) {
	objectURL, err := s.objectURL(key, query)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, objectURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build S3 request: %w", err)
	}

	now := s.now()

	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("x-amz-date", now.Format("20060102T150405Z"))

	if s.SessionToken != "" {
		req.Header.Set("x-amz-security-token", s.SessionToken)
	}

	s.sign(req, now)

	return req, nil
}

func (s S3) objectURL(key string, query url.Values) (*url.URL, error) {
	if s.Endpoint == "" || s.Bucket == "" || s.AccessKey == "" || s.SecretKey == "" {
		return nil, errS3Config
	}

	base, err := url.Parse(strings.TrimRight(s.Endpoint, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, errS3Endpoint
	}

	base.Path = strings.TrimRight(base.Path, "/") + "/" + pathEscape(s.Bucket)
	if key != "" {
		base.Path += "/" + pathEscape(key)
	}

	base.RawQuery = query.Encode()

	return base, nil
}

func (s S3) sign(req *http.Request, now time.Time) {
	headers := s.signingHeaders(req)

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}

	sort.Strings(names)

	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.TrimSpace(headers[name]))
		canonicalHeaders.WriteByte('\n')
	}

	signedHeaders := strings.Join(names, ";")
	canonical := strings.Join([]string{req.Method, req.URL.EscapedPath(), req.URL.Query().Encode(), canonicalHeaders.String(), signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
	date := now.Format("20060102")

	region := s.Region
	if region == "" {
		region = defaultS3Region
	}

	scope := date + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + now.Format("20060102T150405Z") + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	dateKey := hmacSHA256([]byte("AWS4"+s.SecretKey), date)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, "s3")
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func (s S3) signingHeaders(req *http.Request) map[string]string {
	headers := map[string]string{"host": req.URL.Host, "x-amz-content-sha256": req.Header.Get("x-amz-content-sha256"), "x-amz-date": req.Header.Get("x-amz-date")}
	if token := req.Header.Get("x-amz-security-token"); token != "" {
		headers["x-amz-security-token"] = token
	}

	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-amz-meta-") || lower == "x-amz-server-side-encryption" || lower == "x-amz-server-side-encryption-aws-kms-key-id" {
			headers[lower] = strings.Join(values, ",")
		}
	}

	return headers
}

func (s S3) doNoBody(req *http.Request) error {
	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound && req.Method == http.MethodDelete {
		return nil
	}

	if !successStatus(resp.StatusCode) {
		return s.statusError(req.Method, resp)
	}

	return nil
}

func (s S3) do(req *http.Request) (*http.Response, error) {
	transport := s.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("perform S3 request: %w", err)
	}

	return resp, nil
}

func (s S3) statusError(method string, resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxS3ErrorBodyBytes))

	return fmt.Errorf("%w: %s status %d: %s", errS3Status, method, resp.StatusCode, strings.TrimSpace(string(raw)))
}

func (s S3) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}

	return time.Now().UTC()
}

func successStatus(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func hmacSHA256(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))

	return h.Sum(nil)
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)

	return hex.EncodeToString(sum[:])
}

func pathEscape(value string) string {
	parts := strings.Split(value, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}

	return strings.Join(parts, "/")
}
