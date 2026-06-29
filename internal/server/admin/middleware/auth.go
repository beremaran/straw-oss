package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server/helper"
	adminauth "github.com/beremaran/straw/internal/service/auth"
)

// AccessTokenVerifier verifies access tokens.
type AccessTokenVerifier interface {
	VerifyAccessToken(ctx context.Context, rawToken string) (*adminauth.AccessClaims, error)
}

// SessionAuth authenticates requests using an access token from the Authorization header.
func SessionAuth(verifier AccessTokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if verifier == nil {
				next.ServeHTTP(w, r)

				return
			}

			if _, ok := ActorFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)

				return
			}

			rawToken := bearerToken(r.Header.Get("Authorization"))
			if rawToken == "" {
				next.ServeHTTP(w, r)

				return
			}

			claims, err := verifier.VerifyAccessToken(r.Context(), rawToken)
			if err != nil {
				if looksLikeAccessToken(rawToken) {
					if !errors.Is(err, adminauth.ErrInvalidAccessToken) {
						helper.WriteError(w, http.StatusInternalServerError, "failed to authenticate")

						return
					}

					helper.WriteError(w, http.StatusUnauthorized, "invalid access token")

					return
				}

				next.ServeHTTP(w, r)

				return
			}

			next.ServeHTTP(w, r.WithContext(ContextWithActor(r.Context(), Actor{
				Type:        ActorTypeUser,
				ID:          claims.UserID,
				Email:       claims.Email,
				DisplayName: claims.DisplayName,
				SessionID:   claims.SessionID,
				Permissions: claims.Permissions,
			})))
		})
	}
}

// KeyAuth authenticates requests using a management API key from the Authorization header.
func KeyAuth(cfg config.ServerConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := ActorFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)

				return
			}

			if cfg.ManagementLegacyTokenDisabled {
				helper.WriteError(w, http.StatusUnauthorized, "management API token auth disabled")

				return
			}

			receivedKey := bearerToken(r.Header.Get("Authorization"))
			if receivedKey == "" {
				helper.WriteError(w, http.StatusUnauthorized, "missing management API token")

				return
			}

			if subtle.ConstantTimeCompare([]byte(receivedKey), []byte(cfg.ManagementAPIKey)) != 1 {
				helper.WriteError(w, http.StatusUnauthorized, "invalid management API token")

				return
			}

			next.ServeHTTP(w, r.WithContext(ContextWithActor(r.Context(), LegacyAdminActor())))
		})
	}
}

func bearerToken(header string) string {
	if header == "" || !strings.HasPrefix(header, "Bearer ") {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func looksLikeAccessToken(token string) bool {
	return strings.HasPrefix(token, "eyJ") && strings.Count(token, ".") == 2
}

// RequirePermission returns a middleware that enforces the given permission.
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := ActorFromContext(r.Context())
			if !ok {
				helper.WriteError(w, http.StatusUnauthorized, "missing management actor")

				return
			}

			if !actor.HasPermission(permission) {
				helper.WriteError(w, http.StatusForbidden, "missing permission")

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
