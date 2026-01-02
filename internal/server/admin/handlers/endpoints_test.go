package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/service/endpoint"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// mockHealthStore implements redis.HealthStore
type mockHealthStore struct {
	endpoints map[string]*redis.EndpointHealth
	draining  map[string]bool
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

func TestEndpointHandler_List(t *testing.T) {
	// Setup
	store := &mockHealthStore{
		endpoints: map[string]*redis.EndpointHealth{
			"ep1": {EndpointID: "ep1", State: "healthy"},
			"ep2": {EndpointID: "ep2", State: "unhealthy"},
		},
	}
	// We need a real HealthService but with mock store.
	// We can't use NewHealthService effectively because it requires real broker?
	// NewHealthService(broker, store, opts...)
	// But ListAllEndpoints only uses store.
	healthService := endpoint.NewHealthService(nil, store)

	h := NewEndpointHandler(healthService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/endpoints", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Test
	err := h.HandleListEndpoints(c)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var endpoints []*redis.EndpointHealth
	err = json.Unmarshal(rec.Body.Bytes(), &endpoints)
	assert.NoError(t, err)
	assert.Len(t, endpoints, 2)
}

func TestEndpointHandler_Drain(t *testing.T) {
	// Setup
	store := &mockHealthStore{
		endpoints: map[string]*redis.EndpointHealth{
			"ep1": {EndpointID: "ep1", State: "healthy"},
		},
		draining: make(map[string]bool),
	}
	healthService := endpoint.NewHealthService(nil, store)
	h := NewEndpointHandler(healthService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/endpoints/ep1/drain", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ep1")

	// Test
	err := h.HandleDrainEndpoint(c)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	assert.True(t, store.draining["ep1"])
}
