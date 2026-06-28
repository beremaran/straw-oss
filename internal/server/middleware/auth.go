package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/auth"
)

type ContextApiKey struct {
	Value string
}

const (
	HeaderAuthorization string = "Authorization"
)

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

			ctx := context.WithValue(r.Context(), ContextApiKey{Value: "api_key"}, apiKey)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get(HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}

func GetAPIKey(r *http.Request) interface{} {
	return r.Context().Value(ContextApiKey{Value: "api_key"})
}
