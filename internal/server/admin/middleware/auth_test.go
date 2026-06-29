package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/beremaran/straw/internal/config"
)

func TestKeyAuth(t *testing.T) {
	cfg := config.ServerConfig{
		ManagementAPIKey: "secret-key",
	}
	mw := KeyAuth(cfg)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer secret-key",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid bearer token",
			authHeader: "Bearer wrong-key",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing authorization header",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong auth type",
			authHeader: "Basic abc123",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer without token",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.wantStatus == http.StatusOK {
					actor, ok := ActorFromContext(r.Context())
					assert.True(t, ok)
					assert.Equal(t, "system:legacy-admin", actor.ID)
					assert.True(t, actor.HasPermission(PermissionAPIKeysRead))
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("test"))
			}))

			h.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestKeyAuth_PreResolvedActor(t *testing.T) {
	mw := KeyAuth(config.ServerConfig{ManagementLegacyTokenDisabled: true})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req = req.WithContext(ContextWithActor(req.Context(), Actor{
		Type:        ActorTypeUser,
		ID:          testUserID,
		DisplayName: "User One",
		SessionID:   "session-1",
		Permissions: []string{PermissionUsageRead},
	}))
	rec := httptest.NewRecorder()

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := ActorFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, testUserID, actor.ID)
		w.WriteHeader(http.StatusOK)
	}))

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestKeyAuth_LegacyDisabled(t *testing.T) {
	mw := KeyAuth(config.ServerConfig{
		ManagementAPIKey:              "secret-key",
		ManagementLegacyTokenDisabled: true,
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequirePermission(t *testing.T) {
	tests := []struct {
		name       string
		actor      Actor
		withActor  bool
		wantStatus int
	}{
		{
			name:       "missing actor",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing permission",
			actor: Actor{
				ID:          testUserID,
				Permissions: []string{PermissionUsageRead},
			},
			withActor:  true,
			wantStatus: http.StatusForbidden,
		},
		{
			name: "has permission",
			actor: Actor{
				ID:          testUserID,
				Permissions: []string{PermissionAPIKeysRead},
			},
			withActor:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "legacy wildcard",
			actor:      LegacyAdminActor(),
			withActor:  true,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			if tt.withActor {
				req = req.WithContext(ContextWithActor(req.Context(), tt.actor))
			}
			rec := httptest.NewRecorder()

			h := RequirePermission(PermissionAPIKeysRead)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			h.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
