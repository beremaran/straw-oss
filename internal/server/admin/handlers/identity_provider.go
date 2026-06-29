package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/google/uuid"
)

type IdentityProviderHandler struct {
	repo domain.IdentityRepository
}

func NewIdentityProviderHandler(repo domain.IdentityRepository) *IdentityProviderHandler {
	return &IdentityProviderHandler{repo: repo}
}

func (h *IdentityProviderHandler) HandleListIdentityProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.repo.ListIdentityProviders(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list identity providers")

		return
	}

	data := make([]dto.AdminIdentityProviderResponse, len(providers))
	for i, provider := range providers {
		data[i] = dto.FromDomainIdentityProvider(provider)
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListIdentityProvidersResponse{
		Data: data,
	})
}

func (h *IdentityProviderHandler) HandleCreateIdentityProvider(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateIdentityProviderRequest
	if err := helper.ReadJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		helper.WriteError(w, http.StatusBadRequest, "identity provider name is required")

		return
	}

	providerType := strings.TrimSpace(req.Type)
	if providerType == "" {
		helper.WriteError(w, http.StatusBadRequest, "identity provider type is required")

		return
	}

	providerID := uuid.New().String()
	config := domain.ConfigMap{}
	for k, v := range req.Config {
		config[k] = v
	}

	provider := &domain.AdminIdentityProvider{
		ID:              providerID,
		Name:            name,
		Type:            providerType,
		IssuerURL:       req.IssuerURL,
		ClientID:        req.ClientID,
		ClientSecretRef: req.ClientSecretRef,
		JWKSURL:         req.JWKSURL,
		Scopes:          req.Scopes,
		RoleClaim:       req.RoleClaim,
		DefaultRoleID:   req.DefaultRoleID,
		IsEnabled:       req.IsEnabled,
		Config:          config,
	}

	if err := h.repo.CreateIdentityProvider(r.Context(), provider); err != nil {
		if errors.Is(err, postgres.ErrPlaintextProviderSecret) {
			helper.WriteError(w, http.StatusBadRequest, "identity provider config must not contain secrets")
		} else if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			helper.WriteError(w, http.StatusConflict, "identity provider name already exists")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to create identity provider")
		}

		return
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "identity_provider:create",
		"entity_type", "identity_provider",
		"entity_id", providerID,
		"new_value", provider,
	)

	helper.WriteJSON(w, http.StatusCreated, dto.FromDomainIdentityProvider(*provider))
}

func (h *IdentityProviderHandler) HandleUpdateIdentityProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	provider, err := h.repo.GetIdentityProviderByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch identity provider")

		return
	}
	if provider == nil {
		helper.WriteError(w, http.StatusNotFound, "identity provider not found")

		return
	}

	var req dto.UpdateIdentityProviderRequest
	if err := helper.ReadJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		helper.WriteError(w, http.StatusBadRequest, "identity provider name is required")

		return
	}

	providerType := strings.TrimSpace(req.Type)
	if providerType == "" {
		helper.WriteError(w, http.StatusBadRequest, "identity provider type is required")

		return
	}

	oldProvider := *provider

	config := domain.ConfigMap{}
	for k, v := range req.Config {
		config[k] = v
	}

	provider.Name = name
	provider.Type = providerType
	provider.IssuerURL = req.IssuerURL
	provider.ClientID = req.ClientID
	provider.ClientSecretRef = req.ClientSecretRef
	provider.JWKSURL = req.JWKSURL
	provider.Scopes = req.Scopes
	provider.RoleClaim = req.RoleClaim
	provider.DefaultRoleID = req.DefaultRoleID
	provider.IsEnabled = req.IsEnabled
	provider.Config = config

	if err := h.repo.UpdateIdentityProvider(r.Context(), provider); err != nil {
		if errors.Is(err, postgres.ErrPlaintextProviderSecret) {
			helper.WriteError(w, http.StatusBadRequest, "identity provider config must not contain secrets")
		} else if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			helper.WriteError(w, http.StatusConflict, "identity provider name already exists")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to update identity provider")
		}

		return
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "identity_provider:update",
		"entity_type", "identity_provider",
		"entity_id", id,
		"old_value", oldProvider,
		"new_value", provider,
	)

	helper.WriteJSON(w, http.StatusOK, dto.FromDomainIdentityProvider(*provider))
}

func (h *IdentityProviderHandler) HandleDeleteIdentityProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	provider, err := h.repo.GetIdentityProviderByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch identity provider")

		return
	}
	if provider == nil {
		helper.WriteError(w, http.StatusNotFound, "identity provider not found")

		return
	}

	if err := h.repo.DisableIdentityProvider(r.Context(), id); err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to disable identity provider")

		return
	}

	slog.InfoContext(r.Context(), "audit event",
		"action", "identity_provider:disable",
		"entity_type", "identity_provider",
		"entity_id", id,
	)

	w.WriteHeader(http.StatusNoContent)
}
