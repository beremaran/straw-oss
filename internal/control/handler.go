package control

import (
	"context"
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
	authenticator        *Authenticator
	metadataWriter       RequestMetadataRecorder
	dispatcher           RequestDispatcher
	configCache          *ConfigCache
}

// NewRequestHandler creates a handler with the given config limits. auth
// authenticates every request; a nil authenticator is rejected by
// MustNewRequestHandler-style callers should not happen in production, but
// ServeHTTP treats a nil authenticator as "always deny" rather than
// "always allow" to fail closed.
func NewRequestHandler(maxRequestBodyBytes, maxResponseBodyBytes, maxTimeoutMs uint64, auth *Authenticator, metadataWriter ...RequestMetadataRecorder) *RequestHandler {
	var recorder RequestMetadataRecorder
	if len(metadataWriter) > 0 {
		recorder = metadataWriter[0]
	}

	return &RequestHandler{
		maxRequestBodyBytes:  maxRequestBodyBytes,
		maxResponseBodyBytes: maxResponseBodyBytes,
		maxTimeoutMs:         maxTimeoutMs,
		authenticator:        auth,
		metadataWriter:       recorder,
	}
}

// SetDispatcher wires the real request execution path.
func (h *RequestHandler) SetDispatcher(dispatcher RequestDispatcher) {
	h.dispatcher = dispatcher
}

// SetConfigCache wires tenant policy lookup for timeout and metadata storage.
func (h *RequestHandler) SetConfigCache(cache *ConfigCache) {
	h.configCache = cache
}

// Handler is the http.HandlerFunc for POST /api/v1/requests.
func (h *RequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrorResponse{
			Category:  "client",
			Code:      errorCodeUnsupportedIngressMode,
			Message:   "only POST is allowed on /api/v1/requests",
			Retryable: false,
			RequestID: "",
		})

		return
	}

	requestID := generateRequestID()

	identity, err := h.authenticateAndAuthorize(r)
	if err != nil {
		switch {
		case errors.Is(err, ErrInsufficientPermissions):
			WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, requestID, nil))
		case errors.Is(err, ErrTenantNotFound):
			WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(TenantNotFound, requestID, nil))
		default:
			WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(AuthFailure, requestID, nil))
		}

		return
	}

	body, err := readRequestBody(r)
	if err != nil {
		WriteValidationError(w, requestID, &ValidationError{
			Code:    errorCodeInvalidRequest,
			Message: "request body is required and must be JSON",
		})

		return
	}

	policy := h.tenantPolicy(r.Context(), identity.TenantID)

	validated, err := ValidateRequest(body, h.maxRequestBodyBytes, effectiveMaxTimeout(h.maxTimeoutMs, policy.MaxTimeoutMs))
	if err != nil {
		var verr *ValidationError
		if asValidationError(err, &verr) {
			WriteValidationError(w, requestID, verr)

			return
		}

		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, requestID, nil))

		return
	}

	h.dispatchValidated(w, r, requestID, identity, validated)
}

func (h *RequestHandler) dispatchValidated(w http.ResponseWriter, r *http.Request, requestID string, identity Identity, validated *ValidatedRequest) {
	event := buildRequestEvent(requestID, identity, validated, h.tenantPolicy(r.Context(), identity.TenantID))

	if h.dispatcher == nil {
		perr := &PipelineError{Code: ControlInternalError}
		h.recordOutcome(event, SuccessResponse{}, perr)
		writePipelineError(w, requestID, perr)

		return
	}

	resp, dispatchErr := h.dispatcher.Dispatch(r.Context(), DispatchInput{
		RequestID: requestID,
		Identity:  identity,
		Request:   validated,
	})
	h.recordOutcome(event, resp, dispatchErr)

	if dispatchErr != nil {
		writePipelineError(w, requestID, dispatchErr)

		return
	}

	writeSuccessResponse(w, resp)
}

func (h *RequestHandler) tenantPolicy(ctx context.Context, tenantID string) TenantPolicy {
	if h.configCache == nil {
		return defaultTenantPolicy()
	}

	snap, err := h.configCache.Snapshot(ctx, tenantID)
	if err != nil {
		return defaultTenantPolicy()
	}

	return TenantPolicy{
		DefaultTimeoutMs:     snap.DefaultTimeoutMs,
		MaxTimeoutMs:         snap.MaxTimeoutMs,
		MetadataQueryStorage: MetadataStoragePolicy(snap.MetadataQueryStorage),
		MetadataPathStorage:  MetadataStoragePolicy(snap.MetadataPathStorage),
	}.normalized()
}

func effectiveMaxTimeout(staticMax, tenantMax uint64) uint64 {
	if staticMax == 0 {
		return tenantMax
	}

	if tenantMax == 0 || tenantMax > staticMax {
		return staticMax
	}

	return tenantMax
}

// recordOutcome finalizes and enqueues the request_events row with the real
// dispatch outcome (docs/tasks/p0/32), instead of the pre-dispatch
// placeholder the writer previously emitted.
func (h *RequestHandler) recordOutcome(event RequestEvent, resp SuccessResponse, perr *PipelineError) {
	if h.metadataWriter == nil {
		return
	}

	h.metadataWriter.Enqueue(applyRequestOutcome(event, resp, perr))
}

// authenticateAndAuthorize resolves the caller's Identity and enforces the
// data-plane execution rule from docs/planning/06 and
// docs/planning/07-public-api-surface.md: platform-scoped keys can never
// execute POST /api/v1/requests, and only requester/tenant_admin
// tenant-scoped roles may (see rbac.go for the P0 operator default).
func (h *RequestHandler) authenticateAndAuthorize(r *http.Request) (Identity, error) {
	if h.authenticator == nil {
		return Identity{}, ErrAuthFailure
	}

	identity, err := h.authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		return Identity{}, err
	}

	if !CanExecuteDataPlane(identity) {
		return Identity{}, ErrInsufficientPermissions
	}

	return identity, nil
}

func readRequestBody(r *http.Request) ([]byte, error) {
	const maxBody = 4 << 20

	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
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

func writeSuccessResponse(w http.ResponseWriter, response SuccessResponse) {
	w.Header().Set(headerCanonicalContentType, "application/json")
	w.WriteHeader(http.StatusOK)

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		return
	}
}

func writePipelineError(w http.ResponseWriter, requestID string, err *PipelineError) {
	if err == nil {
		err = &PipelineError{Code: ControlInternalError}
	}

	resp := ErrorResponseFromCodeWithRetry(err.Code, requestID, err.Details, err.RetryAfterMs)
	if err.Message != "" {
		resp.Message = err.Message
	}

	if err.TimeoutType != "" {
		resp.TimeoutType = err.TimeoutType
	}

	status := http.StatusInternalServerError
	if entry, ok := ErrorRegistry[err.Code]; ok {
		status = entry.HTTPStatus
	}

	WriteError(w, status, resp)
}

// SuccessResponse is the JSON envelope for successful upstream transport.
type SuccessResponse struct {
	RequestID string        `json:"request_id"`
	Status    int           `json:"status"`
	Headers   []HeaderPair  `json:"headers,omitempty"`
	Body      ResponseBody  `json:"body"`
	Timing    RequestTiming `json:"timing"`
	// ResponseSizeBytes is the raw (pre-base64) upstream response body size,
	// carried for request_events telemetry (docs/tasks/p0/32). Not part of
	// the wire contract.
	ResponseSizeBytes uint64 `json:"-"`
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
// generateRequestID creates a unique request ID.
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}
