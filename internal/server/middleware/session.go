package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/session"
)

// Session header constants used for session management.
const (
	HeaderSessionID               = "X-Session-ID"
	HeaderSessionEnd              = "X-Session-End"
	HeaderSessionMigrated         = "X-Session-Migrated"
	HeaderSessionMigrationCount   = "X-Session-Migration-Count"
	HeaderSessionPreviousEndpoint = "X-Session-Previous-Endpoint"
)

type sessionContextKey string

// SessionContextKey is the context key for storing the session.
const SessionContextKey sessionContextKey = "session"

func processExistingSession(w http.ResponseWriter, r *http.Request, service *session.Service, sessionID string) (*http.Request, bool) {
	ctx := r.Context()

	sess, err := service.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, domain.ErrSessionExpired) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = w.Header().Get("X-Request-ID")
			}

			helper.WriteJSON(w, domain.ErrSessionExpired.HTTPCode, domain.ErrSessionExpired.ToResponse(requestID, ""))

			return r, true
		}

		return r, false
	}

	_ = service.TouchSession(ctx, sessionID)
	r = r.WithContext(context.WithValue(ctx, SessionContextKey, sess))
	w.Header().Set(HeaderSessionID, sess.ID)

	if sess.MigrationCount > 0 {
		w.Header().Set(HeaderSessionMigrationCount, strconv.Itoa(sess.MigrationCount))
	}

	return r, false
}

// SessionMiddleware manages session lifecycle, including validation and termination.
func SessionMiddleware(service *session.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sessionID := r.Header.Get(HeaderSessionID)

			if r.Header.Get(HeaderSessionEnd) == "true" {
				if sessionID != "" {
					_ = service.EndSession(ctx, sessionID)
				}

				next.ServeHTTP(w, r)

				return
			}

			if sessionID != "" {
				var shouldReturn bool

				r, shouldReturn = processExistingSession(w, r, service, sessionID)
				if shouldReturn {
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetSessionFromContext retrieves the session from the request context.
func GetSessionFromContext(ctx context.Context) *domain.Session {
	sess, ok := ctx.Value(SessionContextKey).(*domain.Session)
	if !ok {
		return nil
	}

	return sess
}
