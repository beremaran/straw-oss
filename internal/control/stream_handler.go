package control

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const (
	streamContentType = "application/vnd.straw.request-stream.v1+binary"

	streamFrameMetadata byte = 1
	streamFrameBody     byte = 2
	streamFrameTrailers byte = 3
	streamFrameEnd      byte = 4
	streamFrameError    byte = 5
)

var errStreamFramePayloadTooLarge = errors.New("stream frame payload too large")

// ServeStreamHTTP handles POST /api/v1/requests:stream.
func (h *RequestHandler) ServeStreamHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrorResponse{
			Category:  errorCategoryClient,
			Code:      errorCodeUnsupportedIngressMode,
			Message:   "only POST is allowed on /api/v1/requests:stream",
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
	capturePolicy := h.payloadCapturePolicy(r.Context(), identity.TenantID)

	validated, err := ValidateRequestWithCapturePolicy(body, h.maxRequestBodyBytes, effectiveMaxTimeout(h.maxTimeoutMs, policy.MaxTimeoutMs), capturePolicy)
	if err != nil {
		var verr *ValidationError
		if asValidationError(err, &verr) {
			WriteValidationError(w, requestID, verr)

			return
		}

		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, requestID, nil))

		return
	}

	h.dispatchStreamValidated(r.Context(), w, requestID, identity, validated)
}

func (h *RequestHandler) dispatchStreamValidated(ctx context.Context, w http.ResponseWriter, requestID string, identity Identity, validated *ValidatedRequest) {
	event := buildRequestEvent(requestID, identity, validated, h.tenantPolicy(ctx, identity.TenantID))

	raw, ok := h.dispatcher.(RawResponseDispatcher)
	if h.dispatcher == nil || !ok {
		perr := &PipelineError{Code: ControlInternalError}
		h.recordOutcome(event, SuccessResponse{}, perr)
		writePipelineError(w, requestID, perr)

		return
	}

	stream := newBinaryStreamResponseWriter(w, requestID)
	resp, perr, wroteHeader := raw.DispatchRaw(ctx, DispatchInput{
		RequestID: requestID,
		Identity:  identity,
		Request:   validated,
	}, stream)
	h.recordOutcome(event, resp, perr)

	if perr != nil {
		if wroteHeader || stream.started() {
			_ = stream.writeError(pipelineErrorResponse(requestID, perr))

			return
		}

		writePipelineError(w, requestID, perr)

		return
	}

	err := stream.writeEnd(resp.Timing)
	if err != nil {
		return
	}
}

type binaryStreamResponseWriter struct {
	w         http.ResponseWriter
	requestID string
	startedOK bool
}

type streamMetadataPayload struct {
	RequestID string       `json:"request_id"`
	Status    int          `json:"status"`
	Headers   []HeaderPair `json:"headers,omitempty"`
}

type streamTrailersPayload struct {
	Headers []HeaderPair `json:"headers,omitempty"`
}

type streamEndPayload struct {
	Timing RequestTiming `json:"timing"`
}

func newBinaryStreamResponseWriter(w http.ResponseWriter, requestID string) *binaryStreamResponseWriter {
	return &binaryStreamResponseWriter{w: w, requestID: requestID}
}

func (s *binaryStreamResponseWriter) Header() http.Header {
	return s.w.Header()
}

func (s *binaryStreamResponseWriter) WriteHeader(status int) {
	if s.startedOK {
		return
	}

	streamStatus, err := uint32FromInt(status)
	if err != nil {
		streamStatus = http.StatusInternalServerError
	}

	s.writeRawResponseStart(streamStatus, headersFromHTTPHeader(s.w.Header()))
}

func (s *binaryStreamResponseWriter) Write(data []byte) (int, error) {
	if !s.startedOK {
		s.WriteHeader(http.StatusOK)
	}

	err := s.writeFrame(streamFrameBody, data)
	if err != nil {
		return 0, err
	}

	flushRawResponse(s.w)

	return len(data), nil
}

func (s *binaryStreamResponseWriter) WriteRawResponseStart(status uint32, headers []*strawpb.Header) {
	s.writeRawResponseStart(status, headersFromProto(headers))
}

func (s *binaryStreamResponseWriter) WriteRawTrailers(headers []*strawpb.Header) {
	if !s.startedOK {
		return
	}

	_ = s.writeJSONFrame(streamFrameTrailers, streamTrailersPayload{Headers: headersFromProto(headers)})
	flushRawResponse(s.w)
}

func (s *binaryStreamResponseWriter) started() bool {
	return s.startedOK
}

func (s *binaryStreamResponseWriter) writeRawResponseStart(status uint32, headers []HeaderPair) {
	if s.startedOK {
		return
	}

	s.w.Header().Set(headerCanonicalContentType, streamContentType)
	s.w.WriteHeader(http.StatusOK)
	s.startedOK = true
	_ = s.writeJSONFrame(streamFrameMetadata, streamMetadataPayload{
		RequestID: s.requestID,
		Status:    int(status),
		Headers:   headers,
	})
	flushRawResponse(s.w)
}

func (s *binaryStreamResponseWriter) writeEnd(timing RequestTiming) error {
	if !s.startedOK {
		s.WriteHeader(http.StatusOK)
	}

	err := s.writeJSONFrame(streamFrameEnd, streamEndPayload{Timing: timing})
	flushRawResponse(s.w)

	return err
}

func (s *binaryStreamResponseWriter) writeError(resp ErrorResponse) error {
	err := s.writeJSONFrame(streamFrameError, resp)
	flushRawResponse(s.w)

	return err
}

func (s *binaryStreamResponseWriter) writeJSONFrame(frameType byte, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal stream frame: %w", err)
	}

	return s.writeFrame(frameType, raw)
}

func (s *binaryStreamResponseWriter) writeFrame(frameType byte, payload []byte) error {
	payloadLen, err := uint32FromInt(len(payload))
	if err != nil {
		return fmt.Errorf("%w: %d", errStreamFramePayloadTooLarge, len(payload))
	}

	header := [5]byte{frameType}
	binary.BigEndian.PutUint32(header[1:], payloadLen)

	_, err = s.w.Write(header[:])
	if err != nil {
		return fmt.Errorf("write stream frame header: %w", err)
	}

	n, err := s.w.Write(payload)
	if err != nil {
		return fmt.Errorf("write stream frame payload: %w", err)
	}

	if n != len(payload) {
		return io.ErrShortWrite
	}

	return nil
}

func headersFromHTTPHeader(h http.Header) []HeaderPair {
	var out []HeaderPair

	for name, values := range h {
		if !rawResponseHeaderAllowed(name) {
			continue
		}

		for _, value := range values {
			out = append(out, HeaderPair{Name: name, Value: base64.StdEncoding.EncodeToString([]byte(value))})
		}
	}

	return out
}

func uint32FromInt(v int) (uint32, error) {
	if v < 0 {
		return 0, errStreamFramePayloadTooLarge
	}

	out, err := strconv.ParseUint(strconv.Itoa(v), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: %d", errStreamFramePayloadTooLarge, v)
	}

	return uint32(out), nil
}
