package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/pkg/broker"
)

const (
	testVersion         = "1.0.0"
	testTypeResidential = "type:residential"
	testSuspectID       = "suspect"
	testUnhealthyID     = "unhealthy"
	testEp1ID           = "ep1"
	testEp2ID           = "ep2"
	testEp3ID           = "ep3"
)

type mockHealthStore struct {
	endpoints map[string]*redis.EndpointHealth
	draining  map[string]bool
	deleted   map[string]bool
}

func newMockHealthStore() *mockHealthStore {
	return &mockHealthStore{
		endpoints: make(map[string]*redis.EndpointHealth),
		draining:  make(map[string]bool),
		deleted:   make(map[string]bool),
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

func (m *mockHealthStore) ListHealthyByTags(_ context.Context, _ []string) ([]*redis.EndpointHealth, error) {
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

func (m *mockHealthStore) SetDeleted(_ context.Context, endpointID string, deleted bool) error {
	if deleted {
		m.deleted[endpointID] = true
	} else {
		delete(m.deleted, endpointID)
	}

	return nil
}

func (m *mockHealthStore) IsDeleted(_ context.Context, endpointID string) (bool, error) {
	return m.deleted[endpointID], nil
}

func TestHealthService_HandleHeartbeat(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:  store,
		logger: slog.Default(),
	}

	ctx := context.Background()

	msg := HeartbeatMessage{
		EndpointID:  "test-endpoint",
		Timestamp:   time.Now().Unix(),
		Version:     testVersion,
		Tags:        []string{testTypeResidential, "region:us"},
		ActiveTasks: 3,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	err = service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Fatalf("handleHeartbeat failed: %v", err)
	}

	health, ok := store.endpoints["test-endpoint"]
	if !ok {
		t.Fatal("endpoint not found in store")
	}

	if health.State != redis.HealthStateHealthy {
		t.Errorf("expected state %s, got %s", redis.HealthStateHealthy, health.State)
	}
	if health.Version != testVersion {
		t.Errorf("expected version %s, got %s", testVersion, health.Version)
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

	err := service.handleHeartbeat(ctx, []byte("not json"))
	if err != nil {
		t.Error("malformed JSON should not return error (just drop the message)")
	}

	msg := HeartbeatMessage{
		EndpointID: "",
		Timestamp:  time.Now().Unix(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	err = service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Error("empty endpoint_id should not return error (just drop the message)")
	}

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

	msg := HeartbeatMessage{
		EndpointID: "old-ts-endpoint",
		Timestamp:  time.Now().Add(-2 * time.Hour).Unix(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	err = service.handleHeartbeat(ctx, data)
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

	msg := HeartbeatMessage{
		EndpointID: "zero-ts-endpoint",
		Timestamp:  0,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	err = service.handleHeartbeat(ctx, data)
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
	store.endpoints[testSuspectID] = &redis.EndpointHealth{
		EndpointID: testSuspectID,
		State:      redis.HealthStateSuspect,
	}
	store.endpoints[testUnhealthyID] = &redis.EndpointHealth{
		EndpointID: testUnhealthyID,
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
		{testSuspectID, true},
		{testUnhealthyID, false},
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

	_ = store.SetDraining(ctx, endpointID, true)

	msg := HeartbeatMessage{
		EndpointID:  endpointID,
		Timestamp:   time.Now().Unix(),
		ActiveTasks: 0,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	err = service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Fatalf("handleHeartbeat failed: %v", err)
	}

	health, ok := store.endpoints[endpointID]
	if !ok {
		t.Fatal("endpoint not found in store")
	}

	if health.State != redis.HealthStateDraining {
		t.Errorf("expected state %s, got %s", redis.HealthStateDraining, health.State)
	}
}

type mockBroker struct {
	subscribeHandler broker.Handler
	subscribeCalled  bool
	subscribeSubject string
	publishCalled    bool
	closed           bool
}

func newMockBroker() *mockBroker {
	return &mockBroker{}
}

func (m *mockBroker) Publish(_ context.Context, _ string, _ []byte) error {
	m.publishCalled = true

	return nil
}

func (m *mockBroker) Subscribe(_ context.Context, subject string, handler broker.Handler, _ ...broker.SubscribeOption) error {
	m.subscribeCalled = true
	m.subscribeSubject = subject
	m.subscribeHandler = handler

	return nil
}

func (m *mockBroker) DeclareStream(_ context.Context, _ string, _ ...string) error {
	return nil
}

func (m *mockBroker) IsConnected() bool {
	return true
}

func (m *mockBroker) ConsumeOnce(_ context.Context, _ string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (m *mockBroker) Close() error {
	m.closed = true

	return nil
}

func TestWithHeartbeatSubject(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:            store,
		logger:           slog.Default(),
		heartbeatSubject: "default",
	}

	customSubject := "custom-heartbeats"
	opt := WithHeartbeatSubject(customSubject)
	opt(service)

	if service.heartbeatSubject != customSubject {
		t.Errorf("expected subject %s, got %s", customSubject, service.heartbeatSubject)
	}
}

func TestWithHealthLogger(t *testing.T) {
	store := newMockHealthStore()
	service := &HealthService{
		store:            store,
		logger:           slog.Default(),
		heartbeatSubject: defaultHeartbeatSubject,
	}

	customLogger := slog.New(slog.NewTextHandler(nil, nil))
	opt := WithHealthLogger(customLogger)
	opt(service)

	if service.logger != customLogger {
		t.Error("expected custom logger to be set")
	}
}

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
				if s.heartbeatSubject != defaultHeartbeatSubject {
					t.Errorf("expected default subject %q, got %s", defaultHeartbeatSubject, s.heartbeatSubject)
				}
			},
		},
		{
			name:    "with custom subject",
			broker:  newMockBroker(),
			store:   newMockHealthStore(),
			opts:    []HealthOption{WithHeartbeatSubject("custom-queue")},
			wantErr: false,
			verify: func(t *testing.T, s *HealthService) {
				if s.heartbeatSubject != "custom-queue" {
					t.Errorf("expected subject 'custom-queue', got %s", s.heartbeatSubject)
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
				WithHeartbeatSubject("multi-queue"),
				WithHealthLogger(slog.New(slog.NewTextHandler(nil, nil))),
			},
			wantErr: false,
			verify: func(t *testing.T, s *HealthService) {
				if s.heartbeatSubject != "multi-queue" {
					t.Errorf("expected subject 'multi-queue', got %s", s.heartbeatSubject)
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

func TestHealthService_StartStop(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*HealthService)
		testFunc func(*testing.T, *HealthService)
	}{
		{
			name:  "start and stop service",
			setup: func(*HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				ctx := context.Background()

				err := s.Start(ctx)
				if err != nil {
					t.Fatalf("Start failed: %v", err)
				}

				if !s.IsRunning() {
					t.Error("expected service to be running after Start")
				}

				s.Stop()

				if s.IsRunning() {
					t.Error("expected service to be stopped after Stop")
				}
			},
		},
		{
			name:  "start already running service",
			setup: func(*HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				ctx := context.Background()

				err := s.Start(ctx)
				if err != nil {
					t.Fatalf("first Start failed: %v", err)
				}

				err = s.Start(ctx)
				if err != nil {
					t.Errorf("second Start should not return error, got: %v", err)
				}

				s.Stop()
			},
		},
		{
			name:  "stop not running service",
			setup: func(*HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				s.Stop()

				if s.IsRunning() {
					t.Error("expected service to not be running")
				}
			},
		},
		{
			name:  "multiple start stop cycles",
			setup: func(*HealthService) {},
			testFunc: func(t *testing.T, s *HealthService) {
				ctx := context.Background()

				for i := range 3 {
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

func TestHealthService_IsRunning(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*HealthService)
		expected bool
	}{
		{
			name:     "not running initially",
			setup:    func(*HealthService) {},
			expected: false,
		},
		{
			name: "running after start",
			setup: func(s *HealthService) {
				ctx := context.Background()
				err := s.Start(ctx)
				if err != nil {
					t.Fatal(err)
				}
			},
			expected: true,
		},
		{
			name: "not running after stop",
			setup: func(s *HealthService) {
				ctx := context.Background()
				err := s.Start(ctx)
				if err != nil {
					t.Fatal(err)
				}
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

			if service.IsRunning() {
				service.Stop()
			}
		})
	}
}

func TestHealthService_run(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*mockBroker, *mockHealthStore)
		verify func(*testing.T, *HealthService, *mockBroker)
	}{
		{
			name: "run subscribes to correct subject",
			setup: func(*mockBroker, *mockHealthStore) {
			},
			verify: func(t *testing.T, _ *HealthService, mb *mockBroker) {
				if !mb.subscribeCalled {
					t.Error("expected Subscribe to be called")
				}
				if mb.subscribeSubject != defaultHeartbeatSubject {
					t.Errorf("expected Subscribe to be called with subject %q, got %s", defaultHeartbeatSubject, mb.subscribeSubject)
				}
			},
		},
		{
			name: "run uses custom subject",
			setup: func(*mockBroker, *mockHealthStore) {
			},
			verify: func(t *testing.T, _ *HealthService, mb *mockBroker) {
				if mb.subscribeSubject != "custom-queue" {
					t.Errorf("expected Subscribe to be called with subject %q, got %s", "custom-queue", mb.subscribeSubject)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := newMockBroker()
			store := newMockHealthStore()
			var service *HealthService

			if tt.name == "run uses custom subject" {
				service = NewHealthService(mb, store, WithHeartbeatSubject("custom-queue"))
			} else {
				service = NewHealthService(mb, store)
			}

			if tt.setup != nil {
				tt.setup(mb, store)
			}

			ctx := context.Background()

			err := service.Start(ctx)
			if err != nil {
				t.Fatalf("Start failed: %v", err)
			}

			time.Sleep(10 * time.Millisecond)

			service.Stop()

			time.Sleep(10 * time.Millisecond)

			if tt.verify != nil {
				tt.verify(t, service, mb)
			}
		})
	}
}

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
					Tags:       []string{testTypeResidential},
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
			wantLen: 2,
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
			setup:   func(*mockHealthStore) {},
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
			wantLen: 2,
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

			for _, ep := range endpoints {
				if ep.State != redis.HealthStateHealthy && ep.State != redis.HealthStateSuspect {
					t.Errorf("GetHealthyEndpoints() returned endpoint %s with state %s", ep.EndpointID, ep.State)
				}
			}
		})
	}
}

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
				store.endpoints[testEp1ID] = &redis.EndpointHealth{
					EndpointID:  testEp1ID,
					State:       redis.HealthStateHealthy,
					LastSeen:    time.Now(),
					Version:     testVersion,
					ActiveTasks: 5,
				}
			},
			endpointID: testEp1ID,
			wantState:  redis.HealthStateHealthy,
			wantErr:    false,
		},
		{
			name:       "returns error for non-existent endpoint",
			setup:      func(*mockHealthStore) {},
			endpointID: "nonexistent",
			wantErr:    true,
		},
		{
			name: "returns unhealthy endpoint",
			setup: func(store *mockHealthStore) {
				store.endpoints[testEp2ID] = &redis.EndpointHealth{
					EndpointID: testEp2ID,
					State:      redis.HealthStateUnhealthy,
					LastSeen:   time.Now().Add(-1 * time.Hour),
				}
			},
			endpointID: testEp2ID,
			wantState:  redis.HealthStateUnhealthy,
			wantErr:    false,
		},
		{
			name: "returns suspect endpoint",
			setup: func(store *mockHealthStore) {
				store.endpoints[testEp3ID] = &redis.EndpointHealth{
					EndpointID: testEp3ID,
					State:      redis.HealthStateSuspect,
					LastSeen:   time.Now().Add(-20 * time.Second),
				}
			},
			endpointID: testEp3ID,
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
				store.endpoints[testEp1ID] = &redis.EndpointHealth{
					EndpointID: testEp1ID,
					State:      redis.HealthStateHealthy,
				}
			},
			endpointID: testEp1ID,
			wantErr:    false,
			verify: func(t *testing.T, store *mockHealthStore) {
				isDraining, err := store.IsDraining(context.Background(), testEp1ID)
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
			setup:      func(*mockHealthStore) {},
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
				store.draining[testEp2ID] = true
			},
			endpointID: testEp2ID,
			wantErr:    false,
			verify: func(t *testing.T, store *mockHealthStore) {
				isDraining, err := store.IsDraining(context.Background(), testEp2ID)
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
				store.endpoints[testEp1ID] = &redis.EndpointHealth{
					EndpointID: testEp1ID,
					State:      redis.HealthStateHealthy,
				}
				store.endpoints[testEp2ID] = &redis.EndpointHealth{
					EndpointID: testEp2ID,
					State:      redis.HealthStateSuspect,
				}
				store.endpoints[testEp3ID] = &redis.EndpointHealth{
					EndpointID: testEp3ID,
					State:      redis.HealthStateUnhealthy,
				}
			},
			wantLen: 3,
			wantErr: false,
		},
		{
			name:    "empty store returns empty list",
			setup:   func(*mockHealthStore) {},
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
					Tags:        []string{testTypeResidential, "region:us"},
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

			for _, ep := range endpoints {
				if ep.EndpointID == "" {
					t.Error("ListAllEndpoints() returned endpoint with empty EndpointID")
				}
			}
		})
	}
}

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

func TestHealthService_HandleHeartbeat_StoreError(t *testing.T) {
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
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	err = service.handleHeartbeat(ctx, data)
	if err == nil {
		t.Error("handleHeartbeat should return error when store fails")
	}
}

func TestHealthService_HandleHeartbeat_DrainingCheckError(t *testing.T) {
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
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	err = service.handleHeartbeat(ctx, data)
	if err != nil {
		t.Errorf("handleHeartbeat should not return error when draining check fails, got: %v", err)
	}
}
