package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

// mockHealthStore is a mock implementation of redis.HealthStore for testing.
type mockHealthStore struct {
	endpoints map[string]*redis.EndpointHealth
	draining  map[string]bool
}

func newMockHealthStore() *mockHealthStore {
	return &mockHealthStore{
		endpoints: make(map[string]*redis.EndpointHealth),
		draining:  make(map[string]bool),
	}
}

func (m *mockHealthStore) UpdateHealth(_ context.Context, health *redis.EndpointHealth) error {
	m.endpoints[health.EndpointID] = health
	return nil
}

func (m *mockHealthStore) GetHealth(_ context.Context, endpointID string) (*redis.EndpointHealth, error) {
	if health, ok := m.endpoints[endpointID]; ok {
		return health, nil
	}
	return nil, redis.ErrCacheMiss
}

func (m *mockHealthStore) ListHealthyByTags(_ context.Context, tags []string) ([]*redis.EndpointHealth, error) {
	var result []*redis.EndpointHealth
	for _, health := range m.endpoints {
		if health.State == redis.HealthStateHealthy || health.State == redis.HealthStateSuspect {
			result = append(result, health)
		}
	}
	return result, nil
}

func (m *mockHealthStore) ListAllEndpoints(_ context.Context) ([]*redis.EndpointHealth, error) {
	result := make([]*redis.EndpointHealth, 0, len(m.endpoints))
	for _, health := range m.endpoints {
		result = append(result, health)
	}
	return result, nil
}

func (m *mockHealthStore) DeleteHealth(_ context.Context, endpointID string) error {
	delete(m.endpoints, endpointID)
	return nil
}

func (m *mockHealthStore) SetDraining(_ context.Context, endpointID string, draining bool) error {
	if draining {
		m.draining[endpointID] = true
	} else {
		delete(m.draining, endpointID)
	}
	return nil
}

func (m *mockHealthStore) IsDraining(_ context.Context, endpointID string) (bool, error) {
	return m.draining[endpointID], nil
}

func TestHealthService_HandleHeartbeat(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()

	// Create a heartbeat message
	msg := HeartbeatMessage{
		EndpointID:  "test-endpoint",
		Timestamp:   time.Now().Unix(),
		Version:     "1.0.0",
		Tags:        []string{"type:residential", "region:us"},
		ActiveTasks: 3,
	}

	data, _ := json.Marshal(msg)

	// Process heartbeat
	err := service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Fatalf("handleHeartbeat failed: %v", err)
	}

	// Verify endpoint was updated
	health, ok := store.endpoints["test-endpoint"]
	if !ok {
		t.Fatal("endpoint not found in store")
	}

	if health.State != redis.HealthStateHealthy {
		t.Errorf("expected state %s, got %s", redis.HealthStateHealthy, health.State)
	}
	if health.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", health.Version)
	}
	if health.ActiveTasks != 3 {
		t.Errorf("expected active_tasks 3, got %d", health.ActiveTasks)
	}
}

func TestHealthService_HandleHeartbeat_MalformedMessage(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()

	// Malformed JSON
	err := service.handleHeartbeat(ctx, []byte("not json"))
	if err != nil {
		t.Error("malformed JSON should not return error (just drop the message)")
	}

	// Empty endpoint ID
	msg := HeartbeatMessage{
		EndpointID: "",
		Timestamp:  time.Now().Unix(),
	}
	data, _ := json.Marshal(msg)

	err = service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Error("empty endpoint_id should not return error (just drop the message)")
	}

	// Verify no endpoints were stored
	if len(store.endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(store.endpoints))
	}
}

func TestHealthService_HandleHeartbeat_OldTimestamp(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()

	// Very old timestamp (should be replaced with current time)
	msg := HeartbeatMessage{
		EndpointID: "old-ts-endpoint",
		Timestamp:  time.Now().Add(-2 * time.Hour).Unix(),
	}
	data, _ := json.Marshal(msg)

	err := service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Fatalf("handleHeartbeat failed: %v", err)
	}

	health := store.endpoints["old-ts-endpoint"]
	if time.Since(health.LastSeen) > time.Minute {
		t.Error("expected LastSeen to be updated to current time for old timestamp")
	}
}

func TestHealthService_HandleHeartbeat_ZeroTimestamp(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()

	// Zero timestamp (should be replaced with current time)
	msg := HeartbeatMessage{
		EndpointID: "zero-ts-endpoint",
		Timestamp:  0,
	}
	data, _ := json.Marshal(msg)

	err := service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Fatalf("handleHeartbeat failed: %v", err)
	}

	health := store.endpoints["zero-ts-endpoint"]
	if time.Since(health.LastSeen) > time.Minute {
		t.Error("expected LastSeen to be updated to current time for zero timestamp")
	}
}

func TestHealthService_IsHealthyEndpoint(t *testing.T) {
	store := newMockHealthStore()
	store.endpoints["healthy"] = &redis.EndpointHealth{
		EndpointID: "healthy",
		State:      redis.HealthStateHealthy,
	}
	store.endpoints["suspect"] = &redis.EndpointHealth{
		EndpointID: "suspect",
		State:      redis.HealthStateSuspect,
	}
	store.endpoints["unhealthy"] = &redis.EndpointHealth{
		EndpointID: "unhealthy",
		State:      redis.HealthStateUnhealthy,
	}

	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()

	tests := []struct {
		endpointID string
		expected   bool
	}{
		{"healthy", true},
		{"suspect", true},
		{"unhealthy", false},
	}

	for _, tt := range tests {
		t.Run(tt.endpointID, func(t *testing.T) {
			isHealthy, err := service.IsEndpointHealthy(ctx, tt.endpointID)
			if err != nil {
				t.Fatalf("IsEndpointHealthy failed: %v", err)
			}
			if isHealthy != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, isHealthy)
			}
		})
	}
}

func TestHealthService_IsHealthyEndpoint_NotFound(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()

	_, err := service.IsEndpointHealthy(ctx, "nonexistent")
	if !errors.Is(err, redis.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
}

func TestHealthService_HandleHeartbeat_Draining(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()
	endpointID := "draining-endpoint"

	// Mark endpoint as draining
	_ = store.SetDraining(ctx, endpointID, true)

	// Send heartbeat
	msg := HeartbeatMessage{
		EndpointID:  endpointID,
		Timestamp:   time.Now().Unix(),
		ActiveTasks: 0,
	}
	data, _ := json.Marshal(msg)

	err := service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Fatalf("handleHeartbeat failed: %v", err)
	}

	// Verify endpoint state is draining
	health, ok := store.endpoints[endpointID]
	if !ok {
		t.Fatal("endpoint not found in store")
	}

	if health.State != redis.HealthStateDraining {
		t.Errorf("expected state %s, got %s", redis.HealthStateDraining, health.State)
	}
}

// mockBroker is a mock implementation of broker.MessageBroker for testing.
type mockBroker struct {
	subscribeHandler broker.Handler
	subscribeCalled  bool
	subscribeQueue   string
	publishCalled    bool
	closed           bool
}

func newMockBroker() *mockBroker {
	return &mockBroker{}
}

func (m *mockBroker) Publish(_ context.Context, exchange, routingKey string, body []byte) error {
	m.publishCalled = true
	return nil
}

func (m *mockBroker) Subscribe(ctx context.Context, queue string, handler broker.Handler) error {
	m.subscribeCalled = true
	m.subscribeQueue = queue
	m.subscribeHandler = handler
	return nil
}

func (m *mockBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
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

func (m *mockBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockBroker) Close() error {
	m.closed = true
	return nil
}

// TestWithQueue tests the WithQueue option function.
func TestWithQueue(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
		queue:  "default",
	}

	customQueue := "custom-heartbeats"
	opt := WithQueue(customQueue)
	opt(service)

	if service.queue != customQueue {
		t.Errorf("expected queue %s, got %s", customQueue, service.queue)
	}
}

// TestWithHealthLogger tests the WithHealthLogger option function.
func TestWithHealthLogger(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
		queue:  "heartbeats",
	}

	customLogger := slog.New(slog.NewTextHandler(nil, nil))
	opt := WithHealthLogger(customLogger)
	opt(service)

	if service.logger != customLogger {
		t.Error("expected custom logger to be set")
	}
}

// TestNewHealthService tests the NewHealthService constructor.
func TestNewHealthService(t *testing.T) {
	tests := []struct {
		name    string
		broker  broker.MessageBroker
		store   redis.HealthStore
		opts    []HealthOption
		wantErr bool
		verify  func(*testing.T, *HealthService)
	}{
		{
			name:    "default configuration",
			broker:  newMockBroker(),
			store:   newMockHealthStore(),
			opts:    nil,
			wantErr: false,
			verify: func(t *testing.T, s *HealthService) {
				if s.broker == nil {
					t.Error("expected broker to be set")
				}
				if s.store == nil {
					t.Error("expected store to be set")
				}
				if s.logger == nil {
					t.Error("expected logger to be set")
				}
				if s.queue != "heartbeats" {
					t.Errorf("expected default queue 'heartbeats', got %s", s.queue)
				}
			},
		},
		{
			name:    "with custom queue",
			broker:  newMockBroker(),
			store:   newMockHealthStore(),
			opts:    []HealthOption{WithQueue("custom-queue")},
			wantErr: false,
			verify: func(t *testing.T, s *HealthService) {
				if s.queue != "custom-queue" {
					t.Errorf("expected queue 'custom-queue', got %s", s.queue)
				}
			},
		},
		{
			name:    "with custom logger",
			broker:  newMockBroker(),
			store:   newMockHealthStore(),
			opts:    []HealthOption{WithHealthLogger(slog.New(slog.NewTextHandler(nil, nil)))},
			wantErr: false,
			verify: func(t *testing.T, s *HealthService) {
				if s.logger == nil {
					t.Error("expected custom logger to be set")
				}
			},
		},
		{
			name:   "with multiple options",
			broker: newMockBroker(),
			store:  newMockHealthStore(),
			opts: []HealthOption{
				WithQueue("multi-queue"),
				WithHealthLogger(slog.New(slog.NewTextHandler(nil, nil))),
			},
			wantErr: false,
			verify: func(t *testing.T, s *HealthService) {
				if s.queue != "multi-queue" {
					t.Errorf("expected queue 'multi-queue', got %s", s.queue)
				}
				if s.logger == nil {
					t.Error("expected custom logger to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewHealthService(tt.broker, tt.store, tt.opts...)
			if tt.verify != nil {
				tt.verify(t, service)
			}
		})
	}
}

// TestHealthService_StartStop tests the Start and Stop lifecycle methods.
func TestHealthService_StartStop(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*HealthService)
		testFunc func(*testing.T, *HealthService)
	}{
		{
			name:  "start and stop service",
			setup: func(s *HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				ctx := context.Background()

				// Start the service
				err := s.Start(ctx)
				if err != nil {
					t.Fatalf("Start failed: %v", err)
				}

				// Check if running
				if !s.IsRunning() {
					t.Error("expected service to be running after Start")
				}

				// Stop the service
				s.Stop()

				// Check if stopped
				if s.IsRunning() {
					t.Error("expected service to be stopped after Stop")
				}
			},
		},
		{
			name:  "start already running service",
			setup: func(s *HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				ctx := context.Background()

				// First start
				err := s.Start(ctx)
				if err != nil {
					t.Fatalf("first Start failed: %v", err)
				}

				// Second start should be idempotent
				err = s.Start(ctx)
				if err != nil {
					t.Errorf("second Start should not return error, got: %v", err)
				}

				s.Stop()
			},
		},
		{
			name:  "stop not running service",
			setup: func(s *HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				// Stop without starting should not panic
				s.Stop()

				if s.IsRunning() {
					t.Error("expected service to not be running")
				}
			},
		},
		{
			name:  "multiple start stop cycles",
			setup: func(s *HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				ctx := context.Background()

				for i := 0; i < 3; i++ {
					err := s.Start(ctx)
					if err != nil {
						t.Fatalf("Start failed on cycle %d: %v", i, err)
					}

					if !s.IsRunning() {
						t.Errorf("expected service to be running on cycle %d", i)
					}

					s.Stop()

					if s.IsRunning() {
						t.Errorf("expected service to be stopped on cycle %d", i)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := newMockBroker()
			store := newMockHealthStore()
			service := NewHealthService(mb, store)
			tt.setup(service)
			tt.testFunc(t, service)
		})
	}
}

// TestHealthService_IsRunning tests the IsRunning method.
func TestHealthService_IsRunning(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*HealthService)
		expected bool
	}{
		{
			name:     "not running initially",
			setup:    func(s *HealthService) {},
			expected: false,
		},
		{
			name: "running after start",
			setup: func(s *HealthService) {
				ctx := context.Background()
				s.Start(ctx)
			},
			expected: true,
		},
		{
			name: "not running after stop",
			setup: func(s *HealthService) {
				ctx := context.Background()
				s.Start(ctx)
				s.Stop()
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := newMockBroker()
			store := newMockHealthStore()
			service := NewHealthService(mb, store)
			tt.setup(service)

			if got := service.IsRunning(); got != tt.expected {
				t.Errorf("IsRunning() = %v, want %v", got, tt.expected)
			}

			// Cleanup
			if service.IsRunning() {
				service.Stop()
			}
		})
	}
}

// TestHealthService_run tests the run method behavior.
func TestHealthService_run(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*mockBroker, *mockHealthStore)
		verify func(*testing.T, *HealthService, *mockBroker)
	}{
		{
			name: "run subscribes to correct queue",
			setup: func(mb *mockBroker, store *mockHealthStore) {
				// No special setup needed
			},
			verify: func(t *testing.T, s *HealthService, mb *mockBroker) {
				if !mb.subscribeCalled {
					t.Error("expected Subscribe to be called")
				}
				if mb.subscribeQueue != "heartbeats" {
					t.Errorf("expected Subscribe to be called with queue 'heartbeats', got %s", mb.subscribeQueue)
				}
			},
		},
		{
			name: "run uses custom queue",
			setup: func(mb *mockBroker, store *mockHealthStore) {
				// Custom queue will be set in service creation
			},
			verify: func(t *testing.T, s *HealthService, mb *mockBroker) {
				if mb.subscribeQueue != "custom-queue" {
					t.Errorf("expected Subscribe to be called with queue 'custom-queue', got %s", mb.subscribeQueue)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := newMockBroker()
			store := newMockHealthStore()
			var service *HealthService

			if tt.name == "run uses custom queue" {
				service = NewHealthService(mb, store, WithQueue("custom-queue"))
			} else {
				service = NewHealthService(mb, store)
			}

			if tt.setup != nil {
				tt.setup(mb, store)
			}

			ctx := context.Background()

			// Start the service (which calls run internally)
			err := service.Start(ctx)
			if err != nil {
				t.Fatalf("Start failed: %v", err)
			}

			// Give it a moment to start
			time.Sleep(10 * time.Millisecond)

			// Stop the service
			service.Stop()

			// Wait a bit for cleanup
			time.Sleep(10 * time.Millisecond)

			if tt.verify != nil {
				tt.verify(t, service, mb)
			}
		})
	}
}

// TestHealthService_GetHealthyEndpoints tests the GetHealthyEndpoints method.
func TestHealthService_GetHealthyEndpoints(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockHealthStore)
		tags    []string
		wantLen int
		wantErr bool
	}{
		{
			name: "returns healthy endpoints",
			setup: func(store *mockHealthStore) {
				store.endpoints["healthy1"] = &redis.EndpointHealth{
					EndpointID: "healthy1",
					State:      redis.HealthStateHealthy,
					Tags:       []string{"type:residential"},
				}
				store.endpoints["healthy2"] = &redis.EndpointHealth{
					EndpointID: "healthy2",
					State:      redis.HealthStateHealthy,
					Tags:       []string{"type:datacenter"},
				}
				store.endpoints["unhealthy"] = &redis.EndpointHealth{
					EndpointID: "unhealthy",
					State:      redis.HealthStateUnhealthy,
				}
			},
			tags:    []string{"type:residential"},
			wantLen: 2, // Both healthy and suspect are returned
			wantErr: false,
		},
		{
			name: "returns suspect endpoints",
			setup: func(store *mockHealthStore) {
				store.endpoints["suspect"] = &redis.EndpointHealth{
					EndpointID: "suspect",
					State:      redis.HealthStateSuspect,
				}
			},
			tags:    []string{},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "excludes unhealthy endpoints",
			setup: func(store *mockHealthStore) {
				store.endpoints["unhealthy1"] = &redis.EndpointHealth{
					EndpointID: "unhealthy1",
					State:      redis.HealthStateUnhealthy,
				}
				store.endpoints["unhealthy2"] = &redis.EndpointHealth{
					EndpointID: "unhealthy2",
					State:      redis.HealthStateUnhealthy,
				}
			},
			tags:    []string{},
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "empty store returns empty list",
			setup:   func(store *mockHealthStore) {},
			tags:    []string{},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "mixed states",
			setup: func(store *mockHealthStore) {
				store.endpoints["h1"] = &redis.EndpointHealth{
					EndpointID: "h1",
					State:      redis.HealthStateHealthy,
				}
				store.endpoints["s1"] = &redis.EndpointHealth{
					EndpointID: "s1",
					State:      redis.HealthStateSuspect,
				}
				store.endpoints["u1"] = &redis.EndpointHealth{
					EndpointID: "u1",
					State:      redis.HealthStateUnhealthy,
				}
				store.endpoints["d1"] = &redis.EndpointHealth{
					EndpointID: "d1",
					State:      redis.HealthStateDraining,
				}
			},
			tags:    []string{},
			wantLen: 2, // Only healthy and suspect
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockHealthStore()
			tt.setup(store)

			service := NewHealthService(nil, store)

			ctx := context.Background()
			endpoints, err := service.GetHealthyEndpoints(ctx, tt.tags)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetHealthyEndpoints() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(endpoints) != tt.wantLen {
				t.Errorf("GetHealthyEndpoints() returned %d endpoints, want %d", len(endpoints), tt.wantLen)
			}

			// Verify all returned endpoints are healthy or suspect
			for _, ep := range endpoints {
				if ep.State != redis.HealthStateHealthy && ep.State != redis.HealthStateSuspect {
					t.Errorf("GetHealthyEndpoints() returned endpoint %s with state %s", ep.EndpointID, ep.State)
				}
			}
		})
	}
}

// TestHealthService_GetEndpointHealth tests the GetEndpointHealth method.
func TestHealthService_GetEndpointHealth(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*mockHealthStore)
		endpointID string
		wantState  string
		wantErr    bool
	}{
		{
			name: "returns existing endpoint health",
			setup: func(store *mockHealthStore) {
				store.endpoints["ep1"] = &redis.EndpointHealth{
					EndpointID:  "ep1",
					State:       redis.HealthStateHealthy,
					LastSeen:    time.Now(),
					Version:     "1.0.0",
					ActiveTasks: 5,
				}
			},
			endpointID: "ep1",
			wantState:  redis.HealthStateHealthy,
			wantErr:    false,
		},
		{
			name:       "returns error for non-existent endpoint",
			setup:      func(store *mockHealthStore) {},
			endpointID: "nonexistent",
			wantErr:    true,
		},
		{
			name: "returns unhealthy endpoint",
			setup: func(store *mockHealthStore) {
				store.endpoints["ep2"] = &redis.EndpointHealth{
					EndpointID: "ep2",
					State:      redis.HealthStateUnhealthy,
					LastSeen:   time.Now().Add(-1 * time.Hour),
				}
			},
			endpointID: "ep2",
			wantState:  redis.HealthStateUnhealthy,
			wantErr:    false,
		},
		{
			name: "returns suspect endpoint",
			setup: func(store *mockHealthStore) {
				store.endpoints["ep3"] = &redis.EndpointHealth{
					EndpointID: "ep3",
					State:      redis.HealthStateSuspect,
					LastSeen:   time.Now().Add(-20 * time.Second),
				}
			},
			endpointID: "ep3",
			wantState:  redis.HealthStateSuspect,
			wantErr:    false,
		},
		{
			name: "returns draining endpoint",
			setup: func(store *mockHealthStore) {
				store.endpoints["ep4"] = &redis.EndpointHealth{
					EndpointID: "ep4",
					State:      redis.HealthStateDraining,
					LastSeen:   time.Now(),
				}
			},
			endpointID: "ep4",
			wantState:  redis.HealthStateDraining,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockHealthStore()
			tt.setup(store)

			service := NewHealthService(nil, store)

			ctx := context.Background()
			health, err := service.GetEndpointHealth(ctx, tt.endpointID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetEndpointHealth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if health == nil {
					t.Fatal("GetEndpointHealth() returned nil health")
				}
				if health.State != tt.wantState {
					t.Errorf("GetEndpointHealth() state = %s, want %s", health.State, tt.wantState)
				}
				if health.EndpointID != tt.endpointID {
					t.Errorf("GetEndpointHealth() endpointID = %s, want %s", health.EndpointID, tt.endpointID)
				}
			}
		})
	}
}

// TestHealthService_DrainEndpoint tests the DrainEndpoint method.
func TestHealthService_DrainEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*mockHealthStore)
		endpointID string
		wantErr    bool
		verify     func(*testing.T, *mockHealthStore)
	}{
		{
			name: "marks endpoint as draining",
			setup: func(store *mockHealthStore) {
				store.endpoints["ep1"] = &redis.EndpointHealth{
					EndpointID: "ep1",
					State:      redis.HealthStateHealthy,
				}
			},
			endpointID: "ep1",
			wantErr:    false,
			verify: func(t *testing.T, store *mockHealthStore) {
				isDraining, err := store.IsDraining(context.Background(), "ep1")
				if err != nil {
					t.Fatalf("IsDraining failed: %v", err)
				}
				if !isDraining {
					t.Error("expected endpoint to be marked as draining")
				}
			},
		},
		{
			name:       "can drain non-existent endpoint",
			setup:      func(store *mockHealthStore) {},
			endpointID: "nonexistent",
			wantErr:    false,
			verify: func(t *testing.T, store *mockHealthStore) {
				isDraining, err := store.IsDraining(context.Background(), "nonexistent")
				if err != nil {
					t.Fatalf("IsDraining failed: %v", err)
				}
				if !isDraining {
					t.Error("expected endpoint to be marked as draining")
				}
			},
		},
		{
			name: "can drain already draining endpoint",
			setup: func(store *mockHealthStore) {
				store.draining["ep2"] = true
			},
			endpointID: "ep2",
			wantErr:    false,
			verify: func(t *testing.T, store *mockHealthStore) {
				isDraining, err := store.IsDraining(context.Background(), "ep2")
				if err != nil {
					t.Fatalf("IsDraining failed: %v", err)
				}
				if !isDraining {
					t.Error("expected endpoint to be marked as draining")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockHealthStore()
			tt.setup(store)

			service := NewHealthService(nil, store)

			ctx := context.Background()
			err := service.DrainEndpoint(ctx, tt.endpointID)

			if (err != nil) != tt.wantErr {
				t.Errorf("DrainEndpoint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.verify != nil {
				tt.verify(t, store)
			}
		})
	}
}

// TestHealthService_ListAllEndpoints tests the ListAllEndpoints method.
func TestHealthService_ListAllEndpoints(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockHealthStore)
		wantLen int
		wantErr bool
	}{
		{
			name: "returns all endpoints",
			setup: func(store *mockHealthStore) {
				store.endpoints["ep1"] = &redis.EndpointHealth{
					EndpointID: "ep1",
					State:      redis.HealthStateHealthy,
				}
				store.endpoints["ep2"] = &redis.EndpointHealth{
					EndpointID: "ep2",
					State:      redis.HealthStateSuspect,
				}
				store.endpoints["ep3"] = &redis.EndpointHealth{
					EndpointID: "ep3",
					State:      redis.HealthStateUnhealthy,
				}
			},
			wantLen: 3,
			wantErr: false,
		},
		{
			name:    "empty store returns empty list",
			setup:   func(store *mockHealthStore) {},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "includes all states",
			setup: func(store *mockHealthStore) {
				store.endpoints["h"] = &redis.EndpointHealth{
					EndpointID: "h",
					State:      redis.HealthStateHealthy,
				}
				store.endpoints["s"] = &redis.EndpointHealth{
					EndpointID: "s",
					State:      redis.HealthStateSuspect,
				}
				store.endpoints["u"] = &redis.EndpointHealth{
					EndpointID: "u",
					State:      redis.HealthStateUnhealthy,
				}
				store.endpoints["d"] = &redis.EndpointHealth{
					EndpointID: "d",
					State:      redis.HealthStateDraining,
				}
			},
			wantLen: 4,
			wantErr: false,
		},
		{
			name: "returns endpoints with metadata",
			setup: func(store *mockHealthStore) {
				store.endpoints["ep1"] = &redis.EndpointHealth{
					EndpointID:  "ep1",
					State:       redis.HealthStateHealthy,
					Tags:        []string{"type:residential", "region:us"},
					Version:     "1.2.3",
					ActiveTasks: 10,
					LastSeen:    time.Now(),
				}
			},
			wantLen: 1,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockHealthStore()
			tt.setup(store)

			service := NewHealthService(nil, store)

			ctx := context.Background()
			endpoints, err := service.ListAllEndpoints(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListAllEndpoints() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(endpoints) != tt.wantLen {
				t.Errorf("ListAllEndpoints() returned %d endpoints, want %d", len(endpoints), tt.wantLen)
			}

			// Verify all returned endpoints have required fields
			for _, ep := range endpoints {
				if ep.EndpointID == "" {
					t.Error("ListAllEndpoints() returned endpoint with empty EndpointID")
				}
			}
		})
	}
}

// errorMockHealthStore is a mock that can return errors.
type errorMockHealthStore struct {
	*mockHealthStore
	updateHealthError error
	isDrainingError   error
}

func (m *errorMockHealthStore) UpdateHealth(ctx context.Context, health *redis.EndpointHealth) error {
	if m.updateHealthError != nil {
		return m.updateHealthError
	}
	return m.mockHealthStore.UpdateHealth(ctx, health)
}

func (m *errorMockHealthStore) IsDraining(ctx context.Context, endpointID string) (bool, error) {
	if m.isDrainingError != nil {
		return false, m.isDrainingError
	}
	return m.mockHealthStore.IsDraining(ctx, endpointID)
}

// TestHealthService_HandleHeartbeat_StoreError tests error handling in handleHeartbeat.
func TestHealthService_HandleHeartbeat_StoreError(t *testing.T) {
	// Create a mock store that returns an error
	errorStore := &errorMockHealthStore{
		mockHealthStore:   newMockHealthStore(),
		updateHealthError: redis.ErrCacheMiss,
	}

	service := &HealthService{
		store:  errorStore,
		logger: slog.Default(),
	}

	ctx := context.Background()

	msg := HeartbeatMessage{
		EndpointID: "error-endpoint",
		Timestamp:  time.Now().Unix(),
	}
	data, _ := json.Marshal(msg)

	err := service.handleHeartbeat(ctx, data)
	if err == nil {
		t.Error("handleHeartbeat should return error when store fails")
	}
}

// TestHealthService_HandleHeartbeat_DrainingCheckError tests when draining check fails.
func TestHealthService_HandleHeartbeat_DrainingCheckError(t *testing.T) {
	// Create a mock store that returns error on IsDraining
	errorStore := &errorMockHealthStore{
		mockHealthStore: newMockHealthStore(),
		isDrainingError: redis.ErrCacheMiss,
	}

	service := &HealthService{
		store:  errorStore,
		logger: slog.Default(),
	}

	ctx := context.Background()

	msg := HeartbeatMessage{
		EndpointID: "draining-error-endpoint",
		Timestamp:  time.Now().Unix(),
	}
	data, _ := json.Marshal(msg)

	// Should not return error, just continue without draining state
	err := service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Errorf("handleHeartbeat should not return error when draining check fails, got: %v", err)
	}
}
