package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockExecer struct {
	mock.Mock
}

func (m *MockExecer) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	args := m.Called(ctx, sql, arguments)

	return args.Get(0).(pgconn.CommandTag), args.Error(1)
}

func TestAuditLog(t *testing.T) {
	mockDB := new(MockExecer)

	auditLogger := NewAuditLogger(context.Background(), mockDB, 10, 1)
	defer auditLogger.Stop()
	mw := AuditLog(auditLogger)

	mockDB.On("Exec", mock.Anything, mock.MatchedBy(func(sql string) bool {
		return strings.Contains(sql, "INSERT INTO admin_audit_log")
	}), mock.Anything).Return(pgconn.CommandTag{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"foo":"bar"}`))
	rec := httptest.NewRecorder()

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))

	h.ServeHTTP(rec, req)

	time.Sleep(100 * time.Millisecond)

	mockDB.AssertExpectations(t)
}

func TestAuditLog_SkipGet(t *testing.T) {
	mockDB := new(MockExecer)

	auditLogger := NewAuditLogger(context.Background(), mockDB, 10, 1)
	defer auditLogger.Stop()
	mw := AuditLog(auditLogger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	h.ServeHTTP(rec, req)

	mockDB.AssertNotCalled(t, "Exec")
}

func TestAuditLogger_BufferFull(t *testing.T) {
	mockDB := new(MockExecer)

	auditLogger := &AuditLogger{
		db:      mockDB,
		entries: make(chan AuditEntry, 1),
		logger:  slog.Default(),
	}

	entry := AuditEntry{
		Timestamp: time.Now(),
		Method:    "POST",
		Path:      "/test",
	}

	ok := auditLogger.Log(entry)
	assert.True(t, ok)

	ok = auditLogger.Log(entry)
	assert.False(t, ok)
}

func TestAuditLogger_Closed(t *testing.T) {
	mockDB := new(MockExecer)
	auditLogger := NewAuditLogger(context.Background(), mockDB, 10, 1)
	auditLogger.Stop()

	entry := AuditEntry{
		Timestamp: time.Now(),
		Method:    "POST",
		Path:      "/test",
	}
	ok := auditLogger.Log(entry)
	assert.False(t, ok)
}
