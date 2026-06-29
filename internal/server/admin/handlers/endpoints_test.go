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
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/service/endpoint"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/stretchr/testify/assert"
)

type mockHealthStore struct {
	endpoints map[string]*redis.EndpointHealth
	draining  map[string]bool
	deleted   map[string]bool
}

func (m *mockHealthStore) UpdateHealth(ctx context.Context, health *redis.EndpointHealth) error {
	m.endpoints[health.EndpointID] = health

	return nil
}

func (m *mockHealthStore) GetHealth(ctx context.Context, endpointID string) (*redis.EndpointHealth, error) {
	if h, ok := m.endpoints[endpointID]; ok {
		return h, nil
	}

	return nil, redis.ErrCacheMiss
}

func (m *mockHealthStore) ListHealthyByTags(ctx context.Context, tags []string) ([]*redis.EndpointHealth, error) {
	return nil, nil
}

func (m *mockHealthStore) ListAllEndpoints(ctx context.Context) ([]*redis.EndpointHealth, error) {
	var list []*redis.EndpointHealth
	for _, h := range m.endpoints {
		list = append(list, h)
	}

	return list, nil
}

func (m *mockHealthStore) DeleteHealth(ctx context.Context, endpointID string) error {
	delete(m.endpoints, endpointID)

	return nil
}

func (m *mockHealthStore) SetDraining(ctx context.Context, endpointID string, draining bool) error {
	if draining {
		m.draining[endpointID] = true
	} else {
		delete(m.draining, endpointID)
	}

	return nil
}

func (m *mockHealthStore) IsDraining(ctx context.Context, endpointID string) (bool, error) {
	return m.draining[endpointID], nil
}

func (m *mockHealthStore) SetDeleted(ctx context.Context, endpointID string, deleted bool) error {
	if deleted {
		m.deleted[endpointID] = true
	} else {
		delete(m.deleted, endpointID)
	}

	return nil
}

func (m *mockHealthStore) IsDeleted(ctx context.Context, endpointID string) (bool, error) {
	return m.deleted[endpointID], nil
}

type mockEndpointRepo struct {
	endpoints map[string]*domain.Endpoint
}

func (m *mockEndpointRepo) Create(ctx context.Context, ep *domain.Endpoint) error {
	m.endpoints[ep.ID] = ep

	return nil
}

func (m *mockEndpointRepo) GetByID(ctx context.Context, id string) (*domain.Endpoint, error) {
	if ep, ok := m.endpoints[id]; ok {
		return ep, nil
	}

	return nil, nil
}

func (m *mockEndpointRepo) Update(ctx context.Context, ep *domain.Endpoint) error {
	m.endpoints[ep.ID] = ep

	return nil
}

func (m *mockEndpointRepo) Delete(ctx context.Context, id string) error {
	if ep, ok := m.endpoints[id]; ok {
		now := time.Now().UTC()
		ep.DeletedAt = &now
		ep.DesiredState = domain.DesiredStateDeleted
		ep.IsRegistered = false

		return nil
	}

	return postgres.ErrEndpointNotFound
}

func (m *mockEndpointRepo) List(ctx context.Context, limit, offset int, includeDeleted bool) ([]domain.Endpoint, int, error) {
	var list []domain.Endpoint
	for _, ep := range m.endpoints {
		if !includeDeleted && ep.DeletedAt != nil {
			continue
		}

		list = append(list, *ep)
	}

	return list, len(list), nil
}

type mockCommandRepo struct {
	commands map[string]*domain.EndpointCommand
}

func (m *mockCommandRepo) Create(ctx context.Context, cmd *domain.EndpointCommand) error {
	m.commands[cmd.ID] = cmd

	return nil
}

func (m *mockCommandRepo) GetByID(ctx context.Context, id string) (*domain.EndpointCommand, error) {
	if cmd, ok := m.commands[id]; ok {
		return cmd, nil
	}

	return nil, nil
}

func (m *mockCommandRepo) Update(ctx context.Context, cmd *domain.EndpointCommand) error {
	m.commands[cmd.ID] = cmd

	return nil
}

func (m *mockCommandRepo) ListByEndpointID(ctx context.Context, endpointID string, limit, offset int) ([]domain.EndpointCommand, int, error) {
	var list []domain.EndpointCommand
	for _, cmd := range m.commands {
		if cmd.EndpointID == endpointID {
			list = append(list, *cmd)
		}
	}

	return list, len(list), nil
}

func (m *mockCommandRepo) ListPending(ctx context.Context, before time.Time) ([]domain.EndpointCommand, error) {
	var list []domain.EndpointCommand
	for _, cmd := range m.commands {
		if (cmd.Status == domain.CommandStatusAccepted ||
			cmd.Status == domain.CommandStatusAcknowledged ||
			cmd.Status == domain.CommandStatusRunning) && cmd.RequestedAt.Before(before) {
			list = append(list, *cmd)
		}
	}

	return list, nil
}

type mockEndpointBroker struct {
	published map[string][]byte
}

func (m *mockEndpointBroker) Publish(ctx context.Context, subject string, body []byte) error {
	m.published[subject] = body

	return nil
}

func (m *mockEndpointBroker) Subscribe(ctx context.Context, subject string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	return nil
}

func (m *mockEndpointBroker) ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockEndpointBroker) DeclareStream(ctx context.Context, name string, subjects ...string) error {
	return nil
}

func (m *mockEndpointBroker) Close() error {
	return nil
}

func (m *mockEndpointBroker) IsConnected() bool {
	return true
}

func TestEndpointHandler_List_LegacyFallback(t *testing.T) {
	store := &mockHealthStore{
		endpoints: map[string]*redis.EndpointHealth{
			"ep1": {EndpointID: "ep1", State: "healthy"},
			"ep2": {EndpointID: "ep2", State: "unhealthy"},
		},
	}
	healthService := endpoint.NewHealthService(nil, store)

	h := NewEndpointHandler(healthService, nil, nil, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/endpoints", nil)
	rec := httptest.NewRecorder()

	h.HandleListEndpoints(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var endpoints []dto.EndpointHealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &endpoints)
	assert.NoError(t, err)
	assert.Len(t, endpoints, 2)
}

func TestEndpointHandler_Lifecycle(t *testing.T) {
	store := &mockHealthStore{
		endpoints: map[string]*redis.EndpointHealth{
			"ep-1": {EndpointID: "ep-1", State: "healthy", LastSeen: time.Now()},
		},
		draining: make(map[string]bool),
		deleted:  make(map[string]bool),
	}
	healthService := endpoint.NewHealthService(nil, store)
	epRepo := &mockEndpointRepo{endpoints: make(map[string]*domain.Endpoint)}
	cmdRepo := &mockCommandRepo{commands: make(map[string]*domain.EndpointCommand)}
	mb := &mockEndpointBroker{published: make(map[string][]byte)}

	h := NewEndpointHandler(healthService, epRepo, cmdRepo, mb, nil)

	// 1. Create Endpoint
	reqBody := `{"id":"ep-1","tags":["type:residential","region:us"],"desired_state":"active"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/endpoints", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()
	h.HandleCreateEndpoint(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var createResp dto.EndpointResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	assert.Equal(t, "ep-1", createResp.ID)
	assert.Equal(t, "active", createResp.DesiredState)
	assert.Equal(t, "healthy", createResp.Health.State)

	// 2. Get Endpoint
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/endpoints/ep-1", nil)
	req.SetPathValue("id", "ep-1")
	rec = httptest.NewRecorder()
	h.HandleGetEndpoint(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var getResp dto.EndpointResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	assert.Equal(t, "ep-1", getResp.ID)

	// 3. Drain Endpoint
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/endpoints/ep-1/drain", nil)
	req.SetPathValue("id", "ep-1")
	rec = httptest.NewRecorder()
	h.HandleDrainEndpoint(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var drainResp dto.EndpointDrainResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &drainResp))
	assert.Equal(t, "ep-1", drainResp.EndpointID)
	assert.Equal(t, "draining", drainResp.DesiredState)
	assert.NotEmpty(t, drainResp.CommandID)
	assert.True(t, store.draining["ep-1"])

	// Check NATS publish
	assert.NotEmpty(t, mb.published["endpoint.control.ep-1"])

	// 4. Undrain Endpoint
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/endpoints/ep-1/undrain", nil)
	req.SetPathValue("id", "ep-1")
	rec = httptest.NewRecorder()
	h.HandleUndrainEndpoint(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, store.draining["ep-1"])

	// 5. Restart Endpoint
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/endpoints/ep-1/restart", nil)
	req.SetPathValue("id", "ep-1")
	rec = httptest.NewRecorder()
	h.HandleRestartEndpoint(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 6. Delete Endpoint
	req = httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/endpoints/ep-1", nil)
	req.SetPathValue("id", "ep-1")
	rec = httptest.NewRecorder()
	h.HandleDeleteEndpoint(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, store.deleted["ep-1"])

	// 7. Get Command detail
	var cmdID string
	for k := range cmdRepo.commands {
		cmdID = k

		break
	}
	assert.NotEmpty(t, cmdID)

	// Test Command Detail dispatch
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/endpoints/commands/"+cmdID, nil)
	req.SetPathValue("segment3", "commands")
	req.SetPathValue("segment4", cmdID)
	rec = httptest.NewRecorder()
	h.HandleCommandDispatch(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var cmdResp dto.EndpointCommandDTO
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cmdResp))
	assert.Equal(t, cmdID, cmdResp.ID)
	assert.Equal(t, "ep-1", cmdResp.EndpointID)

	// Test Command List dispatch
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/endpoints/ep-1/commands", nil)
	req.SetPathValue("segment3", "ep-1")
	req.SetPathValue("segment4", "commands")
	rec = httptest.NewRecorder()
	h.HandleCommandDispatch(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var cmdListResp dto.EndpointCommandListResponse
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cmdListResp))
	assert.NotEmpty(t, cmdListResp.Data)
}
