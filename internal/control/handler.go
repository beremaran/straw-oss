package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	maxHandlerBodyBytes  = 4 << 20
	receiptFinishTimeout = 5 * time.Second
)

var requestSequence atomic.Uint64

// RequestHandler handles POST /api/v1/requests.
type RequestHandler struct {
	maxRequestBodyBytes uint64
	maxTimeoutMs        uint64
	authenticator       *Authenticator
	dispatcher          RequestDispatcher
	configCache         *ConfigCache
	receipts            RequestReceiptPreparer
}

// RequestReceiptPreparer claims and releases verified request receipts.
type RequestReceiptPreparer interface {
	PrepareRequest(ctx context.Context, deploymentID, receiptID, requestID string) (*strawpb.BodyRefFrame, error)
	FinishRequest(ctx context.Context, deploymentID, receiptID, requestID string, consumed bool) error
}

// NewRequestHandler creates the REST request handler.
func NewRequestHandler(maxRequestBodyBytes, maxTimeoutMs uint64, auth *Authenticator) *RequestHandler {
	return &RequestHandler{
		maxRequestBodyBytes: maxRequestBodyBytes,
		maxTimeoutMs:        maxTimeoutMs,
		authenticator:       auth,
	}
}

// SetDispatcher attaches the request execution pipeline.
func (h *RequestHandler) SetDispatcher(dispatcher RequestDispatcher) { h.dispatcher = dispatcher }

// SetConfigCache attaches the immutable deployment policy.
func (h *RequestHandler) SetConfigCache(cache *ConfigCache) { h.configCache = cache }

// SetReceiptPreparer enables receipt request bodies.
func (h *RequestHandler) SetReceiptPreparer(preparer RequestReceiptPreparer) { h.receipts = preparer }

// ServeHTTP validates, dispatches, and returns one upstream response envelope.
func (h *RequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()

	identity, err := h.authenticate(r)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(AuthFailure, requestID, nil))

		return
	}

	body, err := readRequestBody(r)
	if err != nil {
		WriteValidationError(w, requestID, &ValidationError{Code: errorCodeInvalidRequest, Message: "request body is required and must be JSON"})

		return
	}

	validated, err := ValidateRequest(body, h.maxRequestBodyBytes, h.effectiveMaxTimeout())
	if err != nil {
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			WriteValidationError(w, requestID, validationErr)
		} else {
			WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, requestID, nil))
		}

		return
	}

	if h.dispatcher == nil {
		writePipelineError(w, requestID, &PipelineError{Code: ControlInternalError})

		return
	}

	prepareErr := h.prepareReceipt(r.Context(), identity, requestID, validated)
	if prepareErr != nil {
		writePipelineError(w, requestID, prepareErr)

		return
	}

	response, dispatchErr := h.dispatcher.Dispatch(r.Context(), DispatchInput{
		RequestID: requestID,
		Identity:  identity,
		Request:   validated,
	})
	if dispatchErr != nil {
		h.finishReceipt(r.Context(), identity, requestID, validated, false)
		writePipelineError(w, requestID, dispatchErr)

		return
	}

	h.finishReceipt(r.Context(), identity, requestID, validated, true)
	writeSuccessResponse(w, response)
}

func (h *RequestHandler) prepareReceipt(ctx context.Context, identity Identity, requestID string, request *ValidatedRequest) *PipelineError {
	if request.ResponseBodyMode == bodyModeReceipt && h.receipts == nil {
		return &PipelineError{Code: BodyRefUnavailable, Message: "response receipts are not enabled"}
	}

	if request.BodyReceiptID == "" {
		return nil
	}

	if h.receipts == nil {
		return &PipelineError{Code: BodyRefUnavailable}
	}

	bodyRef, err := h.receipts.PrepareRequest(ctx, identity.DeploymentID, request.BodyReceiptID, requestID)
	if err != nil || bodyRef.GetExpectedSizeBytes() > math.MaxInt64 {
		return &PipelineError{Code: BodyRefUnavailable}
	}

	bodySize, sizeErr := strconv.ParseInt(strconv.FormatUint(bodyRef.GetExpectedSizeBytes(), 10), 10, 64)
	if sizeErr != nil {
		return &PipelineError{Code: BodyRefUnavailable}
	}

	request.BodyRef = bodyRef
	request.BodySizeBytes = bodySize

	return nil
}

func (h *RequestHandler) finishReceipt(ctx context.Context, identity Identity, requestID string, request *ValidatedRequest, consumed bool) {
	if request.BodyReceiptID == "" || h.receipts == nil {
		return
	}

	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), receiptFinishTimeout)
	defer cancel()

	_ = h.receipts.FinishRequest(finishCtx, identity.DeploymentID, request.BodyReceiptID, requestID, consumed)
}

func (h *RequestHandler) authenticate(r *http.Request) (Identity, error) {
	if h.authenticator == nil {
		return Identity{}, ErrAuthFailure
	}

	return h.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
}

func (h *RequestHandler) effectiveMaxTimeout() uint64 {
	if h.configCache == nil {
		return h.maxTimeoutMs
	}

	snapshot := h.configCache.Snapshot()
	if snapshot.MaxTimeoutMs == 0 || snapshot.MaxTimeoutMs > h.maxTimeoutMs {
		return h.maxTimeoutMs
	}

	return snapshot.MaxTimeoutMs
}

func readRequestBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxHandlerBodyBytes)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	return body, nil
}

func writeSuccessResponse(w http.ResponseWriter, response SuccessResponse) {
	w.Header().Set(headerCanonicalContentType, "application/json")
	w.WriteHeader(http.StatusOK)

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		return
	}
}

func writePipelineError(w http.ResponseWriter, requestID string, err *PipelineError) {
	status, response := pipelineHTTPError(requestID, err)
	WriteError(w, status, response)
}

func pipelineErrorResponse(requestID string, err *PipelineError) ErrorResponse {
	if err == nil {
		err = &PipelineError{Code: ControlInternalError}
	}

	response := ErrorResponseFromCodeWithRetry(err.Code, requestID, err.Details, err.RetryAfterMs)
	if err.Message != "" {
		response.Message = err.Message
	}

	if err.TimeoutType != "" {
		response.TimeoutType = err.TimeoutType
	}

	return response
}

func pipelineHTTPError(requestID string, err *PipelineError) (int, ErrorResponse) {
	response := pipelineErrorResponse(requestID, err)
	if err == nil {
		err = &PipelineError{Code: ControlInternalError}
	}

	status := http.StatusInternalServerError
	if entry, ok := ErrorRegistry[err.Code]; ok {
		status = entry.HTTPStatus
	}

	return status, response
}

// SuccessResponse is the JSON envelope for a successful upstream response.
type SuccessResponse struct {
	RequestID string        `json:"request_id"`
	Status    int           `json:"status"`
	Headers   []HeaderPair  `json:"headers,omitempty"`
	Body      ResponseBody  `json:"body"`
	Timing    RequestTiming `json:"timing"`

	ResponseSizeBytes          uint64 `json:"-"`
	SelectedFingerprintProfile string `json:"-"`
	ExecutedFingerprintProfile string `json:"-"`
}

// ResponseBody carries the base64-encoded upstream response body.
type ResponseBody struct {
	Mode       string `json:"mode"`
	DataBase64 string `json:"data_base64,omitempty"`
	Truncated  bool   `json:"truncated"`
	ReceiptID  string `json:"receipt_id,omitempty"`
	SizeBytes  uint64 `json:"size_bytes,omitempty"`
	SHA256Hex  string `json:"sha256_hex,omitempty"`
}

// RequestTiming reports request pipeline phases in milliseconds.
type RequestTiming struct {
	RoutingMs    int64 `json:"routing_ms"`
	AssignmentMs int64 `json:"assignment_ms"`
	EgressMs     int64 `json:"egress_ms"`
	TotalMs      int64 `json:"total_ms"`
}

func generateRequestID() string {
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), requestSequence.Add(1))
}
