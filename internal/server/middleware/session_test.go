package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/server/middleware"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Re-using mock store from service test is hard across packages unless exported.
// I will implement a quick inline mock or move mock to a test package.
// For simplicity, I'll just re-implement a minimal mock here.

type mockStore struct {
	sessions map[string]*domain.Session
}

func newMockStore() *mockStore {
	return &mockStore{sessions: make(map[string]*domain.Session)}
}
func (m *mockStore) Save(ctx context.Context, s *domain.Session, ttl time.Duration) error {
	m.sessions[s.ID] = s
	return nil
}
func (m *mockStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, domain.ErrSessionExpired
	}
	return s, nil
}
func (m *mockStore) Delete(ctx context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}
func (m *mockStore) Touch(ctx context.Context, id string, ttl time.Duration) error {
	if _, ok := m.sessions[id]; !ok {
		return domain.ErrSessionExpired
	}
	return nil
}

func TestSessionMiddleware(t *testing.T) {
	store := newMockStore()
	svc := session.NewService(store)
	e := echo.New()

	// Pre-create a session
	ctx := context.Background()
	sess, err := svc.CreateSession(ctx, "ep1", "rule1", nil)
	require.NoError(t, err)

	mw := middleware.SessionMiddleware(svc)

	t.Run("Existing Session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(middleware.HeaderSessionID, sess.ID)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		h := mw(func(c echo.Context) error {
			// Check context
			extracted := middleware.GetSessionFromContext(c.Request().Context())
			assert.NotNil(t, extracted)
			if extracted != nil {
				assert.Equal(t, sess.ID, extracted.ID)
			}
			return c.NoContent(200)
		})

		assert.NoError(t, h(c))
		assert.Equal(t, sess.ID, rec.Header().Get(middleware.HeaderSessionID))
	})

	t.Run("Missing Session ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		h := mw(func(c echo.Context) error {
			extracted := middleware.GetSessionFromContext(c.Request().Context())
			assert.Nil(t, extracted)
			return c.NoContent(200)
		})

		assert.NoError(t, h(c))
		assert.Empty(t, rec.Header().Get(middleware.HeaderSessionID))
	})

	t.Run("Invalid Session ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(middleware.HeaderSessionID, "invalid-123")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		h := mw(func(c echo.Context) error {
			return c.NoContent(200) // Should not reach here
		})

		// Expect 410 Gone (Session Expired error mapping)
		// Or whatever default error handler does with it.
		// Wait, in middleware I returned `c.JSON(410, ...)`
		err := h(c)
		assert.NoError(t, err) // Handler returns error? No, c.JSON returns nil usually.
		assert.Equal(t, 410, rec.Code)
	})

	t.Run("End Session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(middleware.HeaderSessionID, sess.ID)
		req.Header.Set(middleware.HeaderSessionEnd, "true")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		h := mw(func(c echo.Context) error {
			return c.NoContent(200)
		})

		assert.NoError(t, h(c))

		// Verify deleted
		_, err := store.Get(ctx, sess.ID)
		assert.ErrorIs(t, err, domain.ErrSessionExpired)
	})
}
