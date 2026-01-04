package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/labstack/echo/v4"
)

const (
	HeaderAuthorization = "Authorization"
	ContextKeyAPIKey    = "api_key"
)

// AuthMiddleware creates a middleware that validates API keys using Bearer tokens.
func AuthMiddleware(validator auth.Validator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 1. Extract Token from Bearer header
			token := extractBearerToken(c.Request())
			if token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}

			// 2. Validate Token
			apiKey, err := validator.ValidateKey(c.Request().Context(), token)
			if err != nil {
				if errors.Is(err, auth.ErrInvalidKey) {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid bearer token")
				}
				// Internal error
				c.Logger().Errorf("failed to validate bearer token: %v", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "internal auth error")
			}

			// 3. Set in Context
			c.Set(ContextKeyAPIKey, apiKey)

			// 4. Continue
			return next(c)
		}
	}
}

func extractBearerToken(r *http.Request) string {
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
