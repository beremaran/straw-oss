// Package handlers provides HTTP handlers for the control server.
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

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/protocol"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/validator"
)

// ControlHandler sends an incoming HTTP request to one configured egress.
type ControlHandler struct {
	broker          broker.MessageBroker
	egressID        string
	resultTimeout   time.Duration
	allowPrivateIPs bool
}

// ControlHandlerOption configures a ControlHandler.
type ControlHandlerOption func(*ControlHandler)

// WithAllowPrivateIPs permits control requests to target private IP addresses.
func WithAllowPrivateIPs(allow bool) ControlHandlerOption {
	return func(h *ControlHandler) {
		h.allowPrivateIPs = allow
	}
}

// NewControlHandler creates a control handler.
func NewControlHandler(
	b broker.MessageBroker,
	egressID string,
	resultTimeout time.Duration,
	opts ...ControlHandlerOption,
) *ControlHandler {
	h := &ControlHandler{
		broker:        b,
		egressID:      egressID,
		resultTimeout: resultTimeout,
	}
	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Handle proxies a request through the configured egress.
func (h *ControlHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, ok := h.readControlRequest(w, r)
	if !ok {
		return
	}

	if !h.prepareControlRequest(w, r, req) {
		return
	}

	result, ok := h.sendAndWait(ctx, w, req)
	if !ok {
		return
	}

	writeControlResult(w, result)
	slog.InfoContext(ctx, "request proxied", "request_id", req.ID, "egress_id", result.EgressID)
}

func (h *ControlHandler) readControlRequest(w http.ResponseWriter, r *http.Request) (*protocol.Request, bool) {
	var reqDTO dto.ControlRequest

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

func (h *ControlHandler) prepareControlRequest(w http.ResponseWriter, r *http.Request, req *protocol.Request) bool {
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

func (h *ControlHandler) sendAndWait(ctx context.Context, w http.ResponseWriter, req *protocol.Request) (*protocol.Response, bool) {
	replyTo := "results." + req.ID
	req.ReplyTo = replyTo

	body, err := protocol.MarshalRequest(req)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to encode request")

		return nil, false
	}

	subject := "tasks." + h.egressID + ".tasks"

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

	result, err := protocol.UnmarshalResponse(resultBody)
	if err != nil {
		helper.WriteError(w, http.StatusBadGateway, "invalid egress response")

		return nil, false
	}

	return result, true
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
}

func writeControlResult(w http.ResponseWriter, result *protocol.Response) {
	for _, h := range result.Headers {
		if !isFilteredResponseHeader(h.Key) {
			w.Header().Add(h.Key, h.Value)
		}
	}

	if result.EgressID != "" {
		w.Header().Set("X-Control-Egress", result.EgressID)
	}

	if result.Timing != nil {
		w.Header().Set("X-Control-Timing", result.Timing.Total.Round(time.Millisecond).String())
	}

	status := result.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	if result.Error != nil {
		if status == http.StatusOK {
			status = http.StatusBadGateway
		}

		writeEgressError(w, status, result.Error)

		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(result.Body)
}

func isFilteredResponseHeader(key string) bool {
	for _, filtered := range filteredResponseHeaders {
		if strings.EqualFold(key, filtered) {
			return true
		}
	}

	return false
}

func writeEgressError(w http.ResponseWriter, status int, errInfo *protocol.ErrorInfo) {
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
		slog.Error("failed to encode egress error", "error", err)
	}
}

func writeTimeoutResponse(w http.ResponseWriter, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGatewayTimeout)

	err := json.NewEncoder(w).Encode(errorResponse{
		Error: errorBody{
			"code":       protocol.ErrCodeEgressTimeout,
			"message":    "egress did not respond in time",
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
