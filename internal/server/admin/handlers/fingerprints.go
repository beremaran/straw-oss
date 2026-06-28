package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/pkg/broker"
)

type FingerprintHandler struct {
	repo   domain.FingerprintRepository
	broker broker.MessageBroker
}

func NewFingerprintHandler(repo domain.FingerprintRepository, broker broker.MessageBroker) *FingerprintHandler {
	return &FingerprintHandler{repo: repo, broker: broker}
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
	if err := helper.ReadJSON(r, &req); err != nil {
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

	if existing != nil {
		err := h.repo.UpdatePreset(r.Context(), preset)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to update preset")

			return
		}
	} else {
		err := h.repo.CreatePreset(r.Context(), preset)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to create preset")

			return
		}
	}

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

	err = h.broker.Publish(r.Context(), "fingerprint_broadcast", "", body)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to broadcast")

		return
	}

	w.WriteHeader(http.StatusOK)
}
