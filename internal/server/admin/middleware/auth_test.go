package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestKeyAuth(t *testing.T) {
	e := echo.New()
	cfg := config.ServerConfig{AdminAPIKey: "secret-key"}
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
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			h := mw(func(c echo.Context) error {
				return c.String(http.StatusOK, "test")
			})

			err := h(c)
			if tt.wantStatus != http.StatusOK {
				if assert.Error(t, err) {
					he, ok := err.(*echo.HTTPError)
					if assert.True(t, ok) {
						assert.Equal(t, tt.wantStatus, he.Code)
					}
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, rec.Code)
			}
		})
	}
}
