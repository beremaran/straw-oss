package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/labstack/echo/v4"
)

// KeyAuth returns a middleware that validates the Admin API Key using Bearer token.
func KeyAuth(cfg config.ServerConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing admin token")
			}

			receivedKey := strings.TrimPrefix(authHeader, "Bearer ")
			if receivedKey == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing admin token")
			}

			// Constant time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(receivedKey), []byte(cfg.AdminAPIKey)) != 1 {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid admin token")
			}

			return next(c)
		}
	}
}
