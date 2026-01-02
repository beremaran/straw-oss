package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
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
func (h *FingerprintHandler) HandleListPresets(c echo.Context) error {
	presets, err := h.repo.ListPresets(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list presets"})
	}
	return c.JSON(http.StatusOK, presets)
}

// HandleCreatePreset creates or updates a preset
func (h *FingerprintHandler) HandleCreatePreset(c echo.Context) error {
	var preset domain.FingerprintPreset
	if err := c.Bind(&preset); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if preset.ID == "" || preset.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id and name are required"})
	}

	// Check availability
	existing, err := h.repo.GetPreset(c.Request().Context(), preset.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to check existing preset"})
	}

	if existing != nil {
		if err := h.repo.UpdatePreset(c.Request().Context(), &preset); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update preset"})
		}
	} else {
		if err := h.repo.CreatePreset(c.Request().Context(), &preset); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create preset"})
		}
	}

	return c.JSON(http.StatusOK, preset)
}

// HandleBroadcastPresets sends all presets to endpoints via fanout
func (h *FingerprintHandler) HandleBroadcastPresets(c echo.Context) error {
	presets, err := h.repo.ListPresets(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list presets"})
	}

	body, err := json.Marshal(presets)
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
