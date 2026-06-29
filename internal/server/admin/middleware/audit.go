package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/beremaran/straw/internal/domain"
)

var redactionRegex = regexp.MustCompile(`(?i)("(raw_key|token|password|client_secret|Authorization)"\s*:\s*)("[^"]*"|[0-9]+|true|false|null)`)

const auditTimeout = 5 * time.Second

func redactSensitiveFields(body string) string {
	return redactionRegex.ReplaceAllString(body, `$1"[REDACTED]"`)
}

const (
	maxAuditBodySize = 1024

	defaultAuditBufferSize = 1000

	defaultAuditWorkers = 2
)

// Execer is the interface for executing SQL commands.
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// AuditEntry holds fields for a single audit log entry.
type AuditEntry struct {
	Timestamp        time.Time
	ActorType        string
	ActorID          string
	ActorDisplayName string
	SessionID        string
	RequestID        string
	TraceID          string
	Method           string
	Path             string
	Query            string
	Body             string
	IP               string
	UserAgent        string
	Status           int
	Error            string
}

// AuditLogger buffers and asynchronously writes audit log entries to the database.
type AuditLogger struct {
	db      Execer
	entries chan AuditEntry
	wg      sync.WaitGroup
	logger  *slog.Logger
	closed  bool
	mu      sync.RWMutex
}

// AuditLoggerOption configures an AuditLogger.
type AuditLoggerOption func(*AuditLogger)

// WithAuditLogger sets the slog logger on the AuditLogger.
func WithAuditLogger(logger *slog.Logger) AuditLoggerOption {
	return func(al *AuditLogger) {
		al.logger = logger
	}
}

// NewAuditLogger creates a new AuditLogger with the given database, buffer size, and worker count.
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

// Log queues an audit entry for asynchronous persistence.
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

// Stop closes the logger and waits for all workers to finish.
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
		INSERT INTO admin_audit_log (
			timestamp, method, path, query, body, ip, user_agent, status, error,
			actor_type, actor_id, actor_display_name, session_id, request_id, trace_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	for entry := range al.entries {
		ctx, cancel := context.WithTimeout(context.Background(), auditTimeout)
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
			entry.ActorType,
			entry.ActorID,
			entry.ActorDisplayName,
			entry.SessionID,
			entry.RequestID,
			entry.TraceID,
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

// AuditLog returns a middleware that records non-GET/HEAD request audit entries.
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
				RequestID:        w.Header().Get("X-Request-Id"),
				TraceID:          w.Header().Get("Trace-Id"),
				Method:           method,
				Path:             path,
				Query:            queryStr,
				Body:             redactSensitiveFields(bodyStr),
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
	n, err := sw.ResponseWriter.Write(b)
	if err != nil {
		return 0, fmt.Errorf("write response: %w", err)
	}

	return n, nil
}

func getRealIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if before, _, ok := strings.Cut(ip, ","); ok {
			return strings.TrimSpace(before)
		}

		return strings.TrimSpace(ip)
	}

	return r.RemoteAddr
}

// NewAuditEvent creates a management audit event from the request context.
func NewAuditEvent(r *http.Request, action, entityType, entityID string, oldVal, newVal any) *domain.ManagementAuditEvent {
	actor, _ := ActorFromContext(r.Context())
	ip := getRealIP(r)
	userAgent := r.UserAgent()

	return &domain.ManagementAuditEvent{
		ActorType:    actor.Type,
		ActorID:      actor.ID,
		ActorDisplay: actor.DisplayName,
		Action:       action,
		EntityType:   entityType,
		EntityID:     entityID,
		OldValue:     oldVal,
		NewValue:     newVal,
		RequestID:    r.Header.Get("X-Request-Id"),
		TraceID:      r.Header.Get("Trace-Id"),
		IP:           ip,
		UserAgent:    userAgent,
	}
}
