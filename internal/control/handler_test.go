package control

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
)

const (
	handlerTestInvalidRequestCode = "invalid_request"
	handlerTestUnsupportedCode    = "unsupported_ingress_mode"
	handlerTestBodyTooLargeCode   = "body_too_large"
	handlerTestAuthFailureCode    = "auth_failure"
	handlerTestClientCategory     = "client"
	handlerTestInlineBase64       = "inline_base64"
	handlerTestMessage            = "test"
	handlerTestTenantID           = "ten_test"
	handlerTestUpstreamTrailer    = "X-Upstream-Trailer"
	handlerTestMalformedUTF8      = "malformed utf8"
	testExampleHost               = "example.com"
)

var handlerTestReqURL = func() string {
	u := &url.URL{Scheme: urlSchemeHTTPS, Host: testExampleHost, Path: "/path"}

	return u.String()
}()

func TestHandlerValidRequest(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"GET","url":"` + handlerTestReqURL + `"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp SuccessResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("response status = %d, want 200", resp.Status)
	}
	if resp.RequestID == "" {
		t.Fatal("request_id is empty")
	}
}

func TestStreamHandlerWritesBinaryMetadataBeforeBodyAndTrailers(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	h.SetDispatcher(streamingFakeDispatcher{
		status:   http.StatusPartialContent,
		headers:  []*strawpb.Header{{Name: "Content-Type", Value: []byte("text/plain")}},
		chunks:   [][]byte{[]byte("hello"), []byte(" world")},
		trailers: []*strawpb.Header{{Name: handlerTestUpstreamTrailer, Value: []byte("done")}},
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests:stream", strings.NewReader(`{"method":"GET","url":"https://example.com"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeStreamHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get(headerCanonicalContentType); got != streamContentType {
		t.Fatalf("content type = %q, want %q", got, streamContentType)
	}

	frames := decodeStreamFrames(t, w.Body.Bytes())
	if len(frames) != 5 {
		t.Fatalf("frame count = %d, want 5", len(frames))
	}
	if frames[0].typ != streamFrameMetadata {
		t.Fatalf("first frame type = %d, want metadata", frames[0].typ)
	}

	var metadata streamMetadataPayload
	mustUnmarshalFrame(t, frames[0], &metadata)
	if metadata.RequestID == "" {
		t.Fatal("metadata request_id is empty")
	}
	if metadata.Status != http.StatusPartialContent {
		t.Fatalf("metadata status = %d, want %d", metadata.Status, http.StatusPartialContent)
	}
	if len(metadata.Headers) != 1 || metadata.Headers[0].Name != "Content-Type" {
		t.Fatalf("metadata headers = %#v", metadata.Headers)
	}

	if frames[1].typ != streamFrameBody || string(frames[1].payload) != "hello" {
		t.Fatalf("body frame 1 = (%d, %q), want body hello", frames[1].typ, frames[1].payload)
	}
	if frames[2].typ != streamFrameBody || string(frames[2].payload) != " world" {
		t.Fatalf("body frame 2 = (%d, %q), want body world", frames[2].typ, frames[2].payload)
	}

	var trailers streamTrailersPayload
	if frames[3].typ != streamFrameTrailers {
		t.Fatalf("fourth frame type = %d, want trailers", frames[3].typ)
	}
	mustUnmarshalFrame(t, frames[3], &trailers)
	if len(trailers.Headers) != 1 || trailers.Headers[0].Name != handlerTestUpstreamTrailer {
		t.Fatalf("trailers = %#v", trailers.Headers)
	}

	if frames[4].typ != streamFrameEnd {
		t.Fatalf("last frame type = %d, want end", frames[4].typ)
	}

	var end streamEndPayload
	mustUnmarshalFrame(t, frames[4], &end)
	if end.Timing.TotalMs != 4 {
		t.Fatalf("end timing total_ms = %d, want 4", end.Timing.TotalMs)
	}
}

func TestStreamHandlerWritesErrorFrameAfterPartialBody(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	h.SetDispatcher(streamingFakeDispatcher{
		status:  http.StatusOK,
		chunks:  [][]byte{[]byte("partial")},
		perr:    &PipelineError{Code: UpstreamReset},
		started: true,
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests:stream", strings.NewReader(`{"method":"GET","url":"https://example.com"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeStreamHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	frames := decodeStreamFrames(t, w.Body.Bytes())
	if len(frames) != 3 {
		t.Fatalf("frame count = %d, want 3", len(frames))
	}
	if frames[0].typ != streamFrameMetadata || frames[1].typ != streamFrameBody || frames[2].typ != streamFrameError {
		t.Fatalf("frame types = %d, %d, %d; want metadata, body, error", frames[0].typ, frames[1].typ, frames[2].typ)
	}

	var errResp ErrorResponse
	mustUnmarshalFrame(t, frames[2], &errResp)
	if errResp.Code != errorCodeUpstreamReset {
		t.Fatalf("error code = %q, want %q", errResp.Code, errorCodeUpstreamReset)
	}
}

func TestStreamHandlerDoesNotApplyInlineResponseBodyLimit(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	h.maxResponseBodyBytes = 3
	h.SetDispatcher(streamingFakeDispatcher{status: http.StatusOK, chunks: [][]byte{[]byte("larger than inline")}})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests:stream", strings.NewReader(`{"method":"GET","url":"https://example.com"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeStreamHTTP(w, req)

	frames := decodeStreamFrames(t, w.Body.Bytes())
	if len(frames) != 3 {
		t.Fatalf("frame count = %d, want metadata/body/end", len(frames))
	}
	if frames[1].typ != streamFrameBody || string(frames[1].payload) != "larger than inline" {
		t.Fatalf("body frame = (%d, %q), want streamed body", frames[1].typ, frames[1].payload)
	}
	if frames[2].typ != streamFrameEnd {
		t.Fatalf("last frame type = %d, want end", frames[2].typ)
	}
}

func TestStreamHandlerCaptureHintAllowedByTenantPolicy(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	h.SetPayloadCapturePolicyStore(&staticPayloadCapturePolicyStore{policy: PayloadCapturePolicy{
		TenantID: handlerTestTenantID, Enabled: true, AllowedDecisions: []CaptureDecision{CaptureDecisionHeaders},
	}})
	h.SetDispatcher(streamingFakeDispatcher{status: http.StatusOK})

	payload := `{"method":"GET","url":"https://example.com","capture_hint":"headers"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests:stream", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeStreamHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestStreamHandlerEnforcesRequestBodyLimit(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	largeData := strings.Repeat("A", 1_400_000)
	payload := `{"method":"POST","url":"https://example.com","body":{"mode":"inline_base64","data_base64":"` + largeData + `"}}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests:stream", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeStreamHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestBodyTooLargeCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestBodyTooLargeCode)
	}
}

func TestStreamHandlerAuthAndRBAC(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t)
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests:stream", strings.NewReader(`{"method":"GET","url":"https://example.com"}`))
		w := httptest.NewRecorder()

		h.ServeStreamHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("viewer denied", func(t *testing.T) {
		t.Parallel()

		h, token := newTestHandlerWithRole(t, RoleViewer)
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests:stream", strings.NewReader(`{"method":"GET","url":"https://example.com"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		h.ServeStreamHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
		}
	})
}

func TestStreamHandlerClientCancellationReturnsExistingError(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	h.SetDispatcher(cancelAwareStreamDispatcher{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/requests:stream", strings.NewReader(`{"method":"GET","url":"https://example.com"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeStreamHTTP(w, req)

	if w.Code != statusClientClosedRequest {
		t.Fatalf("status = %d, want %d", w.Code, statusClientClosedRequest)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp.Code != errorCodeCancelled {
		t.Fatalf("code = %q, want %q", errResp.Code, errorCodeCancelled)
	}
}

func TestHandlerMissingMethod(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"url":"https://example.com"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestInvalidRequestCode)
	}
}

func TestHandlerCONNECTRejected(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"CONNECT","url":"https://example.com"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestUnsupportedCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestUnsupportedCode)
	}
}

func TestHandlerURLFragmentRejected(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"GET","url":"https://example.com/path#section"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestInvalidRequestCode)
	}
}

func TestHandlerURLUserInfoRejected(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	u := &url.URL{Scheme: urlSchemeHTTPS, Host: testExampleHost, Path: "/path"}
	u.User = url.UserPassword("user", "pass")
	payload := `{"method":"GET","url":"` + u.String() + `"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestInvalidRequestCode)
	}
}

func TestHandlerHostHeaderRejected(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"GET","url":"https://example.com","headers":[{"name":"Host","value_base64":"ZXhhbXBsZS5jb20="}]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestInvalidRequestCode)
	}
}

func TestHandlerDuplicateHeaders(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	// "hello" in base64
	payload := `{"method":"GET","url":"https://example.com","headers":[{"name":"X-Custom","value_base64":"aGVsbG8="},{"name":"X-Custom","value_base64":"d29ybGQ="}]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandlerBodyLimitExceeded(t *testing.T) {
	t.Parallel()

	// 1 MB limit
	h, token := newTestHandler(t)

	// 2 MB of base64 data (which decodes to ~1.5 MB)
	largeData := strings.Repeat("A", 1_400_000)
	payload := `{"method":"POST","url":"https://example.com","body":{"mode":"inline_base64","data_base64":"` + largeData + `"}}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestBodyTooLargeCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestBodyTooLargeCode)
	}
}

func TestHandlerCaptureHintRejected(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"GET","url":"https://example.com","capture_hint":"full"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", errResp.Code, handlerTestInvalidRequestCode)
	}
}

func TestHandlerNonPOSTMethod(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/requests", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlerValidRequestWithHeadersAndBody(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	// "application/json" in base64
	payload := `{"method":"POST","url":"https://example.com/api","headers":[{"name":"Content-Type","value_base64":"YXBwbGljYXRpb24vanNvbg=="}],"body":{"mode":"inline_base64","data_base64":"aGVsbG8="}}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp SuccessResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("response status = %d, want 200", resp.Status)
	}
}

func TestHandlerTimeoutTooLow(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"GET","url":"https://example.com","timeout_ms":500}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlerUnknownFieldsRejected(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"GET","url":"https://example.com","unknown_field":true}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlerInvalidMethodCasing(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"get","url":"https://example.com"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestValidateRequestUnknownMethodRejected(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"BREW","url":"https://example.com"}`)
	_, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for unknown HTTP method")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", verr.Code, handlerTestInvalidRequestCode)
	}
}

func TestErrorRegistryComplete(t *testing.T) {
	t.Parallel()

	// Verify all expected error codes from the canonical registry are present.
	expectedCodes := []ErrorCode{
		1,   // auth_failure
		2,   // tenant_not_found
		3,   // insufficient_permissions
		4,   // rate_limit_exceeded
		5,   // quota_exhausted
		6,   // invalid_request
		7,   // destination_denied
		100, // route_no_match
		101, // route_unavailable
		200, // assignment_timeout
		201, // worker_disconnected
		202, // transport_unavailable
		300, // upstream_dns_failure
		301, // upstream_tls_failure
		403, // body_too_large
		500, // control_internal_error
	}

	for _, code := range expectedCodes {
		entry, ok := ErrorRegistry[code]
		if !ok {
			t.Fatalf("missing error code %d in registry", code)
		}
		if entry.Category == "" {
			t.Errorf("error code %d has empty category", code)
		}
		if entry.Code == "" {
			t.Errorf("error code %d has empty code name", code)
		}
		if entry.HTTPStatus < 100 || entry.HTTPStatus > 599 {
			t.Errorf("error code %d has invalid HTTP status %d", code, entry.HTTPStatus)
		}
	}
}

func TestErrorResponseFromCode(t *testing.T) {
	t.Parallel()

	resp := ErrorResponseFromCode(1, "req_abc", nil)
	if resp.Code != handlerTestAuthFailureCode {
		t.Fatalf("code = %q, want %q", resp.Code, handlerTestAuthFailureCode)
	}
	if resp.Category != handlerTestClientCategory {
		t.Fatalf("category = %q, want %q", resp.Category, handlerTestClientCategory)
	}
	if resp.Retryable {
		t.Fatal("auth_failure should not be retryable")
	}
	if resp.RequestID != "req_abc" {
		t.Fatalf("request_id = %q, want %q", resp.RequestID, "req_abc")
	}
}

func TestValidateRequestEmptyMethod(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"url":"https://example.com"}`)
	var err error
	_, err = ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for empty method")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", verr.Code, handlerTestInvalidRequestCode)
	}
}

func TestValidateRequestInvalidURLScheme(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"ftp://example.com"}`)
	var err error
	_, err = ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for invalid URL scheme")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestValidateRequestBodyRefRejected(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"POST","url":"https://example.com","body":{"mode":"body_ref","body_ref_id":"ref_123"}}`)
	var err error
	_, err = ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for BodyRef body")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", verr.Code, handlerTestInvalidRequestCode)
	}
}

func TestValidateRequestCaptureHintOtherThanNone(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com","capture_hint":"full"}`)
	var err error
	_, err = ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for non-none capture_hint")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestValidateRequestCaptureHintAllowedByPolicy(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com","capture_hint":"headers"}`)
	_, err := ValidateRequestWithCapturePolicy(raw, 1_048_576, 120_000, PayloadCapturePolicy{
		TenantID: adminTestTenantA, Enabled: true, AllowedDecisions: []CaptureDecision{CaptureDecisionHeaders},
	})
	if err != nil {
		t.Fatalf("ValidateRequestWithCapturePolicy() error = %v", err)
	}
}

func TestHandlerCaptureHintAllowedByTenantPolicy(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	h.SetPayloadCapturePolicyStore(&staticPayloadCapturePolicyStore{policy: PayloadCapturePolicy{
		TenantID: handlerTestTenantID, Enabled: true, AllowedDecisions: []CaptureDecision{CaptureDecisionHeaders},
	}})

	payload := `{"method":"GET","url":"https://example.com","capture_hint":"headers"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandlerPayloadCaptureStoresSuccessfulRESTResponse(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	h.SetPayloadCapturePolicyStore(&staticPayloadCapturePolicyStore{policy: PayloadCapturePolicy{
		TenantID: handlerTestTenantID, Enabled: true, AllowedDecisions: []CaptureDecision{CaptureDecisionBodyFull},
	}})

	bodies := &fakeCaptureBodyStore{keyReq: captureTestReqKey, keyResp: captureTestRespKey}
	rec := &recordingCaptureRecorder{}
	h.SetPayloadCaptureStore(NewPayloadCaptureStore(bodies, rec))
	h.SetDispatcher(bodyCaptureDispatcher{})

	payload := `{"method":"POST","url":"https://example.com","headers":[{"name":"Authorization","value_base64":"c2VjcmV0"}],"body":{"mode":"inline_base64","data_base64":"cmVxdWVzdA=="},"capture_hint":"body_full"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d capture events, want 1", len(rec.events))
	}

	ev := rec.events[0]
	if ev.TenantID != handlerTestTenantID || ev.CaptureScope != string(ScopeTenant) || ev.CaptureDecision != string(CaptureDecisionBodyFull) {
		t.Fatalf("capture identity/scope/decision = %+v", ev)
	}

	if ev.RequestBodyRef != captureTestReqKey || ev.ResponseBodyRef != captureTestRespKey {
		t.Fatalf("capture body refs = %q/%q, want %s/%s", ev.RequestBodyRef, ev.ResponseBodyRef, captureTestReqKey, captureTestRespKey)
	}

	if strings.Contains(ev.RequestHeaders, "secret") || !strings.Contains(ev.RequestHeaders, requestMetadataRedacted) {
		t.Fatalf("request headers not redacted: %s", ev.RequestHeaders)
	}
}

func TestValidateRequestValidBase64Headers(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com","headers":[{"name":"X-Custom","value_base64":"aGVsbG8="}]}`)
	vr, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if len(vr.Headers) != 1 {
		t.Fatalf("headers count = %d, want 1", len(vr.Headers))
	}
	if vr.Headers[0].Name != "X-Custom" {
		t.Fatalf("header name = %q, want %q", vr.Headers[0].Name, "X-Custom")
	}
}

func TestValidateRequestBodyDataDecoded(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"POST","url":"https://example.com","body":{"mode":"inline_base64","data_base64":"aGVsbG8="}}`)
	vr, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if string(vr.BodyData) != "hello" {
		t.Fatalf("body data = %q, want %q", vr.BodyData, "hello")
	}
}

func TestValidateRequestNoBody(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com"}`)
	vr, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if vr.BodyData != nil {
		t.Fatalf("body data = %q, want nil", vr.BodyData)
	}
}

func TestValidateRequestTimeoutDefault(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com"}`)
	vr, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if vr.TimeoutMs != 0 {
		t.Fatalf("timeout_ms = %d, want 0 (default)", vr.TimeoutMs)
	}
}

func TestValidateRequestTimeoutWithinLimit(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com","timeout_ms":30000}`)
	vr, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if vr.TimeoutMs != 30000 {
		t.Fatalf("timeout_ms = %d, want 30000", vr.TimeoutMs)
	}
}

func TestHandlerRejectsTimeoutAboveTenantMax(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)
	store := NewInMemorySnapshotStore()
	_, err := store.SaveTenantSnapshot(context.Background(), config.TenantSnapshot{
		TenantID:             "ten_test",
		ConfigVersion:        1,
		DefaultTimeoutMs:     2000,
		MaxTimeoutMs:         2500,
		MetadataQueryStorage: "drop",
		MetadataPathStorage:  "hash",
	}, 0)
	if err != nil {
		t.Fatalf("SaveTenantSnapshot() error = %v", err)
	}
	h.SetConfigCache(NewConfigCache(store, nil))

	payload := `{"method":"GET","url":"https://example.com","timeout_ms":3000}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlerSuccessEnvelopeStructure(t *testing.T) {
	t.Parallel()

	h, token := newTestHandler(t)

	payload := `{"method":"GET","url":"https://example.com"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp SuccessResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Verify envelope structure
	if resp.RequestID == "" {
		t.Fatal("request_id is empty")
	}
	if resp.Status != 200 {
		t.Fatalf("envelope status = %d, want 200", resp.Status)
	}
	if resp.Body.Mode != handlerTestInlineBase64 {
		t.Fatalf("body mode = %q, want %q", resp.Body.Mode, handlerTestInlineBase64)
	}
	if resp.Body.Truncated {
		t.Fatal("truncated should be false")
	}
	if resp.Timing.TotalMs < 0 {
		t.Fatalf("total_ms = %d, want >= 0", resp.Timing.TotalMs)
	}
}

func TestValidateRequestIPv6ZoneRejected(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://[::1%eth0]/path"}`)
	var err error
	_, err = ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for IPv6 zone identifier")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Code != handlerTestInvalidRequestCode {
		t.Fatalf("code = %q, want %q", verr.Code, handlerTestInvalidRequestCode)
	}
}

func TestValidateRequestEmptyHostRejected(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https:///path"}`)
	var err error
	_, err = ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestValidateRequestHeaderCountLimit(t *testing.T) {
	t.Parallel()

	// Build a request with 65 headers
	var headers []map[string]string
	for i := range 65 {
		headers = append(headers, map[string]string{
			"name":         fmt.Sprintf("X-Header-%d", i),
			"value_base64": "AA==",
		})
	}
	body := map[string]any{
		"method":  "GET",
		"url":     "https://example.com",
		"headers": headers,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	_, err = ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for header count exceeding 64")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestValidateRequestHeaderNameTooLong(t *testing.T) {
	t.Parallel()

	longName := strings.Repeat("a", 65)
	raw := fmt.Appendf(nil, `{"method":"GET","url":"https://example.com","headers":[{"name":"%s","value_base64":"AA=="}]}`, longName)
	_, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for header name exceeding 64 bytes")
	}
}

func TestValidateRequestInvalidHeaderName(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com","headers":[{"name":"X-Invalid Header!","value_base64":"AA=="}]}`)
	_, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for invalid header name")
	}
}

func TestValidateRequestCRInHeaderValue(t *testing.T) {
	t.Parallel()

	// "YQ0KYg==" is base64 for "a\r\nb": valid base64, but the decoded bytes
	// contain a bare CR/LF and must be rejected at ingress.
	raw := []byte(`{"method":"GET","url":"https://example.com","headers":[{"name":"X-CR","value_base64":"YQ0KYg=="}]}`)
	_, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for CR/LF in decoded header value")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Code != errorCodeInvalidRequest {
		t.Fatalf("expected code %q, got %q", errorCodeInvalidRequest, verr.Code)
	}
}

func TestValidateRequestTimeoutExceedsMax(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"method":"GET","url":"https://example.com","timeout_ms":130000}`)
	_, err := ValidateRequest(raw, 1_048_576, 120_000)
	if err == nil {
		t.Fatal("expected error for timeout exceeding max")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestHTTPValidationErrorStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected int
	}{
		{handlerTestInvalidRequestCode, http.StatusBadRequest},
		{handlerTestBodyTooLargeCode, http.StatusRequestEntityTooLarge},
		{handlerTestUnsupportedCode, http.StatusBadRequest},
		{handlerTestAuthFailureCode, http.StatusUnauthorized},
		{"insufficient_permissions", http.StatusForbidden},
		{"destination_denied", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			verr := &ValidationError{Code: tt.code, Message: handlerTestMessage}
			if got := verr.HTTPStatus(); got != tt.expected {
				t.Errorf("HTTPStatus(%q) = %d, want %d", tt.code, got, tt.expected)
			}
		})
	}
}

// newTestHandler builds a RequestHandler wired to an Authenticator seeded
// with one active tenant-scoped "requester" key, and returns the plaintext
// bearer token for that key. Handler tests that only exercise request
// validation (not auth/RBAC itself, which is covered in auth_test.go and
// admin_handlers_test.go) use this helper to authenticate as a caller who
// is always allowed to execute data-plane requests.
func newTestHandler(t *testing.T) (*RequestHandler, string) {
	t.Helper()

	return newTestHandlerWithRole(t, RoleRequester)
}

func newTestHandlerWithRole(t *testing.T, role Role) (*RequestHandler, string) {
	t.Helper()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("test-pepper")

	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	record := APIKeyRecord{
		ID:         "key_test_requester",
		ScopeType:  ScopeTenant,
		TenantID:   handlerTestTenantID,
		Role:       role,
		Prefix:     generated.Prefix,
		SecretHash: HashAPIKeySecret(generated.Secret, pepper),
		Status:     APIKeyStatusActive,
		CreatedAt:  time.Now().UTC(),
	}
	err = store.Create(context.Background(), record)
	if err != nil {
		t.Fatalf("store.Create() error = %v", err)
	}

	authenticator := NewAuthenticator(store, pepper)
	h := NewRequestHandler(1_048_576, 1_048_576, 120_000, authenticator)
	h.SetDispatcher(fakeRequestDispatcher{})

	return h, generated.Secret
}

type fakeRequestDispatcher struct{}

func (fakeRequestDispatcher) Dispatch(_ context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	return SuccessResponse{
		RequestID: in.RequestID,
		Status:    http.StatusOK,
		Body: ResponseBody{
			Mode:      handlerTestInlineBase64,
			Truncated: false,
		},
	}, nil
}

type bodyCaptureDispatcher struct{}

func (bodyCaptureDispatcher) Dispatch(_ context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	return SuccessResponse{
		RequestID: in.RequestID,
		Status:    http.StatusOK,
		Headers: []HeaderPair{{
			Name:  testSetCookieHeader,
			Value: base64.StdEncoding.EncodeToString([]byte("secret-cookie")),
		}},
		Body: ResponseBody{
			Mode:       handlerTestInlineBase64,
			DataBase64: base64.StdEncoding.EncodeToString([]byte("response")),
		},
	}, nil
}

type unsupportedFingerprintDispatcher struct{}

func (unsupportedFingerprintDispatcher) Dispatch(_ context.Context, _ DispatchInput) (SuccessResponse, *PipelineError) {
	return SuccessResponse{}, &PipelineError{Code: UnsupportedFingerprint}
}

type countingValidationDispatcher struct {
	calls int
}

func (d *countingValidationDispatcher) Dispatch(_ context.Context, _ DispatchInput) (SuccessResponse, *PipelineError) {
	d.calls++

	return SuccessResponse{}, nil
}

func (d *countingValidationDispatcher) DispatchRaw(_ context.Context, _ DispatchInput, _ http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	d.calls++

	return SuccessResponse{}, nil, false
}

func TestUnsupportedFingerprintValidationRecordsOneEventWithoutDispatch(t *testing.T) {
	t.Parallel()

	malformed := append([]byte(`{"method":"GET","url":"https://example.com/","fingerprint_profile":"`), 0xff)
	malformed = append(malformed, []byte(`","timeout_ms":5000}`)...)

	tests := []struct {
		name          string
		body          []byte
		wantRequested string
	}{
		{name: "literal baseline", body: []byte(`{"method":"GET","url":"https://example.com/","fingerprint_profile":"baseline","timeout_ms":5000}`), wantRequested: baselineFingerprintEvidence},
		{name: handlerTestMalformedUTF8, body: malformed, wantRequested: projectFingerprintEvidence(string([]byte{0xff}))},
	}

	for _, tt := range tests {
		for _, endpoint := range []struct {
			name  string
			serve func(*RequestHandler, http.ResponseWriter, *http.Request)
		}{
			{name: "rest", serve: func(h *RequestHandler, w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }},
			{name: "stream", serve: func(h *RequestHandler, w http.ResponseWriter, r *http.Request) { h.ServeStreamHTTP(w, r) }},
		} {
			t.Run(tt.name+"/"+endpoint.name, func(t *testing.T) {
				recorder := &captureRequestMetadataRecorder{}
				dispatcher := &countingValidationDispatcher{}
				h, token := newTestHandler(t)
				h.metadataWriter = recorder
				h.SetDispatcher(dispatcher)

				req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(string(tt.body)))
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()

				endpoint.serve(h, w, req)

				if dispatcher.calls != 0 {
					t.Fatalf("dispatch calls = %d, want zero", dispatcher.calls)
				}
				var response ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				if err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}
				if response.Code != errorCodeUnsupportedFingerprint {
					t.Fatalf("response code = %q, want unsupported_fingerprint", response.Code)
				}
				if len(recorder.events) != 1 {
					t.Fatalf("recorded events = %d, want exactly 1", len(recorder.events))
				}
				event := recorder.events[0]
				if event.RequestID != response.RequestID {
					t.Fatalf("event request_id = %q, response request_id = %q", event.RequestID, response.RequestID)
				}
				if event.RequestedFingerprintProfile != tt.wantRequested {
					t.Fatalf("requested profile = %q, want %q", event.RequestedFingerprintProfile, tt.wantRequested)
				}
				if event.SelectedFingerprintProfile != "" || event.ExecutedFingerprintProfile != "" {
					t.Fatalf("selected/executed profiles = %q/%q, want empty/empty", event.SelectedFingerprintProfile, event.ExecutedFingerprintProfile)
				}
				if event.ErrorCode != errorCodeUnsupportedFingerprint {
					t.Fatalf("event error code = %q, want unsupported_fingerprint", event.ErrorCode)
				}
			})
		}
	}
}

func TestHandlerUnsupportedFingerprintRecordsSingleCorrelatedEvent(t *testing.T) {
	t.Parallel()

	recorder := &captureRequestMetadataRecorder{}
	h, token := newTestHandler(t)
	h.metadataWriter = recorder
	h.SetDispatcher(unsupportedFingerprintDispatcher{})

	payload := `{"method":"GET","url":"` + handlerTestReqURL + `","fingerprint_profile":"` + testFutureFingerprint + `"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	if len(recorder.events) != 1 {
		t.Fatalf("recorded events = %d, want exactly 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.RequestedFingerprintProfile != testFutureFingerprint {
		t.Fatalf("requested profile = %q, want %s", event.RequestedFingerprintProfile, testFutureFingerprint)
	}
	if event.SelectedFingerprintProfile != "" {
		t.Fatalf("selected profile = %q, want empty for control-local rejection", event.SelectedFingerprintProfile)
	}
	if event.ExecutedFingerprintProfile != "" {
		t.Fatalf("executed profile = %q, want empty for control-local rejection", event.ExecutedFingerprintProfile)
	}
	if event.ErrorCode != ErrorRegistry[UnsupportedFingerprint].Code {
		t.Fatalf("error code = %q, want %q", event.ErrorCode, ErrorRegistry[UnsupportedFingerprint].Code)
	}
}

type staticPayloadCapturePolicyStore struct {
	policy PayloadCapturePolicy
}

func (s *staticPayloadCapturePolicyStore) Get(_ context.Context, _ string) (PayloadCapturePolicy, error) {
	return s.policy, nil
}

func (s *staticPayloadCapturePolicyStore) Put(context.Context, PayloadCapturePolicy, uint64) (PayloadCapturePolicy, error) {
	return PayloadCapturePolicy{}, nil
}

type streamFrame struct {
	typ     byte
	payload []byte
}

func decodeStreamFrames(t *testing.T, raw []byte) []streamFrame {
	t.Helper()

	var frames []streamFrame
	for len(raw) > 0 {
		if len(raw) < 5 {
			t.Fatalf("short stream frame header: %d bytes", len(raw))
		}

		frameType := raw[0]
		n := int(binary.BigEndian.Uint32(raw[1:5]))
		raw = raw[5:]
		if len(raw) < n {
			t.Fatalf("short stream frame payload: got %d bytes, want %d", len(raw), n)
		}

		payload := append([]byte(nil), raw[:n]...)
		frames = append(frames, streamFrame{typ: frameType, payload: payload})
		raw = raw[n:]
	}

	return frames
}

func mustUnmarshalFrame(t *testing.T, frame streamFrame, out any) {
	t.Helper()

	err := json.Unmarshal(frame.payload, out)
	if err != nil {
		t.Fatalf("unmarshal frame type %d: %v", frame.typ, err)
	}
}

type streamingFakeDispatcher struct {
	status   int
	headers  []*strawpb.Header
	chunks   [][]byte
	trailers []*strawpb.Header
	perr     *PipelineError
	started  bool
}

func (d streamingFakeDispatcher) Dispatch(_ context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	return SuccessResponse{RequestID: in.RequestID, Status: d.status}, d.perr
}

func (d streamingFakeDispatcher) DispatchRaw(_ context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	status := d.status
	if status == 0 {
		status = http.StatusOK
	}

	writeRawResponseStart(w, uint32(status), d.headers)
	for _, chunk := range d.chunks {
		_, _ = w.Write(chunk)
	}
	if len(d.trailers) > 0 {
		writeRawTrailers(w, d.trailers)
	}

	return SuccessResponse{
		RequestID: in.RequestID,
		Status:    status,
		Timing: RequestTiming{
			RoutingMs:    1,
			AssignmentMs: 2,
			EgressMs:     3,
			TotalMs:      4,
		},
		ResponseSizeBytes: totalChunkLen(d.chunks),
	}, d.perr, d.started || d.perr == nil || len(d.chunks) > 0 || len(d.headers) > 0
}

func totalChunkLen(chunks [][]byte) uint64 {
	var n uint64
	for _, chunk := range chunks {
		n += uint64FromInt(len(chunk))
	}

	return n
}

type cancelAwareStreamDispatcher struct{}

func (cancelAwareStreamDispatcher) Dispatch(ctx context.Context, _ DispatchInput) (SuccessResponse, *PipelineError) {
	<-ctx.Done()

	return SuccessResponse{}, &PipelineError{Code: Cancelled}
}

func (cancelAwareStreamDispatcher) DispatchRaw(ctx context.Context, _ DispatchInput, _ http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	<-ctx.Done()

	return SuccessResponse{}, &PipelineError{Code: Cancelled}, false
}
