package middleware

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
)

// Execer is the interface for database execution (satisfied by *pgxpool.Pool).
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// AuditLog returns a middleware that logs state-changing requests to the database.
// It skips GET and HEAD requests.
func AuditLog(db Execer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Skip safe methods
			if req.Method == "GET" || req.Method == "HEAD" {
				return next(c)
			}

			// Capture request body
			var reqBody []byte
			if req.Body != nil {
				reqBody, _ = io.ReadAll(req.Body)
				// Restore body for next handlers
				req.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}

			// Capture error and response status
			err := next(c)

			status := c.Response().Status
			errStr := ""
			if err != nil {
				// If error is an HTTP error, use its code
				if he, ok := err.(*echo.HTTPError); ok {
					status = he.Code
					errStr = he.Error()
				} else {
					errStr = err.Error()
				}
			}

			// Capture values before goroutine to avoid race conditions
			method := req.Method
			path := req.URL.Path
			queryStr := req.URL.RawQuery
			ip := c.RealIP()
			userAgent := req.UserAgent()
			bodyStr := string(reqBody)

			// Async logging
			go func() {
				// Use background context for async operation
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				// Truncate body if too large
				if len(bodyStr) > 4000 {
					bodyStr = bodyStr[:4000] + "...(truncated)"
				}

				query := `
					INSERT INTO admin_audit_log (timestamp, method, path, query, body, ip, user_agent, status, error)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				`

				_, _ = db.Exec(ctx, query,
					time.Now(),
					method,
					path,
					queryStr,
					bodyStr,
					ip,
					userAgent,
					status,
					errStr,
				)
			}()

			return err
		}
	}
}
