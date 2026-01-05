package middleware

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
)

const (
	// maxAuditBodySize limits memory usage for audit logging.
	maxAuditBodySize = 1024 // 1KB

	// defaultAuditBufferSize is the default channel buffer size.
	defaultAuditBufferSize = 1000

	// defaultAuditWorkers is the default number of worker goroutines.
	defaultAuditWorkers = 2
)

// Execer is the interface for database execution (satisfied by *pgxpool.Pool).
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Timestamp time.Time
	Method    string
	Path      string
	Query     string
	Body      string
	IP        string
	UserAgent string
	Status    int
	Error     string
}

// AuditLogger handles async audit logging with a worker pool.
type AuditLogger struct {
	db      Execer
	entries chan AuditEntry
	wg      sync.WaitGroup
	logger  *slog.Logger
	closed  bool
	mu      sync.RWMutex
}

// AuditLoggerOption configures the AuditLogger.
type AuditLoggerOption func(*AuditLogger)

// WithAuditLogger sets a custom logger.
func WithAuditLogger(logger *slog.Logger) AuditLoggerOption {
	return func(al *AuditLogger) {
		al.logger = logger
	}
}

// NewAuditLogger creates a new AuditLogger with a worker pool.
func NewAuditLogger(db Execer, bufferSize, workers int, opts ...AuditLoggerOption) *AuditLogger {
	if bufferSize <= 0 {
		bufferSize = defaultAuditBufferSize
	}
	if workers <= 0 {
		workers = defaultAuditWorkers
	}

	al := &AuditLogger{
		db:      db,
		entries: make(chan AuditEntry, bufferSize),
		logger:  slog.Default(),
	}

	for _, opt := range opts {
		opt(al)
	}

	// Start workers
	for i := 0; i < workers; i++ {
		al.wg.Add(1)
		go al.worker()
	}

	return al
}

// Log queues an audit entry for async processing.
// Returns false if the logger is closed or buffer is full.
func (al *AuditLogger) Log(entry AuditEntry) bool {
	al.mu.RLock()
	defer al.mu.RUnlock()

	if al.closed {
		return false
	}

	select {
	case al.entries <- entry:
		return true
	default:
		// Buffer full, drop entry
		al.logger.Warn("audit log buffer full, dropping entry",
			"method", entry.Method,
			"path", entry.Path,
		)
		return false
	}
}

// Stop gracefully shuts down the audit logger.
func (al *AuditLogger) Stop() {
	al.mu.Lock()
	if al.closed {
		al.mu.Unlock()
		return
	}
	al.closed = true
	al.mu.Unlock()

	close(al.entries)
	al.wg.Wait()
}

func (al *AuditLogger) worker() {
	defer al.wg.Done()

	const insertQuery = `
		INSERT INTO admin_audit_log (timestamp, method, path, query, body, ip, user_agent, status, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	for entry := range al.entries {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := al.db.Exec(ctx, insertQuery,
			entry.Timestamp,
			entry.Method,
			entry.Path,
			entry.Query,
			entry.Body,
			entry.IP,
			entry.UserAgent,
			entry.Status,
			entry.Error,
		)
		cancel()

		if err != nil {
			al.logger.Error("failed to insert audit log",
				"error", err,
				"method", entry.Method,
				"path", entry.Path,
			)
		}
	}
}

// AuditLog returns a middleware that logs state-changing requests to the database.
// It skips GET and HEAD requests.
func AuditLog(auditLogger *AuditLogger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Skip safe methods
			if req.Method == "GET" || req.Method == "HEAD" {
				return next(c)
			}

			// Capture request body with size limit
			var bodyStr string
			if req.Body != nil {
				// Read only up to maxAuditBodySize+1 to detect truncation
				limitedReader := io.LimitReader(req.Body, maxAuditBodySize+1)
				reqBody, _ := io.ReadAll(limitedReader)

				// Check if body was truncated
				if len(reqBody) > maxAuditBodySize {
					bodyStr = string(reqBody[:maxAuditBodySize]) + "...(truncated)"
				} else {
					bodyStr = string(reqBody)
				}

				// Restore body for next handlers
				// We need to reconstruct the full body by combining what we read with remaining
				req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(reqBody), req.Body))
			}

			// Capture values before next() to avoid race conditions
			method := req.Method
			path := req.URL.Path
			queryStr := req.URL.RawQuery
			ip := c.RealIP()
			userAgent := req.UserAgent()
			timestamp := time.Now()

			// Execute next handler
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

			// Queue for async logging via worker pool
			auditLogger.Log(AuditEntry{
				Timestamp: timestamp,
				Method:    method,
				Path:      path,
				Query:     queryStr,
				Body:      bodyStr,
				IP:        ip,
				UserAgent: userAgent,
				Status:    status,
				Error:     errStr,
			})

			return err
		}
	}
}

// AuditLogLegacy returns a middleware using the old unbounded goroutine pattern.
// Deprecated: Use AuditLog with AuditLogger instead.
func AuditLogLegacy(db Execer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Skip safe methods
			if req.Method == "GET" || req.Method == "HEAD" {
				return next(c)
			}

			// Capture request body with size limit
			var bodyStr string
			if req.Body != nil {
				limitedReader := io.LimitReader(req.Body, maxAuditBodySize+1)
				reqBody, _ := io.ReadAll(limitedReader)

				if len(reqBody) > maxAuditBodySize {
					bodyStr = string(reqBody[:maxAuditBodySize]) + "...(truncated)"
				} else {
					bodyStr = string(reqBody)
				}

				req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(reqBody), req.Body))
			}

			method := req.Method
			path := req.URL.Path
			queryStr := req.URL.RawQuery
			ip := c.RealIP()
			userAgent := req.UserAgent()

			err := next(c)

			status := c.Response().Status
			errStr := ""
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					status = he.Code
					errStr = he.Error()
				} else {
					errStr = err.Error()
				}
			}

			// Async logging (unbounded goroutine - legacy pattern)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

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
