package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server/helper"
)

func KeyAuth(cfg config.ServerConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				helper.WriteError(w, http.StatusUnauthorized, "missing management API token")

				return
			}

			receivedKey := strings.TrimPrefix(authHeader, "Bearer ")
			if receivedKey == "" {
				helper.WriteError(w, http.StatusUnauthorized, "missing management API token")

				return
			}

			if subtle.ConstantTimeCompare([]byte(receivedKey), []byte(cfg.ManagementAPIKey)) != 1 {
				helper.WriteError(w, http.StatusUnauthorized, "invalid management API token")

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
