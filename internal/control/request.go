package control

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
)

const (
	urlSchemeHTTP               = "http"
	urlSchemeHTTPS              = "https"
	bodyModeInlineBase64        = "inline_base64"
	bodyModeReceipt             = "receipt"
	maxFingerprintEvidenceBytes = 64
)

func projectFingerprintEvidence(value string) string {
	if len(value) > maxFingerprintEvidenceBytes {
		return ""
	}

	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}

		return ""
	}

	return value
}

// Ingress types used in routing and telemetry.
const (
	IngressTypeREST      = "rest"
	IngressTypeHTTPProxy = "http_proxy"
	IngressTypeConnect   = "connect"
	IngressTypeMITM      = "mitm"
)

const (
	headerNameAuthorization      = "authorization"
	headerNameProxyAuthorization = "proxy-authorization"
	headerNameContentLength      = "content-length"
	headerNameTransferEncoding   = "transfer-encoding"
	headerNameConnection         = "connection"
	headerCanonicalContentType   = "Content-Type"
	headerCanonicalContentLength = "Content-Length"
	headerCanonicalConnection    = "Connection"
	mediaTypeTextPlain           = "text/plain"
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

var idnaLookup = idna.New(idna.MapForLookup(), idna.VerifyDNSLength(true))

const (
	maxRequestHeaderCount = 64
	maxRequestHeaderBytes = 16384
	minRequestTimeoutMs   = 1000
)

// RequestEnvelope is the JSON shape for POST /api/v1/requests.
type RequestEnvelope struct {
	Method           string       `json:"method"`
	URL              string       `json:"url"`
	Headers          []HeaderPair `json:"headers,omitempty"`
	Body             *RequestBody `json:"body,omitempty"`
	FingerprintProto string       `json:"fingerprint_profile,omitempty"`
	TimeoutMs        uint64       `json:"timeout_ms,omitempty"`
	Replayable       bool         `json:"replayable"`
	ResponseBodyMode string       `json:"response_body_mode,omitempty"`
}

// HeaderPair preserves header order and duplicates.
type HeaderPair struct {
	Name  string `json:"name"`
	Value string `json:"value_base64"`
}

// RequestBody carries an inline body or a previously verified receipt.
type RequestBody struct {
	Mode       string `json:"mode"`
	DataBase64 string `json:"data_base64,omitempty"`
	ReceiptID  string `json:"receipt_id,omitempty"`
}

// RoutingHints is the internal routing input used by the static deployment.
type RoutingHints struct {
	Tags            []string `json:"tags,omitempty"`
	Country         string   `json:"country,omitempty"`
	Region          string   `json:"region,omitempty"`
	IPType          string   `json:"ip_type,omitempty"`
	StickySessionID string   `json:"sticky_session_id,omitempty"`
}

// ValidatedRequest is the internal representation after validation.
type ValidatedRequest struct {
	Method           string
	URL              *url.URL
	Headers          []HeaderPair
	BodyData         []byte
	BodyReader       io.ReadCloser
	BodySizeBytes    int64
	BodyReceiptID    string
	BodyRef          *strawpb.BodyRefFrame
	Routing          RoutingHints
	IngressType      string
	Fingerprint      string
	TimeoutMs        uint64
	Replayable       bool
	StickySessionID  string
	CaptureDecision  string
	ResponseBodyMode string
}

// ValidateRequest parses and validates a RequestEnvelope against config limits.
func ValidateRequest(raw []byte, maxRequestBodyBytes, maxTimeoutMs uint64) (*ValidatedRequest, error) {
	profileErr := validateFingerprintProfileJSON(raw)
	if profileErr != nil {
		return nil, profileErr
	}

	var env RequestEnvelope

	dec := json.NewDecoder(bytes.NewReader(raw))

	dec.DisallowUnknownFields()

	err := dec.Decode(&env)
	if err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}

	parsedURL, headers, bodyData, bodyReceiptID, timeoutMs, err := validateRequestComponents(env, maxRequestBodyBytes, maxTimeoutMs)
	if err != nil {
		return nil, err
	}

	profileValueErr := validateFingerprintProfileValue(env.FingerprintProto)
	if profileValueErr != nil {
		return nil, profileValueErr
	}

	routing := RoutingHints{}

	return &ValidatedRequest{
		Method:           env.Method,
		URL:              parsedURL,
		Headers:          headers,
		BodyData:         bodyData,
		BodyReceiptID:    bodyReceiptID,
		Routing:          routing,
		IngressType:      IngressTypeREST,
		Fingerprint:      env.FingerprintProto,
		TimeoutMs:        timeoutMs,
		Replayable:       env.Replayable,
		StickySessionID:  routing.StickySessionID,
		CaptureDecision:  "none",
		ResponseBodyMode: env.ResponseBodyMode,
	}, nil
}

// validateFingerprintProfileJSON applies the profile-specific JSON rules that
// encoding/json does not enforce by default. In particular, it rejects
// duplicate members (including case variants accepted by encoding/json) and
// non-string values before the decoder can silently keep the last value or
// turn null into an empty string.
func validateFingerprintProfileJSON(raw []byte) *ValidationError {
	if !utf8.Valid(raw) {
		return invalidUTF8RequestError(raw)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))

	token, err := dec.Token()
	if err != nil {
		return invalidRequestJSONError()
	}

	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil
	}

	seen := false

	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return invalidRequestJSONError()
		}

		key, ok := keyToken.(string)
		if !ok {
			return invalidRequestJSONError()
		}

		var value json.RawMessage

		valueErr := dec.Decode(&value)
		if valueErr != nil {
			return invalidRequestJSONError()
		}

		memberErr := validateFingerprintProfileJSONMember(key, value, &seen)
		if memberErr != nil {
			return memberErr
		}
	}

	return nil
}

func validateFingerprintProfileJSONMember(key string, value json.RawMessage, seen *bool) *ValidationError {
	if !strings.EqualFold(key, "fingerprint_profile") {
		return nil
	}

	if *seen {
		return &ValidationError{Code: errorCodeInvalidRequest, Message: "fingerprint_profile must not be repeated"}
	}

	*seen = true

	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return &ValidationError{Code: errorCodeInvalidRequest, Message: "fingerprint_profile must be a string"}
	}

	return nil
}

func invalidRequestJSONError() *ValidationError {
	return &ValidationError{Code: errorCodeInvalidRequest, Message: "invalid request JSON"}
}

func invalidUTF8RequestError(raw []byte) *ValidationError {
	if evidence := malformedFingerprintProfileEvidence(raw); evidence != "" {
		return unsupportedFingerprintValidationError(evidence)
	}

	return &ValidationError{Code: errorCodeInvalidRequest, Message: "fingerprint_profile must contain valid UTF-8"}
}

func malformedFingerprintProfileEvidence(raw []byte) string {
	const key = `"fingerprint_profile"`

	_, value, found := bytes.Cut(raw, []byte(key))
	if !found {
		return ""
	}

	_, value, found = bytes.Cut(value, []byte{':'})
	if !found {
		return ""
	}

	value = bytes.TrimSpace(value)
	if len(value) == 0 || value[0] != '"' {
		return ""
	}

	value = fingerprintJSONValue(value[1:])
	if utf8.Valid(value) {
		return ""
	}

	return projectFingerprintEvidence(string(value))
}

func fingerprintJSONValue(value []byte) []byte {
	escaped := false

	for i, b := range value {
		if b == '"' && !escaped {
			return value[:i]
		}

		if b == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}

	return value
}

func validateFingerprintProfileValue(value string) *ValidationError {
	if !utf8.ValidString(value) {
		return unsupportedFingerprintValidationError(projectFingerprintEvidence(value))
	}

	if value == baselineFingerprintEvidence {
		return unsupportedFingerprintValidationError(projectFingerprintEvidence(value))
	}

	return nil
}

func unsupportedFingerprintValidationError(evidence string) *ValidationError {
	return &ValidationError{
		Code:                        errorCodeUnsupportedFingerprint,
		Message:                     ErrorRegistry[UnsupportedFingerprint].Message,
		RequestedFingerprintProfile: evidence,
	}
}

func validateRequestComponents(env RequestEnvelope, maxRequestBodyBytes, maxTimeoutMs uint64) (*url.URL, []HeaderPair, []byte, string, uint64, error) {
	err := validateMethod(env.Method)
	if err != nil {
		return nil, nil, nil, "", 0, err
	}

	parsedURL, err := validateURL(env.URL)
	if err != nil {
		return nil, nil, nil, "", 0, err
	}

	headers, err := validateHeaders(env.Headers)
	if err != nil {
		return nil, nil, nil, "", 0, err
	}

	bodyData, bodyReceiptID, err := validateBody(env.Body, maxRequestBodyBytes)
	if err != nil {
		return nil, nil, nil, "", 0, err
	}

	if env.ResponseBodyMode != "" && env.ResponseBodyMode != bodyModeInlineBase64 && env.ResponseBodyMode != bodyModeReceipt {
		return nil, nil, nil, "", 0, &ValidationError{Code: errorCodeInvalidRequest, Message: "response_body_mode must be inline_base64, receipt, or omitted"}
	}

	timeoutMs, err := validateTimeout(env.TimeoutMs, maxTimeoutMs)
	if err != nil {
		return nil, nil, nil, "", 0, err
	}

	return parsedURL, headers, bodyData, bodyReceiptID, timeoutMs, nil
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

	err = normalizeParsedURLHost(parsed)
	if err != nil {
		return nil, err
	}

	return parsed, nil
}

func normalizeParsedURLHost(parsed *url.URL) error {
	host, err := normalizeHostname(parsed.Hostname())
	if err != nil {
		return &ValidationError{Code: errorCodeInvalidRequest, Message: "url host is not a valid hostname"}
	}

	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else {
		parsed.Host = host
	}

	return nil
}

func normalizeHostname(host string) (string, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return "", &ValidationError{Code: errorCodeInvalidRequest, Message: "url host is empty"}
	}

	_, err := netip.ParseAddr(host)
	if err == nil {
		return host, nil
	}

	ascii, err := idnaLookup.ToASCII(host)
	if err != nil {
		return "", fmt.Errorf("normalize hostname %q: %w", host, err)
	}

	return strings.ToLower(ascii), nil
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

func validateBody(body *RequestBody, maxBytes uint64) ([]byte, string, error) {
	if body == nil {
		return nil, "", nil
	}

	switch body.Mode {
	case "":
		if body.ReceiptID != "" {
			return nil, "", &ValidationError{Code: errorCodeInvalidRequest, Message: "receipt_id requires receipt body mode"}
		}

		return nil, "", nil
	case bodyModeReceipt:
		return validateReceiptBody(body)
	case bodyModeInlineBase64:
		return validateInlineBody(body, maxBytes)
	default:
		return nil, "", &ValidationError{Code: errorCodeInvalidRequest, Message: "body mode must be inline_base64, receipt, or omitted"}
	}
}

func validateReceiptBody(body *RequestBody) ([]byte, string, error) {
	if body.ReceiptID == "" || body.DataBase64 != "" {
		return nil, "", &ValidationError{Code: errorCodeInvalidRequest, Message: "receipt body requires receipt_id and no inline data"}
	}

	return nil, body.ReceiptID, nil
}

func validateInlineBody(body *RequestBody, maxBytes uint64) ([]byte, string, error) {
	if body.ReceiptID != "" {
		return nil, "", &ValidationError{Code: errorCodeInvalidRequest, Message: "receipt_id requires receipt body mode"}
	}

	decoded, err := base64.StdEncoding.DecodeString(body.DataBase64)
	if err != nil {
		return nil, "", &ValidationError{Code: errorCodeInvalidRequest, Message: "body data is not valid base64"}
	}

	if uint64(len(decoded)) > maxBytes {
		return nil, "", &ValidationError{Code: errorCodeBodyTooLarge, Message: requestBodyExceedsLimit, Details: map[string]string{
			errorDetailDirectionKey:  "request",
			errorDetailLimitBytesKey: strconv.FormatUint(maxBytes, 10),
		}}
	}

	return decoded, "", nil
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

// ValidationError is a structured validation error.
type ValidationError struct {
	Code                        string            `json:"code"`
	Message                     string            `json:"message"`
	Details                     map[string]string `json:"details,omitempty"`
	RequestedFingerprintProfile string            `json:"-"`
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
