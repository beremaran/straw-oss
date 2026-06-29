package middleware

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxAuditBodySize = 1024

	defaultAuditBufferSize = 1000

	defaultAuditWorkers = 2
)

type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type AuditEntry struct {
	Timestamp        time.Time
	ActorType        string
	ActorID          string
	ActorDisplayName string
	SessionID        string
	Method           string
	Path             string
	Query            string
	Body             string
	IP               string
	UserAgent        string
	Status           int
	Error            string
}

type AuditLogger struct {
	db      Execer
	entries chan AuditEntry
	wg      sync.WaitGroup
	logger  *slog.Logger
	closed  bool
	mu      sync.RWMutex
}

type AuditLoggerOption func(*AuditLogger)

func WithAuditLogger(logger *slog.Logger) AuditLoggerOption {
	return func(al *AuditLogger) {
		al.logger = logger
	}
}

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

	for i := 0; i < workers; i++ {
		al.wg.Add(1)
		go al.worker()
	}

	return al
}

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

		al.logger.Warn("audit log buffer full, dropping entry",
			"method", entry.Method,
			"path", entry.Path,
		)

		return false
	}
}

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

func AuditLog(auditLogger *AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)

				return
			}

			var bodyStr string
			if r.Body != nil {
				limitedReader := io.LimitReader(r.Body, maxAuditBodySize+1)
				reqBody, _ := io.ReadAll(limitedReader)

				if len(reqBody) > maxAuditBodySize {
					bodyStr = string(reqBody[:maxAuditBodySize]) + "...(truncated)"
				} else {
					bodyStr = string(reqBody)
				}

				r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(reqBody), r.Body))
			}

			method := r.Method
			path := r.URL.Path
			queryStr := r.URL.RawQuery
			ip := getRealIP(r)
			userAgent := r.UserAgent()
			timestamp := time.Now()
			actor, _ := ActorFromContext(r.Context())

			sw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			auditLogger.Log(AuditEntry{
				Timestamp:        timestamp,
				ActorType:        actor.Type,
				ActorID:          actor.ID,
				ActorDisplayName: actor.DisplayName,
				SessionID:        actor.SessionID,
				Method:           method,
				Path:             path,
				Query:            queryStr,
				Body:             bodyStr,
				IP:               ip,
				UserAgent:        userAgent,
				Status:           sw.status,
				Error:            "",
			})
		})
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusResponseWriter) WriteHeader(statusCode int) {
	sw.status = statusCode
	sw.ResponseWriter.WriteHeader(statusCode)
}

func (sw *statusResponseWriter) Write(b []byte) (int, error) {
	return sw.ResponseWriter.Write(b)
}

func getRealIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if idx := strings.Index(ip, ","); idx != -1 {
			return strings.TrimSpace(ip[:idx])
		}

		return strings.TrimSpace(ip)
	}

	return r.RemoteAddr
}
