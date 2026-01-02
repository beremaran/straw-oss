package session_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/stretchr/testify/assert"
)

type mockStore struct {
	sessions      map[string]*domain.Session
	ttl           map[string]time.Duration
	saveError     error
	getError      error
	deleteError   error
	touchError    error
	forceGetError bool
}

func newMockStore() *mockStore {
	return &mockStore{
		sessions: make(map[string]*domain.Session),
		ttl:      make(map[string]time.Duration),
	}
}

func (m *mockStore) Save(ctx context.Context, s *domain.Session, ttl time.Duration) error {
	if m.saveError != nil {
		return m.saveError
	}
	m.sessions[s.ID] = s
	m.ttl[s.ID] = ttl
	return nil
}

func (m *mockStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	if m.forceGetError {
		return nil, fmt.Errorf("forced get error")
	}
	s, ok := m.sessions[id]
	if !ok {
		return nil, domain.ErrSessionExpired
	}
	return s, nil
}

func (m *mockStore) Delete(ctx context.Context, id string) error {
	if m.deleteError != nil {
		return m.deleteError
	}
	delete(m.sessions, id)
	delete(m.ttl, id)
	return nil
}

func (m *mockStore) Touch(ctx context.Context, id string, ttl time.Duration) error {
	if m.touchError != nil {
		return m.touchError
	}
	if _, ok := m.sessions[id]; !ok {
		return domain.ErrSessionExpired
	}
	m.ttl[id] = ttl
	return nil
}

func TestService_CreateSession(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	sess, err := svc.CreateSession(ctx, "ep1", "rule1", []string{"tag1"})
	assert.NoError(t, err)
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "ep1", sess.EndpointID)
}

func TestService_GetSession(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	// Valid get
	got, err := svc.GetSession(ctx, sess.ID)
	assert.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)

	// Expired check (Session struct has LastUsedAt, Service checks logic expiry)
	sess.LastUsedAt = time.Now().Add(-20 * time.Minute) // Older than default TTL (10m)
	store.sessions[sess.ID] = sess

	got, err = svc.GetSession(ctx, sess.ID)
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_MigrateSession(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	// Migrate 1
	updated, err := svc.MigrateSession(ctx, sess.ID, "ep2")
	assert.NoError(t, err)
	assert.Equal(t, "ep2", updated.EndpointID)
	assert.Equal(t, 1, updated.MigrationCount)

	// Migrate 2
	updated, err = svc.MigrateSession(ctx, sess.ID, "ep3")
	assert.NoError(t, err)
	assert.Equal(t, 2, updated.MigrationCount)

	// Migrate 3
	updated, err = svc.MigrateSession(ctx, sess.ID, "ep4")
	assert.NoError(t, err)
	assert.Equal(t, 3, updated.MigrationCount)

	// Migrate 4 -> Fail (Max 3)
	_, err = svc.MigrateSession(ctx, sess.ID, "ep5")
	assert.ErrorIs(t, err, domain.ErrSessionMigrationLimit)
}

func TestService_CreateSession_StoreError(t *testing.T) {
	store := newMockStore()
	store.saveError = fmt.Errorf("store save error")
	svc := session.NewService(store)
	ctx := context.Background()

	_, err := svc.CreateSession(ctx, "ep1", "rule1", []string{"tag1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store save error")
}

func TestService_GetSession_StoreError(t *testing.T) {
	store := newMockStore()
	store.forceGetError = true
	svc := session.NewService(store)
	ctx := context.Background()

	_, err := svc.GetSession(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forced get error")
}

func TestService_GetSession_NonExistent(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	_, err := svc.GetSession(ctx, "nonexistent")
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_TouchSession(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	// Touch existing session
	err := svc.TouchSession(ctx, sess.ID)
	assert.NoError(t, err)

	// Touch non-existent session
	err = svc.TouchSession(ctx, "nonexistent")
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_TouchSession_StoreError(t *testing.T) {
	store := newMockStore()
	store.touchError = fmt.Errorf("touch error")
	svc := session.NewService(store)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	err := svc.TouchSession(ctx, sess.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "touch error")
}

func TestService_EndSession(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	// End existing session
	err := svc.EndSession(ctx, sess.ID)
	assert.NoError(t, err)

	// Verify session is deleted
	_, err = svc.GetSession(ctx, sess.ID)
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_EndSession_StoreError(t *testing.T) {
	store := newMockStore()
	store.deleteError = fmt.Errorf("delete error")
	svc := session.NewService(store)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	err := svc.EndSession(ctx, sess.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete error")
}

func TestService_MigrateSession_NonExistent(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	_, err := svc.MigrateSession(ctx, "nonexistent", "ep2")
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

func TestService_MigrateSession_StoreSaveError(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, "ep1", "rule1", nil)

	// Set save error after session is created
	store.saveError = fmt.Errorf("save error")

	_, err := svc.MigrateSession(ctx, sess.ID, "ep2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "save error")
}

func TestService_CreateSession_WithTags(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	ctx := context.Background()

	tags := []string{"tag1", "tag2", "tag3"}
	sess, err := svc.CreateSession(ctx, "ep1", "rule1", tags)
	assert.NoError(t, err)
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "ep1", sess.EndpointID)
	assert.Equal(t, "rule1", sess.RuleID)
	assert.Equal(t, tags, sess.Tags)
	assert.Equal(t, 0, sess.MigrationCount)
	assert.Equal(t, 0, sess.RequestCount)
	assert.False(t, sess.CreatedAt.IsZero())
	assert.False(t, sess.LastUsedAt.IsZero())
}
