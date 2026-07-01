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
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/protocol"
	"github.com/beremaran/straw/internal/protocol/wirepb"
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
	authToken       string
	routes          []config.Route
	resultTimeout   time.Duration
	allowPrivateIPs bool
}

type controlRequest struct {
	ID              string            `json:"id,omitempty"`
	Method          string            `json:"method,omitempty"`
	URL             string            `json:"url"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            []byte            `json:"body,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	MaxResponseSize int64             `json:"max_response_size,omitempty"`
}

// NewControlHandler creates a control handler.
func NewControlHandler(
	b brokerClient,
	egressID string,
	authToken string,
	routes []config.Route,
	resultTimeout time.Duration,
	allowPrivateIPs bool,
) *ControlHandler {
	return &ControlHandler{
		broker:          b,
		egressID:        egressID,
		authToken:       authToken,
		routes:          routes,
		resultTimeout:   resultTimeout,
		allowPrivateIPs: allowPrivateIPs,
	}
}

// Handle proxies a request through the configured egress.
func (h *ControlHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !h.authorize(w, r) {
		return
	}

	req, ok := h.readControlRequest(w, r)
	if !ok {
		return
	}

	egressID, ok := h.resolveEgress(w, r)
	if !ok {
		return
	}

	if !h.prepareControlRequest(w, r, req) {
		return
	}

	result, ok := h.sendAndWait(ctx, w, egressID, req)
	if !ok {
		return
	}

	writeControlResult(w, result)
	slog.InfoContext(ctx, "request proxied", "request_id", req.GetId(), "egress_id", result.GetEgressId())
}

func (h *ControlHandler) authorize(w http.ResponseWriter, r *http.Request) bool {
	if h.authToken == "" {
		return true
	}

	if r.Header.Get("Authorization") == "Bearer "+h.authToken {
		return true
	}

	writeError(w, http.StatusUnauthorized, "missing or invalid authorization")

	return false
}

func (h *ControlHandler) resolveEgress(w http.ResponseWriter, r *http.Request) (string, bool) {
	want := config.Route{
		EgressID: r.Header.Get("X-Straw-Egress-ID"),
		Country:  r.Header.Get("X-Straw-Country"),
		IPType:   r.Header.Get("X-Straw-IP-Type"),
	}

	if want.EgressID == "" && want.Country == "" && want.IPType == "" {
		return h.egressID, true
	}

	if want.EgressID != "" {
		return h.resolveEgressID(w, want.EgressID)
	}

	for _, route := range h.routes {
		if strings.EqualFold(route.Country, want.Country) && strings.EqualFold(route.IPType, want.IPType) {
			return route.EgressID, true
		}
	}

	writeError(w, http.StatusForbidden, "egress route not allowed")

	return "", false
}

func (h *ControlHandler) resolveEgressID(w http.ResponseWriter, egressID string) (string, bool) {
	if egressID == h.egressID {
		return egressID, true
	}

	for _, route := range h.routes {
		if route.EgressID == egressID {
			return route.EgressID, true
		}
	}

	writeError(w, http.StatusForbidden, "egress route not allowed")

	return "", false
}

func (h *ControlHandler) readControlRequest(w http.ResponseWriter, r *http.Request) (*wirepb.Request, bool) {
	var reqDTO controlRequest

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

	req, err := reqDTO.toWire()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return nil, false
	}

	return req, true
}

func (r *controlRequest) toWire() (*wirepb.Request, error) {
	var timeout time.Duration

	if r.Timeout != "" {
		var err error

		timeout, err = time.ParseDuration(r.Timeout)
		if err != nil {
			return nil, fmt.Errorf("parse timeout: %w", err)
		}
	}

	headers := make([]*wirepb.Header, 0, len(r.Headers))
	for k, v := range r.Headers {
		headers = append(headers, &wirepb.Header{Key: k, Value: v})
	}

	return &wirepb.Request{
		Id:              r.ID,
		Method:          r.Method,
		Url:             r.URL,
		Headers:         headers,
		Body:            r.Body,
		TimeoutNanos:    int64(timeout),
		MaxResponseSize: r.MaxResponseSize,
	}, nil
}

func (h *ControlHandler) prepareControlRequest(w http.ResponseWriter, r *http.Request, req *wirepb.Request) bool {
	if req.GetUrl() == "" {
		writeError(w, http.StatusBadRequest, "missing url")

		return false
	}

	if req.GetMethod() == "" {
		req.Method = http.MethodGet
	}

	if req.GetId() == "" {
		req.Id = r.Header.Get("X-Request-ID")
	}

	if req.GetId() == "" {
		req.Id = "req_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	err := validator.ValidateTargetURL(r.Context(), req.GetUrl(), h.allowPrivateIPs)
	if err != nil {
		writeError(w, http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))

		return false
	}

	return true
}

func (h *ControlHandler) sendAndWait(ctx context.Context, w http.ResponseWriter, egressID string, req *wirepb.Request) (*wirepb.Response, bool) {
	replyTo := "results." + req.GetId()
	req.ReplyTo = replyTo

	body, err := protocol.MarshalRequest(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode request")

		return nil, false
	}

	subject := "tasks." + egressID + ".tasks"

	err = h.broker.Publish(ctx, subject, body)
	if err != nil {
		slog.Error("failed to publish task", "error", err, "subject", subject)
		writeError(w, http.StatusBadGateway, "failed to publish request")

		return nil, false
	}

	resultBody, err := h.broker.ConsumeOnce(ctx, replyTo, h.resultTimeout)
	if err != nil {
		if errors.Is(err, broker.ErrTimeout) {
			writeTimeoutResponse(w, req.GetId())
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

func writeControlResult(w http.ResponseWriter, result *wirepb.Response) {
	for _, h := range result.GetHeaders() {
		if !isFilteredResponseHeader(h.GetKey()) {
			w.Header().Add(h.GetKey(), h.GetValue())
		}
	}

	if result.GetEgressId() != "" {
		w.Header().Set("X-Control-Egress", result.GetEgressId())
	}

	if result.GetTiming() != nil {
		w.Header().Set("X-Control-Timing", time.Duration(result.GetTiming().GetTotalNanos()).Round(time.Millisecond).String())
	}

	status := int(result.GetStatusCode())
	if status == 0 {
		status = http.StatusOK
	}

	if result.GetError() != nil {
		if status == http.StatusOK {
			status = http.StatusBadGateway
		}

		writeEgressError(w, status, result.GetError())

		return
	}

	w.WriteHeader(status)
	_, _ = w.Write(result.GetBody())
}

func isFilteredResponseHeader(key string) bool {
	for _, filtered := range filteredResponseHeaders {
		if strings.EqualFold(key, filtered) {
			return true
		}
	}

	return false
}

func writeEgressError(w http.ResponseWriter, status int, errInfo *wirepb.ErrorInfo) {
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
