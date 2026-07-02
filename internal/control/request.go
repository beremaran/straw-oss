package control

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	TimeoutMs       uint64
	Replayable      bool
	StickySessionID string
}

var (
	errMethodRequired     = errors.New("method is required")
	errURLRequired        = errors.New("url is required")
	errURLScheme          = errors.New("url scheme must be http or https")
	errURLFragment        = errors.New("url fragments are rejected")
	errURLUserInfo        = errors.New("url userinfo is rejected")
	errURLEmptyHost       = errors.New("url host is empty")
	errCaptureHintInvalid = errors.New("capture_hint must be absent or none in P0")
	errBodyModeInvalid    = errors.New("body mode must be inline_base64 or omitted in P0")
	errBodyRefInP0        = errors.New("BodyRef body is rejected in P0")
	errTimeoutTooLow      = errors.New("timeout_ms must be at least 1000")
	errTimeoutTooHigh     = errors.New("timeout_ms exceeds maximum")
)

// ValidateRequest parses and validates a RequestEnvelope against config limits.
func ValidateRequest(raw []byte, maxRequestBodyBytes, maxTimeoutMs uint64) (*ValidatedRequest, error) {
	var env RequestEnvelope

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&env); err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}

	if err := validateMethod(env.Method); err != nil {
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

	if err := validateCaptureHint(env.CaptureHint); err != nil {
		return nil, err
	}

	return &ValidatedRequest{
		Method:     env.Method,
		URL:        parsedURL,
		Headers:    headers,
		BodyData:   bodyData,
		TimeoutMs:  timeoutMs,
		Replayable: env.Replayable,
		StickySessionID: func() string {
			if env.Routing != nil {
				return env.Routing.StickySessionID
			}

			return ""
		}(),
	}, nil
}

func validateMethod(method string) error {
	if method == "" {
		return &ValidationError{Code: "invalid_request", Message: "method is required"}
	}

	if method != strings.ToUpper(method) {
		return &ValidationError{Code: "invalid_request", Message: "method must be uppercase"}
	}

	if method == "CONNECT" {
		return &ValidationError{Code: "unsupported_ingress_mode", Message: "CONNECT method is not supported in P0 REST transport"}
	}

	for _, r := range method {
		if !isMethodChar(r) {
			return &ValidationError{Code: "invalid_request", Message: "method contains invalid characters"}
		}
	}

	return nil
}

func isMethodChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') || r == '-'
}

func validateURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, &ValidationError{Code: "invalid_request", Message: "url is required"}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, &ValidationError{Code: "invalid_request", Message: "url is not a valid URL"}
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, &ValidationError{Code: "invalid_request", Message: "url scheme must be http or https"}
	}

	if parsed.Fragment != "" {
		return nil, &ValidationError{Code: "invalid_request", Message: "url fragments are rejected"}
	}

	if parsed.User != nil {
		return nil, &ValidationError{Code: "invalid_request", Message: "url userinfo is rejected"}
	}

	if parsed.Host == "" {
		return nil, &ValidationError{Code: "invalid_request", Message: "url host is empty"}
	}

	if strings.Contains(parsed.Host, "%") {
		return nil, &ValidationError{Code: "invalid_request", Message: "IPv6 zone identifiers are rejected"}
	}

	return parsed, nil
}

func validateHeaders(headers []HeaderPair) ([]HeaderPair, error) {
	if len(headers) > 64 {
		return nil, &ValidationError{Code: "invalid_request", Message: "header count exceeds maximum of 64"}
	}

	var totalBytes int

	for _, h := range headers {
		if len(h.Name) > 64 {
			return nil, &ValidationError{Code: "invalid_request", Message: "header name exceeds maximum length of 64 bytes"}
		}

		if !isValidHTTPToken(h.Name) {
			return nil, &ValidationError{Code: "invalid_request", Message: "header name is not a valid HTTP token"}
		}

		if strings.EqualFold(h.Name, "host") {
			return nil, &ValidationError{Code: "invalid_request", Message: "client-supplied Host header is rejected"}
		}

		if strings.ContainsRune(h.Value, '\r') || strings.ContainsRune(h.Value, '\n') {
			return nil, &ValidationError{Code: "invalid_request", Message: "header values must not contain CR or LF"}
		}

		decoded, err := base64.StdEncoding.DecodeString(h.Value)
		if err != nil {
			return nil, &ValidationError{Code: "invalid_request", Message: "header value is not valid base64"}
		}

		totalBytes += len(h.Name) + len(decoded)

		if totalBytes > 16384 {
			return nil, &ValidationError{Code: "invalid_request", Message: "aggregate header bytes exceed maximum of 16384"}
		}
	}

	return headers, nil
}

func isValidHTTPToken(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		switch {
		case r == '!' || r == '#' || r == '$' || r == '%' || r == '&' || r == '\'' ||
			r == '*' || r == '+' || r == '-' || r == '.' ||
			(r >= '0' && r <= '9') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			r == '^' || r == '_' || r == '`' || r == '|' || r == '~':
			// Valid token chars per RFC 7230
		default:
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
		return nil, &ValidationError{Code: "invalid_request", Message: "body mode must be inline_base64 or omitted in P0"}
	}

	if body.BodyRefID != "" {
		return nil, &ValidationError{Code: "invalid_request", Message: "BodyRef body is rejected in P0"}
	}

	if body.Mode == "" {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(body.DataBase64)
	if err != nil {
		return nil, &ValidationError{Code: "invalid_request", Message: "body data is not valid base64"}
	}

	if uint64(len(decoded)) > maxBytes {
		return nil, &ValidationError{Code: "body_too_large", Message: "request body exceeds limit", Details: map[string]string{
			"direction":   "request",
			"limit_bytes": strconv.FormatUint(maxBytes, 10),
		}}
	}

	return decoded, nil
}

func validateTimeout(timeoutMs, maxTimeoutMs uint64) (uint64, error) {
	if timeoutMs == 0 {
		return 0, nil
	}

	if timeoutMs < 1000 {
		return 0, &ValidationError{Code: "invalid_request", Message: "timeout_ms must be at least 1000"}
	}

	if timeoutMs > maxTimeoutMs {
		return 0, &ValidationError{Code: "invalid_request", Message: "timeout_ms exceeds maximum"}
	}

	return timeoutMs, nil
}

func validateCaptureHint(hint string) error {
	if hint == "" || hint == "none" {
		return nil
	}

	return &ValidationError{Code: "invalid_request", Message: "capture_hint must be absent or none in P0"}
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
	case "invalid_request":
		return http.StatusBadRequest
	case "body_too_large":
		return http.StatusRequestEntityTooLarge
	case "unsupported_ingress_mode":
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
