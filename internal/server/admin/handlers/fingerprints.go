package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/pkg/broker"
)

type FingerprintHandler struct {
	repo      domain.FingerprintRepository
	broker    broker.MessageBroker
	auditRepo domain.ManagementAuditRepository
}

func NewFingerprintHandler(repo domain.FingerprintRepository, broker broker.MessageBroker, auditRepo domain.ManagementAuditRepository) *FingerprintHandler {
	return &FingerprintHandler{repo: repo, broker: broker, auditRepo: auditRepo}
}

func (h *FingerprintHandler) HandleListPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := h.repo.ListPresets(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list presets")

		return
	}
	helper.WriteJSON(w, http.StatusOK, dto.FromFingerprintPresets(presets))
}

func (h *FingerprintHandler) HandleCreatePreset(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateFingerprintRequest
	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.ID == "" || req.Name == "" {
		helper.WriteError(w, http.StatusBadRequest, "id and name are required")

		return
	}

	preset := req.ToDomain()

	existing, err := h.repo.GetPreset(r.Context(), preset.ID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to check existing preset")

		return
	}

	err = h.savePreset(r.Context(), existing, preset)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, presetSaveError(existing))

		return
	}
	h.auditPresetChange(r, existing, preset)

	helper.WriteJSON(w, http.StatusOK, dto.FromFingerprintPreset(preset))
}

func (h *FingerprintHandler) HandleBroadcastPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := h.repo.ListPresets(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list presets")

		return
	}

	presetsDTO := dto.FromFingerprintPresets(presets)

	body, err := json.Marshal(presetsDTO)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to marshal presets")

		return
	}

	err = h.broker.Publish(r.Context(), "fingerprint_broadcast", body)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to broadcast")

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *FingerprintHandler) savePreset(ctx context.Context, existing *domain.FingerprintPreset, preset *domain.FingerprintPreset) error {
	if existing != nil {
		return h.repo.UpdatePreset(ctx, preset)
	}

	return h.repo.CreatePreset(ctx, preset)
}

func (h *FingerprintHandler) auditPresetChange(r *http.Request, existing *domain.FingerprintPreset, preset *domain.FingerprintPreset) {
	if h.auditRepo == nil {
		return
	}

	action := domain.ActionCreate
	var oldVal interface{}
	if existing != nil {
		action = domain.ActionUpdate
		oldVal = dto.FromFingerprintPreset(existing)
	}

	newVal := dto.FromFingerprintPreset(preset)
	event := middleware.NewAuditEvent(r, action, "fingerprint_preset", preset.ID, oldVal, newVal)
	_ = h.auditRepo.Create(r.Context(), event)
}

func presetSaveError(existing *domain.FingerprintPreset) string {
	if existing != nil {
		return "failed to update preset"
	}

	return "failed to create preset"
}
