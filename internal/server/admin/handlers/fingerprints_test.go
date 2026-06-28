package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

func (m *mockBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	args := m.Called(ctx, exchange, routingKey, body)

	return args.Error(0)
}
func (m *mockBroker) Subscribe(ctx context.Context, queue string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	return nil
}
func (m *mockBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
	return nil
}
func (m *mockBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	return nil, nil
}
func (m *mockBroker) Close() error {
	return nil
}
func (m *mockBroker) DeclareExchange(ctx context.Context, name, kind string) error {
	return nil
}
func (m *mockBroker) DeclareQueue(ctx context.Context, name string) error {
	return nil
}
func (m *mockBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	return nil
}

func (m *mockBroker) IsConnected() bool {
	return true
}

func (m *mockBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	return 0, nil
}

func TestFingerprintHandler_List(t *testing.T) {
	repo := new(mockFingerprintRepo)
	mb := new(mockBroker)
	h := NewFingerprintHandler(repo, mb)

	presets := []domain.FingerprintPreset{
		{ID: "p1", Name: "Preset 1"},
	}
	repo.On("ListPresets", mock.Anything).Return(presets, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin/fingerprints", nil)
	rec := httptest.NewRecorder()

	h.HandleListPresets(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	repo.AssertExpectations(t)
}

func TestFingerprintHandler_Create(t *testing.T) {
	repo := new(mockFingerprintRepo)
	mb := new(mockBroker)
	h := NewFingerprintHandler(repo, mb)

	preset := domain.FingerprintPreset{ID: "p1", Name: "Preset 1", Config: domain.ConfigMap{"a": 1}}
	body, err := json.Marshal(preset)
	assert.NoError(t, err)

	repo.On("GetPreset", mock.Anything, "p1").Return((*domain.FingerprintPreset)(nil), nil)
	repo.On("CreatePreset", mock.Anything, mock.MatchedBy(func(p *domain.FingerprintPreset) bool {
		return p.ID == "p1"
	})).Return(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/fingerprints", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleCreatePreset(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	repo.AssertExpectations(t)
}

func TestFingerprintHandler_Broadcast(t *testing.T) {
	repo := new(mockFingerprintRepo)
	mb := new(mockBroker)
	h := NewFingerprintHandler(repo, mb)

	presets := []domain.FingerprintPreset{{ID: "p1"}}
	repo.On("ListPresets", mock.Anything).Return(presets, nil)
	mb.On("Publish", mock.Anything, "fingerprint_broadcast", "", mock.Anything).Return(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/admin/fingerprints/broadcast", nil)
	rec := httptest.NewRecorder()

	h.HandleBroadcastPresets(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	repo.AssertExpectations(t)
	mb.AssertExpectations(t)
}
