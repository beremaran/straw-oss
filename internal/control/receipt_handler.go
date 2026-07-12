package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/beremaran/straw-oss/internal/receipt"
)

const (
	receiptChecksumHeader   = "X-Straw-Part-SHA256"
	maxReceiptMetadataBytes = 64 << 10
)

// ReceiptHandler exposes the deployment-authenticated receipt lifecycle and
// the separately signed assignment download endpoint used by Egress.
type ReceiptHandler struct {
	service *receipt.Service
	auth    *Authenticator
}

// NewReceiptHandler creates the deployment-authenticated receipt API.
func NewReceiptHandler(service *receipt.Service, auth *Authenticator) *ReceiptHandler {
	return &ReceiptHandler{service: service, auth: auth}
}

// Register adds receipt lifecycle and signed-object routes to mux.
func (h *ReceiptHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/receipts", h.create)
	mux.HandleFunc("GET /api/v1/receipts/{id}", h.get)
	mux.HandleFunc("DELETE /api/v1/receipts/{id}", h.cancel)
	mux.HandleFunc("PUT /api/v1/receipts/{id}/parts/{part}", h.putPart)
	mux.HandleFunc("POST /api/v1/receipts/{id}/complete", h.complete)
	mux.HandleFunc("GET /api/v1/receipts/{id}/content", h.content)
	mux.HandleFunc("GET /api/v1/receipt-objects/{id}", h.assignedObject)
}

type createReceiptRequest struct {
	Direction      string `json:"direction"`
	SizeBytes      int64  `json:"size_bytes"`
	SHA256Hex      string `json:"sha256_hex"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}
type receiptResponse struct {
	receipt.Record
	StatusURL          string `json:"status_url"`
	PartUploadTemplate string `json:"part_upload_template,omitempty"`
	CompleteURL        string `json:"complete_url,omitempty"`
	DownloadURL        string `json:"download_url,omitempty"`
}

func (h *ReceiptHandler) create(w http.ResponseWriter, r *http.Request) {
	deployment, ok := h.deployment(w, r)
	if !ok {
		return
	}

	var input createReceiptRequest

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReceiptMetadataBytes))
	dec.DisallowUnknownFields()

	err := dec.Decode(&input)
	if err != nil {
		h.error(w, http.StatusBadRequest, "invalid receipt request")

		return
	}

	record, err := h.service.Create(r.Context(), deployment, receipt.CreateInput{Direction: input.Direction, SizeBytes: input.SizeBytes, SHA256Hex: input.SHA256Hex, IdempotencyKey: input.IdempotencyKey})
	if err != nil {
		h.serviceError(w, err)

		return
	}

	h.write(w, http.StatusCreated, h.response(record))
}

func (h *ReceiptHandler) get(w http.ResponseWriter, r *http.Request) {
	deployment, ok := h.deployment(w, r)
	if !ok {
		return
	}

	record, err := h.service.Get(r.Context(), deployment, r.PathValue("id"))
	if err != nil {
		h.serviceError(w, err)

		return
	}

	h.write(w, http.StatusOK, h.response(record))
}

func (h *ReceiptHandler) cancel(w http.ResponseWriter, r *http.Request) {
	deployment, ok := h.deployment(w, r)
	if !ok {
		return
	}

	record, err := h.service.Cancel(r.Context(), deployment, r.PathValue("id"))
	if err != nil {
		h.serviceError(w, err)

		return
	}

	h.write(w, http.StatusOK, h.response(record))
}

func (h *ReceiptHandler) complete(w http.ResponseWriter, r *http.Request) {
	deployment, ok := h.deployment(w, r)
	if !ok {
		return
	}

	record, err := h.service.Complete(r.Context(), deployment, r.PathValue("id"))
	if err != nil {
		h.serviceError(w, err)

		return
	}

	h.write(w, http.StatusOK, h.response(record))
}

func (h *ReceiptHandler) putPart(w http.ResponseWriter, r *http.Request) {
	deployment, ok := h.deployment(w, r)
	if !ok {
		return
	}

	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil || r.ContentLength < 0 {
		h.error(w, http.StatusBadRequest, "part number and Content-Length are required")

		return
	}

	record, err := h.service.PutPart(r.Context(), deployment, r.PathValue("id"), part, r.Body, r.ContentLength, r.Header.Get(receiptChecksumHeader))
	if err != nil {
		h.serviceError(w, err)

		return
	}

	h.write(w, http.StatusOK, h.response(record))
}

func (h *ReceiptHandler) content(w http.ResponseWriter, r *http.Request) {
	deployment, ok := h.deployment(w, r)
	if !ok {
		return
	}

	body, record, err := h.service.OpenResponse(r.Context(), deployment, r.PathValue("id"))
	if err != nil {
		h.serviceError(w, err)

		return
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(record.SizeBytes, 10))
	w.Header().Set("X-Straw-SHA256", record.SHA256Hex)
	_, _ = io.Copy(w, body)
}

func (h *ReceiptHandler) assignedObject(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	body, record, err := h.service.OpenAssigned(r.Context(), r.PathValue("id"), q.Get("deployment_id"), q.Get("request_id"), q.Get("expires"), q.Get("signature"))
	if err != nil {
		h.error(w, http.StatusForbidden, "assignment reference is invalid or expired")

		return
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(record.SizeBytes, 10))
	_, _ = io.Copy(w, body)
}

func (h *ReceiptHandler) deployment(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.auth == nil {
		h.error(w, http.StatusUnauthorized, "authentication required")

		return "", false
	}

	identity, err := h.auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		h.error(w, http.StatusUnauthorized, "invalid deployment token")

		return "", false
	}

	return identity.DeploymentID, true
}

func (h *ReceiptHandler) response(record receipt.Record) receiptResponse {
	base := "/api/v1/receipts/" + record.ID

	out := receiptResponse{Record: record, StatusURL: base}
	if record.State == receipt.StateUploading {
		out.PartUploadTemplate = base + "/parts/{part_number}"
		out.CompleteURL = base + "/complete"
	}

	if record.Direction == receipt.DirectionResponse && (record.State == receipt.StateVerified || record.State == receipt.StateConsumed) {
		out.DownloadURL = base + "/content"
	}

	return out
}

func (h *ReceiptHandler) write(w http.ResponseWriter, status int, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(raw, '\n'))
}

func (h *ReceiptHandler) error(w http.ResponseWriter, status int, message string) {
	h.write(w, status, map[string]any{"code": "receipt_error", "message": message})
}

func (h *ReceiptHandler) serviceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError

	switch {
	case errors.Is(err, receipt.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, receipt.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, receipt.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, receipt.ErrUnauthorized):
		status = http.StatusForbidden
	}

	h.error(w, status, strings.TrimPrefix(fmt.Sprint(err), "receipt: "))
}
