// Package handlers provides HTTP handlers for the relay server.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
	"github.com/beremaran/straw/pkg/validator"
)

// RelayHandler sends an incoming HTTP request to one configured endpoint.
type RelayHandler struct {
	broker          broker.MessageBroker
	endpointID      string
	hmacSecret      []byte
	resultTimeout   time.Duration
	allowPrivateIPs bool
}

// RelayHandlerOption configures a RelayHandler.
type RelayHandlerOption func(*RelayHandler)

// WithAllowPrivateIPs permits relay requests to target private IP addresses.
func WithAllowPrivateIPs(allow bool) RelayHandlerOption {
	return func(h *RelayHandler) {
		h.allowPrivateIPs = allow
	}
}

// NewRelayHandler creates a relay handler.
func NewRelayHandler(
	b broker.MessageBroker,
	endpointID string,
	hmacSecret []byte,
	resultTimeout time.Duration,
	opts ...RelayHandlerOption,
) *RelayHandler {
	h := &RelayHandler{
		broker:        b,
		endpointID:    endpointID,
		hmacSecret:    hmacSecret,
		resultTimeout: resultTimeout,
	}
	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Handle proxies a request through the configured endpoint.
func (h *RelayHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, ok := h.readRelayRequest(w, r)
	if !ok {
		return
	}

	if !h.prepareRelayRequest(w, r, req) {
		return
	}

	result, ok := h.sendAndWait(ctx, w, req)
	if !ok {
		return
	}

	if !decompressRelayResponse(w, result) {
		return
	}

	writeRelayResult(w, result)
	slog.InfoContext(ctx, "request proxied", "request_id", req.ID, "endpoint_id", result.EndpointID)
}

func (h *RelayHandler) readRelayRequest(w http.ResponseWriter, r *http.Request) (*protocol.Request, bool) {
	var reqDTO dto.RelayRequest

	err := helper.ReadJSON(r, &reqDTO)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "request body too large") {
			helper.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")

			return nil, false
		}

		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return nil, false
	}

	req, err := reqDTO.ToProtocolRequest()
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return nil, false
	}

	return req, true
}

func (h *RelayHandler) prepareRelayRequest(w http.ResponseWriter, r *http.Request, req *protocol.Request) bool {
	if req.URL == "" {
		helper.WriteError(w, http.StatusBadRequest, "missing url")

		return false
	}

	if req.Method == "" {
		req.Method = http.MethodGet
	}

	if req.ID == "" {
		req.ID = r.Header.Get("X-Request-ID")
	}

	if req.ID == "" {
		req.ID = "req_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	validationOpts := []validator.ValidationOption{}
	if h.allowPrivateIPs {
		validationOpts = append(validationOpts, validator.WithAllowPrivateIPs())
	}

	err := validator.ValidateTargetURL(r.Context(), req.URL, validationOpts...)
	if err != nil {
		helper.WriteError(w, http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))

		return false
	}

	return true
}

func (h *RelayHandler) sendAndWait(ctx context.Context, w http.ResponseWriter, req *protocol.Request) (*resultMessage, bool) {
	replyTo := "results." + req.ID
	req.ReplyTo = replyTo

	task, err := protocol.NewSignedTask(req, h.hmacSecret)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to sign request")

		return nil, false
	}

	body, err := json.Marshal(task)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to encode request")

		return nil, false
	}

	subject := "tasks." + h.endpointID + ".tasks"

	err = h.broker.Publish(ctx, subject, body)
	if err != nil {
		slog.Error("failed to publish task", "error", err, "subject", subject)
		helper.WriteError(w, http.StatusBadGateway, "failed to publish request")

		return nil, false
	}

	resultBody, err := h.broker.ConsumeOnce(ctx, replyTo, h.resultTimeout)
	if err != nil {
		if errors.Is(err, broker.ErrTimeout) {
			writeTimeoutResponse(w, req.ID)
		} else {
			slog.Error("failed to consume result", "error", err, "subject", replyTo)
			helper.WriteError(w, http.StatusBadGateway, "failed to receive response")
		}

		return nil, false
	}

	var result resultMessage

	err = json.Unmarshal(resultBody, &result)
	if err != nil {
		helper.WriteError(w, http.StatusBadGateway, "invalid endpoint response")

		return nil, false
	}

	return &result, true
}

// resultMessage carries the response from an endpoint back to the relay.
type resultMessage struct {
	RequestID      string               `json:"request_id"`
	EndpointID     string               `json:"endpoint_id,omitempty"`
	StatusCode     int                  `json:"status_code"`
	Headers        protocol.HeaderMap   `json:"headers"`
	CompressedBody []byte               `json:"body,omitempty"`
	BodyCompressed bool                 `json:"body_compressed"`
	Error          *protocol.ErrorInfo  `json:"error,omitempty"`
	Timing         *protocol.TimingInfo `json:"timing,omitempty"`
}

func decompressRelayResponse(w http.ResponseWriter, res *resultMessage) bool {
	if !res.BodyCompressed || len(res.CompressedBody) == 0 {
		return true
	}

	decompressed, err := protocol.Decompress(res.CompressedBody)
	if err != nil {
		helper.WriteError(w, http.StatusBadGateway, "failed to decompress response")

		return false
	}

	res.CompressedBody = decompressed
	res.BodyCompressed = false

	return true
}

var filteredResponseHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
	"Content-Length",
	"Content-Encoding",
}

func writeRelayResult(w http.ResponseWriter, result *resultMessage) {
	for _, h := range result.Headers {
		if !isFilteredResponseHeader(h.Key) {
			w.Header().Add(h.Key, h.Value)
		}
	}

	if result.EndpointID != "" {
		w.Header().Set("X-Relay-Endpoint", result.EndpointID)
	}

	if result.Timing != nil {
		w.Header().Set("X-Relay-Timing", result.Timing.Total.Round(time.Millisecond).String())
	}

	status := result.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	if result.Error != nil {
		if status == http.StatusOK {
			status = http.StatusBadGateway
		}

		writeEndpointError(w, status, result.Error)

		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(result.CompressedBody)
}

func isFilteredResponseHeader(key string) bool {
	for _, filtered := range filteredResponseHeaders {
		if strings.EqualFold(key, filtered) {
			return true
		}
	}

	return false
}

func writeEndpointError(w http.ResponseWriter, status int, errInfo *protocol.ErrorInfo) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(errorResponse{
		Error: errorBody{
			"code":      errInfo.Code,
			"message":   errInfo.Message,
			"retryable": errInfo.Retryable,
		},
	})
	if err != nil {
		slog.Error("failed to encode endpoint error", "error", err)
	}
}

func writeTimeoutResponse(w http.ResponseWriter, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGatewayTimeout)

	err := json.NewEncoder(w).Encode(errorResponse{
		Error: errorBody{
			"code":       protocol.ErrCodeEndpointTimeout,
			"message":    "endpoint did not respond in time",
			"retryable":  true,
			"request_id": requestID,
		},
	})
	if err != nil {
		slog.Error("failed to encode timeout error", "error", err)
	}
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody map[string]any
