// Package middleware provides HTTP middleware functions for authentication,
// rate limiting, session management, and request handling.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/auth"
)

// ContextAPIKey is the context key type for storing the authenticated API key.
type ContextAPIKey struct {
	Value string
}

const (
	// HeaderAuthorization is the HTTP header name for the Authorization header.
	HeaderAuthorization string = "Authorization"

	// APIKeyContextKey is the context value key for the API key.
	APIKeyContextKey = "api_key"
)

// AuthMiddleware validates bearer tokens and stores the authenticated API key in the request context.
func AuthMiddleware(validator *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				helper.WriteError(w, http.StatusUnauthorized, "missing bearer token")

				return
			}

			apiKey, err := validator.ValidateKey(r.Context(), token)
			if err != nil {
				if errors.Is(err, auth.ErrInvalidKey) {
					helper.WriteError(w, http.StatusUnauthorized, "invalid bearer token")

					return
				}

				helper.WriteError(w, http.StatusInternalServerError, "internal auth error")

				return
			}

			ctx := context.WithValue(r.Context(), ContextAPIKey{Value: APIKeyContextKey}, apiKey)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get(HeaderAuthorization)
	if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
		return after
	}

	return ""
}

// GetAPIKey retrieves the authenticated API key from the request context.
func GetAPIKey(r *http.Request) any {
	return r.Context().Value(ContextAPIKey{Value: APIKeyContextKey})
}
