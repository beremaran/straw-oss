package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/server/dto"
	"github.com/labstack/echo/v4"
)

type FingerprintHandler struct {
	repo   domain.FingerprintRepository
	broker broker.MessageBroker
}

func NewFingerprintHandler(repo domain.FingerprintRepository, broker broker.MessageBroker) *FingerprintHandler {
	return &FingerprintHandler{repo: repo, broker: broker}
}

// HandleListPresets returns all fingerprint presets
//
//	@Summary		List Fingerprint Presets
//	@Description	Returns all available fingerprint presets for TLS fingerprinting
//	@Tags			fingerprints
//	@Produce		json
//	@Success		200	{array}		dto.FingerprintResponse	"List of presets"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/fingerprints [get]
func (h *FingerprintHandler) HandleListPresets(c echo.Context) error {
	presets, err := h.repo.ListPresets(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list presets"})
	}
	return c.JSON(http.StatusOK, dto.FromFingerprintPresets(presets))
}

// HandleCreatePreset creates or updates a preset
//
//	@Summary		Create or Update Fingerprint Preset
//	@Description	Creates a new fingerprint preset or updates an existing one
//	@Tags			fingerprints
//	@Accept			json
//	@Produce		json
//	@Param			preset	body		dto.CreateFingerprintRequest	true	"Fingerprint preset"
//	@Success		200		{object}	dto.FingerprintResponse	"Created or updated preset"
//	@Failure		400		{object}	map[string]string		"Invalid request"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/fingerprints [post]
func (h *FingerprintHandler) HandleCreatePreset(c echo.Context) error {
	var req dto.CreateFingerprintRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.ID == "" || req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id and name are required"})
	}

	preset := req.ToDomain()

	// Check availability
	existing, err := h.repo.GetPreset(c.Request().Context(), preset.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to check existing preset"})
	}

	if existing != nil {
		if err := h.repo.UpdatePreset(c.Request().Context(), preset); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update preset"})
		}
	} else {
		if err := h.repo.CreatePreset(c.Request().Context(), preset); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create preset"})
		}
	}

	return c.JSON(http.StatusOK, dto.FromFingerprintPreset(preset))
}

// HandleBroadcastPresets sends all presets to endpoints via fanout
//
//	@Summary		Broadcast Fingerprint Presets
//	@Description	Sends all fingerprint presets to all connected endpoints via message broker
//	@Tags			fingerprints
//	@Success		200	"Presets broadcast successfully"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/fingerprints/broadcast [post]
func (h *FingerprintHandler) HandleBroadcastPresets(c echo.Context) error {
	presets, err := h.repo.ListPresets(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list presets"})
	}

	// Convert to DTOs for broadcast
	presetsDTO := dto.FromFingerprintPresets(presets)

	body, err := json.Marshal(presetsDTO)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal presets"})
	}

	// Publish to fanout exchange
	// Exchange: "fingerprint_broadcast", routing key: ignored
	err = h.broker.Publish(c.Request().Context(), "fingerprint_broadcast", "", body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to broadcast"})
	}

	return c.NoContent(http.StatusOK)
}
