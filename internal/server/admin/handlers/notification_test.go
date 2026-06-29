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
)

const (
	testNotificationChannelID = "channel-1"
	testNotificationUserID    = "2df3db5e-31f8-4f29-9bb3-64aeb33c78b6"
)

type MockNotificationChannelRepo struct {
	mock.Mock
}

func (m *MockNotificationChannelRepo) Create(ctx context.Context, channel *domain.NotificationChannel) error {
	args := m.Called(ctx, channel)

	return args.Error(0)
}

func (m *MockNotificationChannelRepo) Update(ctx context.Context, channel *domain.NotificationChannel) error {
	args := m.Called(ctx, channel)

	return args.Error(0)
}

func (m *MockNotificationChannelRepo) Disable(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

func (m *MockNotificationChannelRepo) GetByID(ctx context.Context, id string) (*domain.NotificationChannel, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.NotificationChannel), args.Error(1)
}

func (m *MockNotificationChannelRepo) List(ctx context.Context, limit, offset int) ([]domain.NotificationChannel, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.NotificationChannel), args.Int(1), args.Error(2)
}

type MockNotificationPreferenceRepo struct {
	mock.Mock
}

func (m *MockNotificationPreferenceRepo) ListByUserID(ctx context.Context, userID string) ([]domain.NotificationPreference, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.NotificationPreference), args.Error(1)
}

func (m *MockNotificationPreferenceRepo) ReplaceForUser(ctx context.Context, userID string, preferences []domain.NotificationPreference) error {
	args := m.Called(ctx, userID, preferences)

	return args.Error(0)
}

func TestNotificationHandler_HandleCreateChannelRedactsSecretAndAudits(t *testing.T) {
	channelRepo := new(MockNotificationChannelRepo)
	prefRepo := new(MockNotificationPreferenceRepo)
	auditRepo := new(MockManagementAuditRepo)
	handler := NewNotificationHandler(channelRepo, prefRepo, auditRepo, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/notification-channels", bytes.NewBufferString(`{
		"name": "Ops webhook",
		"type": "webhook",
		"config": {"team": "ops", "webhook_url": "https://secret.test"},
		"secret": "https://secret.test"
	}`))
	rec := httptest.NewRecorder()

	channelRepo.On("Create", mock.Anything, mock.MatchedBy(func(channel *domain.NotificationChannel) bool {
		_, leaked := channel.Config["webhook_url"]

		return channel.Name == "Ops webhook" && channel.SecretRef != "" && !leaked
	})).Return(nil).Once()
	auditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		return event.Action == domain.ActionCreate && event.EntityType == "notification_channel"
	})).Return(nil).Once()

	handler.HandleCreateChannel(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["has_secret"])
	assert.NotContains(t, rec.Body.String(), "https://secret.test")
	channelRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestNotificationHandler_HandleTestChannelPostsWebhook(t *testing.T) {
	received := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	channelRepo := new(MockNotificationChannelRepo)
	handler := NewNotificationHandler(channelRepo, new(MockNotificationPreferenceRepo), nil, nil)
	channel := testNotificationChannel()
	channel.SecretRef = server.URL

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/notification-channels/channel-1/test", nil)
	req.SetPathValue("id", testNotificationChannelID)
	rec := httptest.NewRecorder()

	channelRepo.On("GetByID", mock.Anything, testNotificationChannelID).Return(channel, nil).Once()

	handler.HandleTestChannel(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, received)
	assert.Contains(t, rec.Body.String(), `"success":true`)
	channelRepo.AssertExpectations(t)
}

func TestNotificationHandler_PreferencesUseCurrentUser(t *testing.T) {
	channelRepo := new(MockNotificationChannelRepo)
	prefRepo := new(MockNotificationPreferenceRepo)
	handler := NewNotificationHandler(channelRepo, prefRepo, nil, nil)

	ctx := middleware.ContextWithActor(context.Background(), middleware.Actor{
		Type: middleware.ActorTypeUser,
		ID:   testNotificationUserID,
	})
	prefs := []domain.NotificationPreference{
		{ID: "pref-1", UserID: testNotificationUserID, EventType: "alert_fired", ChannelID: testNotificationChannelID, IsEnabled: true},
	}

	getReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/management/notification-preferences", nil)
	getRec := httptest.NewRecorder()
	prefRepo.On("ListByUserID", mock.Anything, testNotificationUserID).Return(prefs, nil).Once()
	handler.HandleGetPreferences(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code)

	updateReq := httptest.NewRequestWithContext(ctx, http.MethodPatch, "/management/notification-preferences", bytes.NewBufferString(`{
		"preferences": [{"event_type":"alert_fired","channel_id":"channel-1","is_enabled":false}]
	}`))
	updateRec := httptest.NewRecorder()
	prefRepo.On("ReplaceForUser", mock.Anything, testNotificationUserID, mock.MatchedBy(func(preferences []domain.NotificationPreference) bool {
		return len(preferences) == 1 && !preferences[0].IsEnabled
	})).Return(nil).Once()
	handler.HandleUpdatePreferences(updateRec, updateReq)
	require.Equal(t, http.StatusOK, updateRec.Code)

	prefRepo.AssertExpectations(t)
	channelRepo.AssertNotCalled(t, "GetByID")
}

func testNotificationChannel() *domain.NotificationChannel {
	now := time.Now().UTC()

	return &domain.NotificationChannel{
		ID:        testNotificationChannelID,
		Name:      "Ops webhook",
		Type:      domain.NotificationChannelWebhook,
		Config:    domain.ConfigMap{"team": "ops"},
		SecretRef: "https://" + "example.test",
		IsEnabled: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
