package middleware

import (
	"net/http"
	"strings"

	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/labstack/echo/v4"
)

const (
	HeaderXAPIKey       = "X-API-Key"
	HeaderAuthorization = "Authorization"
	ContextKeyAPIKey    = "api_key"
)

// AuthMiddleware creates a middleware that validates API keys.
func AuthMiddleware(validator auth.Validator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 1. Extract Key
			key := extractKey(c.Request())
			if key == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing api key")
			}

			// 2. Validate Key
			apiKey, err := validator.ValidateKey(c.Request().Context(), key)
			if err != nil {
				if err == auth.ErrInvalidKey || err == auth.ErrInvalidKeyFormat {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid api key")
				}
				// Internal error
				c.Logger().Errorf("failed to validate api key: %v", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "internal auth error")
			}

			// 3. Set in Context
			c.Set(ContextKeyAPIKey, apiKey)

			// 4. Continue
			return next(c)
		}
	}
}

func extractKey(r *http.Request) string {
	// Try X-API-Key first
	if key := r.Header.Get(HeaderXAPIKey); key != "" {
		return key
	}

	// Try Authorization: Bearer <key>
	authHeader := r.Header.Get(HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}

// GetAPIKey retrieves the authenticated API key from the context.
// Helper for handlers.
func GetAPIKey(c echo.Context) interface{} {
	return c.Get(ContextKeyAPIKey)
}

// To get the actual type, handlers should assert:
// key := c.Get(ContextKeyAPIKey).(*domain.ApiKey)
