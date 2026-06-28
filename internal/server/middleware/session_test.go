package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/middleware"
	"github.com/beremaran/straw/internal/service/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionMiddleware(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client, err := redis.NewClient(config.RedisConfig{Addr: mr.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	store := session.NewRedisStore(client)
	svc := session.NewService(store)

	ctx := context.Background()
	sess, err := svc.CreateSession(ctx, "ep1", "rule1", nil)
	require.NoError(t, err)

	mw := middleware.SessionMiddleware(svc)

	t.Run("Existing Session", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set(middleware.HeaderSessionID, sess.ID)
		rec := httptest.NewRecorder()

		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			extracted := middleware.GetSessionFromContext(r.Context())
			assert.NotNil(t, extracted)
			if extracted != nil {
				assert.Equal(t, sess.ID, extracted.ID)
			}
			w.WriteHeader(http.StatusOK)
		}))

		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, sess.ID, rec.Header().Get(middleware.HeaderSessionID))
	})

	t.Run("Missing Session ID", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			extracted := middleware.GetSessionFromContext(r.Context())
			assert.Nil(t, extracted)
			w.WriteHeader(http.StatusOK)
		}))

		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get(middleware.HeaderSessionID))
	})

	t.Run("Invalid Session ID", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set(middleware.HeaderSessionID, "invalid-123")
		rec := httptest.NewRecorder()

		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		h.ServeHTTP(rec, req)
		assert.Equal(t, 410, rec.Code)
	})

	t.Run("End Session", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set(middleware.HeaderSessionID, sess.ID)
		req.Header.Set(middleware.HeaderSessionEnd, "true")
		rec := httptest.NewRecorder()

		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		h.ServeHTTP(rec, req)

		_, err := store.Get(ctx, sess.ID)
		assert.ErrorIs(t, err, domain.ErrSessionExpired)
	})
}
