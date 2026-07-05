package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	urlSchemeHTTP  = "http"
	urlSchemeHTTPS = "https"
)

// Ingress types used in routing and telemetry.
const (
	IngressTypeREST      = "rest"
	IngressTypeHTTPProxy = "http_proxy"
)

const (
	headerNameAuthorization      = "authorization"
	headerNameProxyAuthorization = "proxy-authorization"
	requestBodyExceedsLimit      = "request body exceeds limit"
)

var httpTokenAllowed = func() [256]bool {
	var allowed [256]bool

	for _, r := range "!#$%&'*+-.^_`|~" {
		allowed[byte(r)] = true
	}

	for r := byte('0'); r <= '9'; r++ {
		allowed[r] = true
	}

	for r := byte('A'); r <= 'Z'; r++ {
		allowed[r] = true
	}

	for r := byte('a'); r <= 'z'; r++ {
		allowed[r] = true
	}

	return allowed
}()

const (
	maxRequestHeaderCount = 64
	maxRequestHeaderBytes = 16384
	minRequestTimeoutMs   = 1000
)

// RequestEnvelope is the JSON shape for POST /api/v1/requests.
type RequestEnvelope struct {
	Method           string        `json:"method"`
	URL              string        `json:"url"`
	Headers          []HeaderPair  `json:"headers,omitempty"`
	Body             *RequestBody  `json:"body,omitempty"`
	Routing          *RoutingHints `json:"routing,omitempty"`
	FingerprintProto string        `json:"fingerprint_profile,omitempty"`
	TimeoutMs        uint64        `json:"timeout_ms,omitempty"`
	Replayable       bool          `json:"replayable"`
	CaptureHint      string        `json:"capture_hint,omitempty"`
}

// HeaderPair preserves header order and duplicates.
type HeaderPair struct {
	Name  string `json:"name"`
	Value string `json:"value_base64"`
}

// RequestBody is the only P0 body mode.
type RequestBody struct {
	Mode       string `json:"mode"`
	DataBase64 string `json:"data_base64,omitempty"`
	BodyRefID  string `json:"body_ref_id,omitempty"` // rejected in P0
}

// RoutingHints are client-supplied routing hints.
type RoutingHints struct {
	Tags            []string `json:"tags,omitempty"`
	Country         string   `json:"country,omitempty"`
	Region          string   `json:"region,omitempty"`
	IPType          string   `json:"ip_type,omitempty"`
	StickySessionID string   `json:"sticky_session_id,omitempty"`
}

// ValidatedRequest is the internal representation after validation.
type ValidatedRequest struct {
	Method          string
	URL             *url.URL
	Headers         []HeaderPair
	BodyData        []byte
	Routing         RoutingHints
	IngressType     string
	Fingerprint     string
	TimeoutMs       uint64
	Replayable      bool
	StickySessionID string
}

// ValidateRequest parses and validates a RequestEnvelope against config limits.
func ValidateRequest(raw []byte, maxRequestBodyBytes, maxTimeoutMs uint64) (*ValidatedRequest, error) {
	var env RequestEnvelope

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	err := dec.Decode(&env)
	if err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}

	err = validateMethod(env.Method)
	if err != nil {
		return nil, err
	}

	parsedURL, err := validateURL(env.URL)
	if err != nil {
		return nil, err
	}

	headers, err := validateHeaders(env.Headers)
	if err != nil {
		return nil, err
	}

	bodyData, err := validateBody(env.Body, maxRequestBodyBytes)
	if err != nil {
		return nil, err
	}

	timeoutMs, err := validateTimeout(env.TimeoutMs, maxTimeoutMs)
	if err != nil {
		return nil, err
	}

	err = validateCaptureHint(env.CaptureHint)
	if err != nil {
		return nil, err
	}

	routing := RoutingHints{}
	if env.Routing != nil {
		routing = *env.Routing
		routing.Tags = append([]string(nil), env.Routing.Tags...)
	}

	return &ValidatedRequest{
		Method:          env.Method,
		URL:             parsedURL,
		Headers:         headers,
		BodyData:        bodyData,
		Routing:         routing,
		IngressType:     IngressTypeREST,
		Fingerprint:     env.FingerprintProto,
		TimeoutMs:       timeoutMs,
		Replayable:      env.Replayable,
		StickySessionID: routing.StickySessionID,
	}, nil
}

func validateMethod(method string) error {
	if method == "" {
		return &ValidationError{Code: errorCodeInvalidRequest, Message: "method is required"}
	}

	if method != strings.ToUpper(method) {
		return &ValidationError{Code: errorCodeInvalidRequest, Message: "method must be uppercase"}
	}

	if method == "CONNECT" {
		return &ValidationError{Code: errorCodeUnsupportedIngressMode, Message: "CONNECT method is not supported in P0 REST transport"}
	}

	if !isKnownHTTPMethod(method) {
		return &ValidationError{Code: errorCodeInvalidRequest, Message: "method is not a supported HTTP method"}
	}

	for _, r := range method {
		if !isMethodChar(r) {
			return &ValidationError{Code: errorCodeInvalidRequest, Message: "method contains invalid characters"}
		}
	}

	return nil
}

func isKnownHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func isMethodChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') || r == '-'
}

func validateURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "url is required"}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "url is not a valid URL"}
	}

	if parsed.Scheme != urlSchemeHTTP && parsed.Scheme != urlSchemeHTTPS {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: fmt.Sprintf("url scheme must be %s or %s", urlSchemeHTTP, urlSchemeHTTPS)}
	}

	if parsed.Fragment != "" {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "url fragments are rejected"}
	}

	if parsed.User != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "url userinfo is rejected"}
	}

	if parsed.Host == "" {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "url host is empty"}
	}

	if strings.Contains(parsed.Host, "%") {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "IPv6 zone identifiers are rejected"}
	}

	return parsed, nil
}

func validateHeaders(headers []HeaderPair) ([]HeaderPair, error) {
	if len(headers) > maxRequestHeaderCount {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "header count exceeds maximum of 64"}
	}

	var totalBytes int

	for _, h := range headers {
		decodedLen, err := validateHeaderPair(h)
		if err != nil {
			return nil, err
		}

		totalBytes += len(h.Name) + decodedLen

		if totalBytes > maxRequestHeaderBytes {
			return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "aggregate header bytes exceed maximum of 16384"}
		}
	}

	return headers, nil
}

func validateHeaderPair(h HeaderPair) (int, error) {
	if len(h.Name) > maxRequestHeaderCount {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "header name exceeds maximum length of 64 bytes"}
	}

	if !isValidHTTPToken(h.Name) {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "header name is not a valid HTTP token"}
	}

	if strings.EqualFold(h.Name, "host") {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "client-supplied Host header is rejected"}
	}

	if strings.ContainsRune(h.Value, '\r') || strings.ContainsRune(h.Value, '\n') {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "header values must not contain CR or LF"}
	}

	decoded, err := base64.StdEncoding.DecodeString(h.Value)
	if err != nil {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "header value is not valid base64"}
	}

	if bytes.ContainsAny(decoded, "\r\n") {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "decoded header value must not contain CR or LF"}
	}

	return len(decoded), nil
}

func isValidHTTPToken(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r > 255 || !httpTokenAllowed[int(r)] {
			return false
		}
	}

	return true
}

func validateBody(body *RequestBody, maxBytes uint64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	if body.Mode != "" && body.Mode != "inline_base64" {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "body mode must be inline_base64 or omitted in P0"}
	}

	if body.BodyRefID != "" {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "BodyRef body is rejected in P0"}
	}

	if body.Mode == "" {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(body.DataBase64)
	if err != nil {
		return nil, &ValidationError{Code: errorCodeInvalidRequest, Message: "body data is not valid base64"}
	}

	if uint64(len(decoded)) > maxBytes {
		return nil, &ValidationError{Code: errorCodeBodyTooLarge, Message: requestBodyExceedsLimit, Details: map[string]string{
			errorDetailDirectionKey:  "request",
			errorDetailLimitBytesKey: strconv.FormatUint(maxBytes, 10),
		}}
	}

	return decoded, nil
}

func validateTimeout(timeoutMs, maxTimeoutMs uint64) (uint64, error) {
	if timeoutMs == 0 {
		return 0, nil
	}

	if timeoutMs < minRequestTimeoutMs {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "timeout_ms must be at least 1000"}
	}

	if timeoutMs > maxTimeoutMs {
		return 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "timeout_ms exceeds maximum"}
	}

	return timeoutMs, nil
}

func validateCaptureHint(hint string) error {
	if hint == "" || hint == "none" {
		return nil
	}

	return &ValidationError{Code: errorCodeInvalidRequest, Message: "capture_hint must be absent or none in P0"}
}

// ValidationError is a structured validation error.
type ValidationError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func (e *ValidationError) Error() string {
	return e.Message
}

// HTTPStatus returns the HTTP status code for this validation error.
func (e *ValidationError) HTTPStatus() int {
	switch e.Code {
	case errorCodeInvalidRequest:
		return http.StatusBadRequest
	case errorCodeBodyTooLarge:
		return http.StatusRequestEntityTooLarge
	case errorCodeUnsupportedIngressMode:
		return http.StatusBadRequest
	case "auth_failure":
		return http.StatusUnauthorized
	case "insufficient_permissions":
		return http.StatusForbidden
	case "destination_denied":
		return http.StatusForbidden
	default:
		return http.StatusBadRequest
	}
}
