package control

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	strawpb "github.com/beremaran/straw-oss/v2/api/proto/straw/v1"
)

// TestErrorRegistryRows pins the canonical category/HTTP/retryable mapping for
// every row in docs/public/architecture.md REST uses 404 for
// route_no_match and 499 for cancelled.
func TestErrorRegistryRows(t *testing.T) {
	rows := []struct {
		code       ErrorCode
		name       string
		category   string
		httpStatus int
		retryable  bool
	}{
		{AuthFailure, "auth_failure", errorCategoryClient, http.StatusUnauthorized, false},
		{InvalidRequest, "invalid_request", errorCategoryClient, http.StatusBadRequest, false},
		{DestinationDenied, errorCodeDestinationDenied, errorCategoryClient, http.StatusForbidden, false},
		{HeaderInjectionFailed, "header_injection_failed", errorCategoryClient, http.StatusBadRequest, false},
		{UnsupportedIngressMode, "unsupported_ingress_mode", errorCategoryClient, http.StatusBadRequest, false},
		{RouteNoMatch, "route_no_match", errorCategoryRouting, http.StatusNotFound, false},
		{RouteUnavailable, "route_unavailable", errorCategoryRouting, http.StatusServiceUnavailable, true},
		{StickySessionUnavailable, "sticky_session_unavailable", errorCategoryRouting, http.StatusServiceUnavailable, false},
		{ExecutorCapacityExhausted, "executor_capacity_exhausted", errorCategoryRouting, http.StatusServiceUnavailable, true},
		{AssignmentTimeout, "assignment_timeout", errorCategoryTransport, http.StatusGatewayTimeout, true},
		{WorkerDisconnected, "worker_disconnected", errorCategoryTransport, http.StatusBadGateway, true},
		{TransportUnavailable, "transport_unavailable", errorCategoryTransport, http.StatusGatewayTimeout, true},
		{ProtocolError, "protocol_error", errorCategoryTransport, http.StatusBadGateway, false},
		{TimeoutExceeded, "timeout_exceeded", errorCategoryTransport, http.StatusGatewayTimeout, false},
		{UnsupportedFingerprint, "unsupported_fingerprint", errorCategoryTransport, http.StatusBadRequest, false},
		{UpstreamDNSFailure, "upstream_dns_failure", errorCategoryEgress, http.StatusBadGateway, true},
		{UpstreamTLSFailure, "upstream_tls_failure", errorCategoryEgress, http.StatusBadGateway, true},
		{UpstreamConnectionRefused, "upstream_connection_refused", errorCategoryEgress, http.StatusBadGateway, true},
		{UpstreamConnectTimeout, "upstream_connect_timeout", errorCategoryEgress, http.StatusGatewayTimeout, true},
		{UpstreamReset, "upstream_reset", errorCategoryEgress, http.StatusBadGateway, true},
		{UpstreamProxyFailure, "upstream_proxy_failure", errorCategoryEgress, http.StatusBadGateway, true},
		{StreamUploadAborted, "stream_upload_aborted", errorCategoryStreaming, http.StatusBadGateway, false},
		{StreamDownloadAborted, "stream_download_aborted", errorCategoryStreaming, http.StatusBadGateway, false},
		{BodyRefUnavailable, "body_ref_unavailable", errorCategoryStreaming, http.StatusConflict, false},
		{BodyTooLarge, "body_too_large", errorCategoryStreaming, http.StatusRequestEntityTooLarge, false},
		{ControlInternalError, "control_internal_error", errorCategoryControl, http.StatusInternalServerError, false},
		{ExecutorInternalError, "executor_internal_error", errorCategoryEgress, http.StatusBadGateway, false},
		{Cancelled, "cancelled", errorCategoryClient, statusClientClosedRequest, false},
	}

	if len(rows) != len(ErrorRegistry) {
		t.Fatalf("test table has %d rows, registry has %d entries", len(rows), len(ErrorRegistry))
	}

	for _, row := range rows {
		entry, ok := ErrorRegistry[row.code]
		if !ok {
			t.Errorf("%s (%d): missing registry entry", row.name, row.code)

			continue
		}

		if entry.Code != row.name {
			t.Errorf("%d: code name = %q, want %q", row.code, entry.Code, row.name)
		}

		if entry.Category != row.category {
			t.Errorf("%s: category = %q, want %q", row.name, entry.Category, row.category)
		}

		if entry.HTTPStatus != row.httpStatus {
			t.Errorf("%s: http = %d, want %d", row.name, entry.HTTPStatus, row.httpStatus)
		}

		if entry.Retryable != row.retryable {
			t.Errorf("%s: retryable = %v, want %v", row.name, entry.Retryable, row.retryable)
		}
	}
}

// TestErrorResponseIsPublicSafe checks that the ErrorResponse envelope never
// carries worker_id or session_id, that details are string-valued, and that
// request_id is always present (docs/public/architecture.md "ErrorResponse JSON Format").
func TestErrorResponseIsPublicSafe(t *testing.T) {
	resp := ErrorResponseFromCode(BodyTooLarge, "req_abc123", map[string]string{
		errorDetailDirectionKey:  "request",
		errorDetailLimitBytesKey: strconv.Itoa(1048576),
	})

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any

	err = json.Unmarshal(raw, &decoded)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, forbidden := range []string{"worker_id", "session_id"} {
		if _, present := decoded[forbidden]; present {
			t.Errorf("ErrorResponse exposes forbidden field %q", forbidden)
		}
	}

	if decoded["request_id"] != "req_abc123" {
		t.Errorf("request_id = %v, want req_abc123", decoded["request_id"])
	}

	details, ok := decoded["details"].(map[string]any)
	if !ok {
		t.Fatalf("details missing or wrong type: %v", decoded["details"])
	}

	for key, value := range details {
		if _, isString := value.(string); !isString {
			t.Errorf("details[%q] = %v is not a string", key, value)
		}
	}
}

// TestErrorResponseOmitsZeroRetryAfter verifies retry_after_ms is not emitted
// and that unknown codes fall back to control_internal_error rather than
// leaking an empty envelope.
func TestErrorResponseFallbackAndOmissions(t *testing.T) {
	resp := ErrorResponseFromCode(ErrorCode(99999), "req_x", nil)
	if resp.Code != errorCodeControlInternalError || resp.Category != errorCategoryControl {
		t.Fatalf("unknown code fallback = %+v, want control_internal_error", resp)
	}

	raw, err := json.Marshal(ErrorResponseFromCode(AuthFailure, "req_y", nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any

	err = json.Unmarshal(raw, &decoded)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, present := decoded["retry_after_ms"]; present {
		t.Errorf("retry_after_ms should be omitted when not set")
	}

	if _, present := decoded["details"]; present {
		t.Errorf("details should be omitted when empty")
	}
}

// TestOriginStatusPassthroughIsNotErrorResponse asserts that upstream 4xx/5xx
// origin statuses are carried in the SuccessResponse envelope's status field
// and are never converted into a Straw ErrorResponse (docs/public/architecture.md
// "Origin Status Passthrough", docs/public/architecture.md "REST outcome" row).
func TestOriginStatusPassthroughIsNotErrorResponse(t *testing.T) {
	for _, originStatus := range []int{404, 429, 500, 502} {
		envelope := SuccessResponse{
			RequestID: "req_origin",
			Status:    originStatus,
			Body:      ResponseBody{Mode: handlerTestInlineBase64},
		}

		raw, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded map[string]any

		err = json.Unmarshal(raw, &decoded)
		if err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if got := int(decoded["status"].(float64)); got != originStatus {
			t.Errorf("envelope status = %d, want %d", got, originStatus)
		}

		// A success envelope must not look like an ErrorResponse: it carries no
		// category/code/retryable fields.
		for _, errField := range []string{"category", "code", "retryable"} {
			if _, present := decoded[errField]; present {
				t.Errorf("origin %d envelope leaked ErrorResponse field %q", originStatus, errField)
			}
		}
	}
}

// TestExecutorEmittableSetMatchesContract pins the executor-emittable code set
// to exactly the list in docs/public/architecture.md "Executor Error Reporting". Any code
// outside the set maps to executor_internal_error and is flagged as a protocol
// violation for cooldown accounting (docs/public/architecture.md "Error mapping" row).
func TestExecutorEmittableSetMatchesContract(t *testing.T) {
	want := map[strawpb.ErrorCode]struct{}{
		strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED:          {},
		strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED:            {},
		strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT:     {},
		strawpb.ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE:        {},
		strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE:        {},
		strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED: {},
		strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECT_TIMEOUT:    {},
		strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET:              {},
		strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE:      {},
		strawpb.ErrorCode_ERROR_CODE_STREAM_UPLOAD_ABORTED:       {},
		strawpb.ErrorCode_ERROR_CODE_STREAM_DOWNLOAD_ABORTED:     {},
		strawpb.ErrorCode_ERROR_CODE_BODY_REF_UNAVAILABLE:        {},
		strawpb.ErrorCode_ERROR_CODE_BODY_TOO_LARGE:              {},
		strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR:     {},
	}

	// Every code in the contract set passes validation unchanged.
	for code := range want {
		mapped, violation := ValidateExecutorError(code)
		if violation || mapped != code {
			t.Errorf("emittable code %v: mapped=%v violation=%v, want pass-through", code, mapped, violation)
		}
	}

	// Every non-emittable canonical code is a protocol violation mapped to
	// executor_internal_error.
	for num := range strawpb.ErrorCode_name {
		code := strawpb.ErrorCode(num)
		if _, emittable := want[code]; emittable {
			continue
		}

		mapped, violation := ValidateExecutorError(code)
		if !violation || mapped != strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR {
			t.Errorf("non-emittable code %v: mapped=%v violation=%v, want executor_internal_error violation", code, mapped, violation)
		}
	}
}

// TestErrorCodeFromName round-trips registry code names.
func TestErrorCodeFromName(t *testing.T) {
	if got := ErrorCodeFromName("header_injection_failed"); got != HeaderInjectionFailed {
		t.Errorf("ErrorCodeFromName(header_injection_failed) = %d, want %d", got, HeaderInjectionFailed)
	}

	if got := ErrorCodeFromName("does_not_exist"); got != 0 {
		t.Errorf("ErrorCodeFromName(unknown) = %d, want 0", got)
	}
}
