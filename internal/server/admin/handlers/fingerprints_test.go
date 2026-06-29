package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/pkg/broker"
)

const testPresetName = "Preset 1"

type mockFingerprintRepo struct {
	mock.Mock
}

func (m *mockFingerprintRepo) ListPresets(ctx context.Context) ([]domain.FingerprintPreset, error) {
	args := m.Called(ctx)

	return args.Get(0).([]domain.FingerprintPreset), args.Error(1)
}

func (m *mockFingerprintRepo) GetPreset(ctx context.Context, id string) (*domain.FingerprintPreset, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.FingerprintPreset), args.Error(1)
}

func (m *mockFingerprintRepo) CreatePreset(ctx context.Context, preset *domain.FingerprintPreset) error {
	args := m.Called(ctx, preset)

	return args.Error(0)
}

func (m *mockFingerprintRepo) UpdatePreset(ctx context.Context, preset *domain.FingerprintPreset) error {
	args := m.Called(ctx, preset)

	return args.Error(0)
}

func (m *mockFingerprintRepo) DeletePreset(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

type mockBroker struct {
	mock.Mock
}

func (m *mockBroker) Publish(ctx context.Context, subject string, body []byte) error {
	args := m.Called(ctx, subject, body)

	return args.Error(0)
}

func (m *mockBroker) Subscribe(_ context.Context, _ string, _ broker.Handler, _ ...broker.SubscribeOption) error {
	return nil
}

func (m *mockBroker) ConsumeOnce(_ context.Context, _ string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockBroker) Close() error {
	return nil
}

func (m *mockBroker) DeclareStream(_ context.Context, _ string, _ ...string) error {
	return nil
}

func (m *mockBroker) IsConnected() bool {
	return true
}

func TestFingerprintHandler_List(t *testing.T) {
	repo := new(mockFingerprintRepo)
	mb := new(mockBroker)
	h := NewFingerprintHandler(repo, nil, nil, mb, nil)

	presets := []domain.FingerprintPreset{
		{ID: "p1", Name: testPresetName},
	}
	repo.On("ListPresets", mock.Anything).Return(presets, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/fingerprints", nil)
	rec := httptest.NewRecorder()

	h.HandleListPresets(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	repo.AssertExpectations(t)
}

func TestFingerprintHandler_Create(t *testing.T) {
	repo := new(mockFingerprintRepo)
	mb := new(mockBroker)
	h := NewFingerprintHandler(repo, nil, nil, mb, nil)

	preset := domain.FingerprintPreset{ID: "p1", Name: testPresetName, Config: domain.ConfigMap{"a": 1}}
	body, err := json.Marshal(preset)
	require.NoError(t, err)

	repo.On("GetPreset", mock.Anything, "p1").Return((*domain.FingerprintPreset)(nil), nil)
	repo.On("CreatePreset", mock.Anything, mock.MatchedBy(func(p *domain.FingerprintPreset) bool {
		return p.ID == "p1"
	})).Return(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/fingerprints", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleCreatePreset(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	repo.AssertExpectations(t)
}

func TestFingerprintHandler_Broadcast(t *testing.T) {
	repo := new(mockFingerprintRepo)
	mb := new(mockBroker)
	h := NewFingerprintHandler(repo, nil, nil, mb, nil)

	presets := []domain.FingerprintPreset{{ID: "p1"}}
	repo.On("ListPresets", mock.Anything).Return(presets, nil)
	mb.On("Publish", mock.Anything, "fingerprint_broadcast", mock.Anything).Return(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/fingerprints/broadcast", nil)
	rec := httptest.NewRecorder()

	h.HandleBroadcastPresets(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	repo.AssertExpectations(t)
	mb.AssertExpectations(t)
}

func TestFingerprintHandler_GetPreset(t *testing.T) {
	repo := new(mockFingerprintRepo)
	h := NewFingerprintHandler(repo, nil, nil, nil, nil)

	preset := &domain.FingerprintPreset{ID: "p1", Name: testPresetName}
	repo.On("GetPreset", mock.Anything, "p1").Return(preset, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/fingerprints/p1", nil)
	req.SetPathValue("id", "p1")
	rec := httptest.NewRecorder()

	h.HandleGetPreset(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	repo.AssertExpectations(t)
}

func TestFingerprintHandler_DeleteConflict(t *testing.T) {
	repo := new(mockFingerprintRepo)
	ruleRepo := new(MockRoutingRuleRepo)
	h := NewFingerprintHandler(repo, ruleRepo, nil, nil, nil)

	preset := &domain.FingerprintPreset{ID: "p1", Name: testPresetName}
	refs := []domain.RoutingRuleReference{{ID: "r1", Name: "Default"}}
	repo.On("GetPreset", mock.Anything, "p1").Return(preset, nil)
	ruleRepo.On("ListActiveRulesReferencingFingerprintPreset", mock.Anything, "p1").Return(refs, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/fingerprints/p1", nil)
	req.SetPathValue("id", "p1")
	rec := httptest.NewRecorder()

	h.HandleDeletePreset(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "FINGERPRINT_REFERENCED")
	assert.Contains(t, rec.Body.String(), "r1")
	repo.AssertExpectations(t)
	ruleRepo.AssertExpectations(t)
}

func TestFingerprintHandler_DeleteForceRequiresOwner(t *testing.T) {
	repo := new(mockFingerprintRepo)
	ruleRepo := new(MockRoutingRuleRepo)
	identityRepo := new(MockIdentityRepo)
	h := NewFingerprintHandler(repo, ruleRepo, identityRepo, nil, nil)

	identityRepo.On("ListUserRoles", mock.Anything, "user-1").Return([]domain.AdminRole{{Name: domain.RoleOperator}}, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/fingerprints/p1?force=true", nil)
	req = req.WithContext(middleware.ContextWithActor(req.Context(), middleware.Actor{
		Type: middleware.ActorTypeUser,
		ID:   "user-1",
	}))
	req.SetPathValue("id", "p1")
	rec := httptest.NewRecorder()

	h.HandleDeletePreset(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	identityRepo.AssertExpectations(t)
	ruleRepo.AssertNotCalled(t, "ListActiveRulesReferencingFingerprintPreset", mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "DeletePreset", mock.Anything, mock.Anything)
}

func TestFingerprintHandler_DeleteForceDeactivatesRules(t *testing.T) {
	repo := new(mockFingerprintRepo)
	ruleRepo := new(MockRoutingRuleRepo)
	identityRepo := new(MockIdentityRepo)
	h := NewFingerprintHandler(repo, ruleRepo, identityRepo, nil, nil)

	preset := &domain.FingerprintPreset{ID: "p1", Name: testPresetName}
	refs := []domain.RoutingRuleReference{{ID: "r1", Name: "Default"}}
	identityRepo.On("ListUserRoles", mock.Anything, "owner-1").Return([]domain.AdminRole{{Name: domain.RoleOwner}}, nil)
	repo.On("GetPreset", mock.Anything, "p1").Return(preset, nil)
	ruleRepo.On("ListActiveRulesReferencingFingerprintPreset", mock.Anything, "p1").Return(refs, nil)
	ruleRepo.On("DeleteRule", mock.Anything, "r1").Return(nil)
	repo.On("DeletePreset", mock.Anything, "p1").Return(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/fingerprints/p1?force=true&deactivate_referencing_rules=true&broadcast=false", nil)
	req = req.WithContext(middleware.ContextWithActor(req.Context(), middleware.Actor{
		Type: middleware.ActorTypeUser,
		ID:   "owner-1",
	}))
	req.SetPathValue("id", "p1")
	rec := httptest.NewRecorder()

	h.HandleDeletePreset(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"broadcast_requested":false`)
	repo.AssertExpectations(t)
	ruleRepo.AssertExpectations(t)
	identityRepo.AssertExpectations(t)
}

func TestFingerprintHandler_DeleteAuditsAndSkipsBroadcast(t *testing.T) {
	repo := new(mockFingerprintRepo)
	ruleRepo := new(MockRoutingRuleRepo)
	auditRepo := new(MockManagementAuditRepo)
	h := NewFingerprintHandler(repo, ruleRepo, nil, nil, auditRepo)

	preset := &domain.FingerprintPreset{
		ID:   "p1",
		Name: testPresetName,
		Config: domain.ConfigMap{
			"api_token": "secret-token",
			"nested": map[string]any{
				"client_secret": "secret-value",
			},
			"user_agent": "Mozilla/5.0",
		},
	}
	repo.On("GetPreset", mock.Anything, "p1").Return(preset, nil)
	ruleRepo.On("ListActiveRulesReferencingFingerprintPreset", mock.Anything, "p1").Return([]domain.RoutingRuleReference{}, nil)
	repo.On("DeletePreset", mock.Anything, "p1").Return(nil)
	auditRepo.On("Create", mock.Anything, mock.MatchedBy(deleteAuditRedacted)).Return(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/fingerprints/p1?broadcast=false", nil)
	req.SetPathValue("id", "p1")
	rec := httptest.NewRecorder()

	h.HandleDeletePreset(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"deleted":true`)
	repo.AssertExpectations(t)
	ruleRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func deleteAuditRedacted(event *domain.ManagementAuditEvent) bool {
	oldValue, ok := event.OldValue.(*dto.FingerprintResponse)
	if !ok {
		return false
	}

	nested, ok := oldValue.Config["nested"].(map[string]any)
	if !ok {
		return false
	}

	return event.Action == domain.ActionDelete &&
		event.EntityID == "p1" &&
		oldValue.Config["api_token"] == redactedPlaceholder &&
		nested["client_secret"] == redactedPlaceholder &&
		oldValue.Config["user_agent"] == "Mozilla/5.0"
}
