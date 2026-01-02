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
		headerKey  string
		wantStatus int
	}{
		{
			name:       "valid key",
			headerKey:  "secret-key",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid key",
			headerKey:  "wrong-key",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing key",
			headerKey:  "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.headerKey != "" {
				req.Header.Set("X-Admin-Key", tt.headerKey)
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
