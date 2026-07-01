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
	"github.com/beremaran/straw/internal/validator"
)

type brokerClient interface {
	Publish(ctx context.Context, subject string, body []byte) error
	ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error)
}

const errorMessageKey = "message"

// ControlHandler sends an incoming HTTP request to one configured egress.
type ControlHandler struct {
	broker          brokerClient
	egressID        string
	resultTimeout   time.Duration
	allowPrivateIPs bool
}

// NewControlHandler creates a control handler.
func NewControlHandler(
	b brokerClient,
	egressID string,
	resultTimeout time.Duration,
	allowPrivateIPs bool,
) *ControlHandler {
	return &ControlHandler{
		broker:          b,
		egressID:        egressID,
		resultTimeout:   resultTimeout,
		allowPrivateIPs: allowPrivateIPs,
	}
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

	err := readJSON(r, &reqDTO)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")

			return nil, false
		}

		writeError(w, http.StatusBadRequest, "invalid request body")

		return nil, false
	}

	req, err := reqDTO.ToProtocolRequest()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return nil, false
	}

	return req, true
}

func (h *ControlHandler) prepareControlRequest(w http.ResponseWriter, r *http.Request, req *protocol.Request) bool {
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "missing url")

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

	err := validator.ValidateTargetURL(r.Context(), req.URL, h.allowPrivateIPs)
	if err != nil {
		writeError(w, http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))

		return false
	}

	return true
}

func (h *ControlHandler) sendAndWait(ctx context.Context, w http.ResponseWriter, req *protocol.Request) (*protocol.Response, bool) {
	replyTo := "results." + req.ID
	req.ReplyTo = replyTo

	body, err := protocol.MarshalRequest(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode request")

		return nil, false
	}

	subject := "tasks." + h.egressID + ".tasks"

	err = h.broker.Publish(ctx, subject, body)
	if err != nil {
		slog.Error("failed to publish task", "error", err, "subject", subject)
		writeError(w, http.StatusBadGateway, "failed to publish request")

		return nil, false
	}

	resultBody, err := h.broker.ConsumeOnce(ctx, replyTo, h.resultTimeout)
	if err != nil {
		if errors.Is(err, broker.ErrTimeout) {
			writeTimeoutResponse(w, req.ID)
		} else {
			slog.Error("failed to consume result", "error", err, "subject", replyTo)
			writeError(w, http.StatusBadGateway, "failed to receive response")
		}

		return nil, false
	}

	result, err := protocol.UnmarshalResponse(resultBody)
	if err != nil {
		writeError(w, http.StatusBadGateway, "invalid egress response")

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
			"code":          errInfo.Code,
			errorMessageKey: errInfo.Message,
			"retryable":     errInfo.Retryable,
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
			"code":          protocol.ErrCodeEgressTimeout,
			errorMessageKey: "egress did not respond in time",
			"retryable":     true,
			"request_id":    requestID,
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

func readJSON(r *http.Request, dst any) error {
	err := json.NewDecoder(r.Body).Decode(dst)
	if err != nil {
		return fmt.Errorf("decode json request: %w", err)
	}

	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(errorResponse{
		Error: errorBody{errorMessageKey: message},
	})
	if err != nil {
		slog.Error("failed to encode error response", "error", err)
	}
}
