package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestKeyAuth(t *testing.T) {
	cfg := config.ServerConfig{ManagementAPIKey: "secret-key"}
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
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("test"))
			}))

			h.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
