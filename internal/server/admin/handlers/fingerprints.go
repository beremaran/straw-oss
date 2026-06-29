package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/pkg/broker"
)

var errFingerprintBrokerUnavailable = errors.New("fingerprint broker unavailable")

var sensitiveFingerprintConfigKeys = []string{"authorization", "password", "raw_key", "secret", "token"}

type FingerprintHandler struct {
	repo            domain.FingerprintRepository
	routingRuleRepo FingerprintRoutingRuleRepository
	identityRepo    FingerprintIdentityRepository
	broker          broker.MessageBroker
	auditRepo       domain.ManagementAuditRepository
}

type FingerprintRoutingRuleRepository interface {
	ListActiveRulesReferencingFingerprintPreset(ctx context.Context, presetID string) ([]domain.RoutingRuleReference, error)
	DeleteRule(ctx context.Context, id string) error
}

type FingerprintIdentityRepository interface {
	ListUserRoles(ctx context.Context, userID string) ([]domain.AdminRole, error)
}

type fingerprintDeleteOptions struct {
	force          bool
	broadcast      bool
	deactivateRefs bool
}

func NewFingerprintHandler(repo domain.FingerprintRepository, routingRuleRepo FingerprintRoutingRuleRepository, identityRepo FingerprintIdentityRepository, broker broker.MessageBroker, auditRepo domain.ManagementAuditRepository) *FingerprintHandler {
	return &FingerprintHandler{
		repo:            repo,
		routingRuleRepo: routingRuleRepo,
		identityRepo:    identityRepo,
		broker:          broker,
		auditRepo:       auditRepo,
	}
}

func (h *FingerprintHandler) HandleListPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := h.repo.ListPresets(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list presets")

		return
	}
	helper.WriteJSON(w, http.StatusOK, dto.FromFingerprintPresets(presets))
}

func (h *FingerprintHandler) HandleGetPreset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	preset, err := h.repo.GetPreset(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get preset")

		return
	}
	if preset == nil {
		helper.WriteError(w, http.StatusNotFound, "preset not found")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.FromFingerprintPreset(preset))
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

func (h *FingerprintHandler) HandleDeletePreset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	opts, ok := deletePresetOptions(w, r)
	if !ok {
		return
	}

	if !h.authorizeForceDelete(w, r, opts.force) {
		return
	}

	preset, ok := h.getPresetForDelete(w, r, id)
	if !ok {
		return
	}

	if !h.prepareReferencingRulesForDelete(w, r, id, opts) {
		return
	}

	if !h.deletePreset(w, r, preset, opts.broadcast) {
		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.FingerprintDeleteResponse{
		ID:                 id,
		Deleted:            true,
		BroadcastRequested: opts.broadcast,
	})
}

func (h *FingerprintHandler) HandleBroadcastPresets(w http.ResponseWriter, r *http.Request) {
	err := h.broadcastPresets(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to broadcast")

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *FingerprintHandler) authorizeForceDelete(w http.ResponseWriter, r *http.Request, force bool) bool {
	if !force {
		return true
	}

	allowed, err := h.canForceDelete(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to verify owner role")

		return false
	}
	if !allowed {
		helper.WriteError(w, http.StatusForbidden, "force delete requires owner role")

		return false
	}

	return true
}

func (h *FingerprintHandler) getPresetForDelete(w http.ResponseWriter, r *http.Request, id string) (*domain.FingerprintPreset, bool) {
	preset, err := h.repo.GetPreset(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get preset")

		return nil, false
	}
	if preset == nil {
		helper.WriteError(w, http.StatusNotFound, "preset not found")

		return nil, false
	}

	return preset, true
}

func (h *FingerprintHandler) prepareReferencingRulesForDelete(w http.ResponseWriter, r *http.Request, id string, opts fingerprintDeleteOptions) bool {
	referencingRules, err := h.routingRuleRepo.ListActiveRulesReferencingFingerprintPreset(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to check referencing rules")

		return false
	}
	if len(referencingRules) == 0 {
		return true
	}
	if !opts.force {
		writeFingerprintDeleteConflict(w, referencingRules)

		return false
	}
	if !opts.deactivateRefs {
		helper.WriteJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Error: "deactivate_referencing_rules=true is required when force=true and referencing rules exist",
			Code:  "FINGERPRINT_FORCE_REQUIRES_DEACTIVATION",
		})

		return false
	}

	err = h.deactivateReferencingRules(r.Context(), referencingRules)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to deactivate referencing rules")

		return false
	}

	return true
}

func (h *FingerprintHandler) deletePreset(w http.ResponseWriter, r *http.Request, preset *domain.FingerprintPreset, broadcast bool) bool {
	err := h.repo.DeletePreset(r.Context(), preset.ID)
	if err != nil {
		if errors.Is(err, postgres.ErrPresetNotFound) {
			helper.WriteError(w, http.StatusNotFound, "preset not found")
		} else {
			helper.WriteError(w, http.StatusInternalServerError, "failed to delete preset")
		}

		return false
	}

	h.auditPresetDelete(r, preset)

	if !broadcast {
		return true
	}

	err = h.broadcastPresets(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to broadcast")

		return false
	}

	return true
}

func (h *FingerprintHandler) broadcastPresets(ctx context.Context) error {
	if h.broker == nil {
		return errFingerprintBrokerUnavailable
	}

	presets, err := h.repo.ListPresets(ctx)
	if err != nil {
		return fmt.Errorf("failed to list presets: %w", err)
	}

	body, err := json.Marshal(dto.FromFingerprintPresets(presets))
	if err != nil {
		return fmt.Errorf("failed to marshal presets: %w", err)
	}

	err = h.broker.Publish(ctx, "fingerprint_broadcast", body)
	if err != nil {
		return fmt.Errorf("failed to publish presets: %w", err)
	}

	return nil
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
		oldVal = redactedFingerprintPreset(existing)
	}

	newVal := redactedFingerprintPreset(preset)
	event := middleware.NewAuditEvent(r, action, "fingerprint_preset", preset.ID, oldVal, newVal)
	_ = h.auditRepo.Create(r.Context(), event)
}

func (h *FingerprintHandler) auditPresetDelete(r *http.Request, preset *domain.FingerprintPreset) {
	if h.auditRepo == nil {
		return
	}

	event := middleware.NewAuditEvent(r, domain.ActionDelete, "fingerprint_preset", preset.ID, redactedFingerprintPreset(preset), nil)
	_ = h.auditRepo.Create(r.Context(), event)
}

func (h *FingerprintHandler) deactivateReferencingRules(ctx context.Context, refs []domain.RoutingRuleReference) error {
	// ponytail: reuse existing rule deletes; add a transaction if partial force deletes become painful.
	for _, ref := range refs {
		err := h.routingRuleRepo.DeleteRule(ctx, ref.ID)
		if err != nil && !errors.Is(err, postgres.ErrRoutingRuleNotFound) {
			return err
		}
	}

	return nil
}

func (h *FingerprintHandler) canForceDelete(ctx context.Context) (bool, error) {
	actor, ok := middleware.ActorFromContext(ctx)
	if !ok {
		return false, nil
	}
	if actor.Type == middleware.ActorTypeLegacy {
		return true, nil
	}
	if actor.Type != middleware.ActorTypeUser || h.identityRepo == nil {
		return false, nil
	}

	roles, err := h.identityRepo.ListUserRoles(ctx, actor.ID)
	if err != nil {
		return false, err
	}

	return hasRoleName(roles, domain.RoleOwner), nil
}

func deletePresetOptions(w http.ResponseWriter, r *http.Request) (fingerprintDeleteOptions, bool) {
	forceDelete, err := parseBoolQuery(r.URL.Query().Get("force"), false)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid force parameter")

		return fingerprintDeleteOptions{}, false
	}

	broadcastRequested, err := parseBoolQuery(r.URL.Query().Get("broadcast"), true)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid broadcast parameter")

		return fingerprintDeleteOptions{}, false
	}

	deactivateRefs, err := parseBoolQuery(r.URL.Query().Get("deactivate_referencing_rules"), false)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid deactivate_referencing_rules parameter")

		return fingerprintDeleteOptions{}, false
	}

	return fingerprintDeleteOptions{
		force:          forceDelete,
		broadcast:      broadcastRequested,
		deactivateRefs: deactivateRefs,
	}, true
}

func parseBoolQuery(value string, defaultValue bool) (bool, error) {
	if value == "" {
		return defaultValue, nil
	}

	return strconv.ParseBool(value)
}

func writeFingerprintDeleteConflict(w http.ResponseWriter, refs []domain.RoutingRuleReference) {
	referencingRules := make([]dto.RoutingRuleReferenceResponse, len(refs))
	for i, ref := range refs {
		referencingRules[i] = dto.RoutingRuleReferenceResponse{
			ID:   ref.ID,
			Name: ref.Name,
		}
	}

	helper.WriteJSON(w, http.StatusConflict, dto.FingerprintDeleteConflictResponse{
		Error: "fingerprint preset is referenced by active routing rules",
		Code:  "FINGERPRINT_REFERENCED",
		Details: dto.FingerprintDeleteConflictDetails{
			ReferencingRules: referencingRules,
		},
	})
}

func redactedFingerprintPreset(preset *domain.FingerprintPreset) *dto.FingerprintResponse {
	resp := dto.FromFingerprintPreset(preset)
	if resp == nil {
		return nil
	}

	resp.Config = redactFingerprintConfig(resp.Config)

	return resp
}

func redactFingerprintConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}

	redacted := make(map[string]interface{}, len(config))
	for key, value := range config {
		if isSensitiveFingerprintConfigKey(key) {
			redacted[key] = "[REDACTED]"

			continue
		}

		redacted[key] = redactFingerprintConfigValue(value)
	}

	return redacted
}

func redactFingerprintConfigValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return redactFingerprintConfig(typed)
	case domain.ConfigMap:
		return redactFingerprintConfig(map[string]interface{}(typed))
	case []interface{}:
		values := make([]interface{}, len(typed))
		for i, item := range typed {
			values[i] = redactFingerprintConfigValue(item)
		}

		return values
	default:
		return value
	}
}

func isSensitiveFingerprintConfigKey(key string) bool {
	lowerKey := strings.ToLower(key)
	for _, sensitiveKey := range sensitiveFingerprintConfigKeys {
		if strings.Contains(lowerKey, sensitiveKey) {
			return true
		}
	}

	return false
}

func presetSaveError(existing *domain.FingerprintPreset) string {
	if existing != nil {
		return "failed to update preset"
	}

	return "failed to create preset"
}
