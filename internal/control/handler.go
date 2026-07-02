package control

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RequestHandler handles POST /api/v1/requests.
type RequestHandler struct {
	maxRequestBodyBytes  uint64
	maxResponseBodyBytes uint64
	maxTimeoutMs         uint64
}

// NewRequestHandler creates a handler with the given config limits.
func NewRequestHandler(maxRequestBodyBytes, maxResponseBodyBytes, maxTimeoutMs uint64) *RequestHandler {
	return &RequestHandler{
		maxRequestBodyBytes:  maxRequestBodyBytes,
		maxResponseBodyBytes: maxResponseBodyBytes,
		maxTimeoutMs:         maxTimeoutMs,
	}
}

// Handler is the http.HandlerFunc for POST /api/v1/requests.
func (h *RequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrorResponse{
			Category:  "client",
			Code:      "unsupported_ingress_mode",
			Message:   "only POST is allowed on /api/v1/requests",
			Retryable: false,
			RequestID: "",
		})
		return
	}

	requestID := generateRequestID()

	body, err := readRequestBody(r)
	if err != nil {
		WriteValidationError(w, requestID, &ValidationError{
			Code:    "invalid_request",
			Message: "request body is required and must be JSON",
		})
		return
	}

	validated, err := ValidateRequest(body, h.maxRequestBodyBytes, h.maxTimeoutMs)
	if err != nil {
		var verr *ValidationError
		if asValidationError(err, &verr) {
			WriteValidationError(w, requestID, verr)
			return
		}
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(6, requestID, nil))
		return
	}

	// TODO: authenticate API key, derive tenant_id (task 07)
	// TODO: authorize data-plane execution
	// TODO: rate limit and quota admission (task 13)
	// TODO: deny rules check
	// TODO: routing evaluation (task 09)
	// TODO: assignment and stream lifecycle (task 10)
	// TODO: egress outbound execution (task 11)

	_ = validated // validated is used by later tasks; stub for now.

	response := SuccessResponse{
		RequestID: requestID,
		Status:    200,
		Headers:   nil,
		Body: ResponseBody{
			Mode:      "inline_base64",
			Truncated: false,
		},
		Timing: RequestTiming{
			RoutingMs:    0,
			AssignmentMs: 0,
			EgressMs:     0,
			TotalMs:      0,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func readRequestBody(r *http.Request) ([]byte, error) {
	const maxBody = 4 << 20
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func asValidationError(err error, target **ValidationError) bool {
	var verr *ValidationError
	if errors.As(err, &verr) {
		*target = verr
		return true
	}
	return false
}

// SuccessResponse is the JSON envelope for successful upstream transport.
type SuccessResponse struct {
	RequestID string        `json:"request_id"`
	Status    int           `json:"status"`
	Headers   []HeaderPair  `json:"headers,omitempty"`
	Body      ResponseBody  `json:"body"`
	Timing    RequestTiming `json:"timing"`
}

// ResponseBody carries the upstream response body.
type ResponseBody struct {
	Mode       string `json:"mode"`
	DataBase64 string `json:"data_base64,omitempty"`
	Truncated  bool   `json:"truncated"`
}

// RequestTiming captures per-phase latency.
type RequestTiming struct {
	RoutingMs    int64 `json:"routing_ms"`
	AssignmentMs int64 `json:"assignment_ms"`
	EgressMs     int64 `json:"egress_ms"`
	TotalMs      int64 `json:"total_ms"`
}

// encodeHeadersBase64 encodes header values as base64 for the response envelope.
func encodeHeadersBase64(headers []HeaderPair) []HeaderPair {
	out := make([]HeaderPair, len(headers))
	for i, h := range headers {
		out[i] = HeaderPair{
			Name:  h.Name,
			Value: base64.StdEncoding.EncodeToString([]byte(h.Value)),
		}
	}
	return out
}

// generateRequestID creates a unique request ID.
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}
