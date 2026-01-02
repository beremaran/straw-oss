package middleware

import (
	"context"
	"errors"
	"strconv"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/labstack/echo/v4"
)

const (
	HeaderSessionID               = "X-Session-ID"
	HeaderSessionEnd              = "X-Session-End"
	HeaderSessionMigrated         = "X-Session-Migrated"
	HeaderSessionMigrationCount   = "X-Session-Migration-Count"
	HeaderSessionPreviousEndpoint = "X-Session-Previous-Endpoint"
)

type contextKey string

const SessionContextKey contextKey = "session"

// SessionMiddleware handles session loading and lifecycle headers.
func SessionMiddleware(service *session.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			sessionID := c.Request().Header.Get(HeaderSessionID)

			// Handle Explicit Session End
			if c.Request().Header.Get(HeaderSessionEnd) == "true" {
				if sessionID != "" {
					_ = service.EndSession(ctx, sessionID)
				}
				// We still proceed, but maybe we should clear the session ID from request?
				// Design says "Server deletes session immediately."
				// If client sent ID and End=true, we probably shouldn't try to load it.
				return next(c)
			}

			// Load Session if ID is present
			if sessionID != "" {
				sess, err := service.GetSession(ctx, sessionID)
				if err == nil {
					// Session found and valid
					// Touch session to keep it alive
					_ = service.TouchSession(ctx, sessionID)

					// Inject into context
					c.SetRequest(c.Request().WithContext(context.WithValue(ctx, SessionContextKey, sess)))

					// Set response headers for existing session
					// (The Service might have mutated it if we did logic here, but for now just ID)
					c.Response().Header().Set(HeaderSessionID, sess.ID)

					// If request checks for migration, we might need to know if it JUST migrated.
					// But migration happens in error handling usually.
					// If the loaded session has migration count > 0, do we send headers?
					// Design says: "Client receives X-Session-Migrated: true header" (on migration event).
					// Persistent headers like Migration count might be useful.
					if sess.MigrationCount > 0 {
						c.Response().Header().Set(HeaderSessionMigrationCount, itoa(sess.MigrationCount))
					}
				} else {
					// Session ID sent but not found/expired
					if errors.Is(err, domain.ErrSessionExpired) {
						// Return 410 or just ignore and create new?
						// Design 5.2 Error Codes: SESSION_EXPIRED -> 410.
						// "Requested session no longer exists".
						return c.JSON(domain.ErrSessionExpired.HTTPCode, domain.ErrSessionExpired.ToResponse(c.Response().Header().Get(echo.HeaderXRequestID), ""))
					}
					// Other errors?
					// Log maybe.
				}
			}

			// Proceed
			return next(c)
		}
	}
}

// GetSessionFromContext retrieves the session from the context if present.
func GetSessionFromContext(ctx context.Context) *domain.Session {
	sess, ok := ctx.Value(SessionContextKey).(*domain.Session)
	if !ok {
		return nil
	}
	return sess
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
