package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/labstack/echo/v4"
)

// KeyAuth returns a middleware that validates the Admin API Key.
func KeyAuth(cfg config.ServerConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			receivedKey := c.Request().Header.Get("X-Admin-Key")
			if receivedKey == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing admin key")
			}

			// Constant time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(receivedKey), []byte(cfg.AdminAPIKey)) != 1 {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid admin key")
			}

			return next(c)
		}
	}
}
