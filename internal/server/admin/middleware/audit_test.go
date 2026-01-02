package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
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
	e := echo.New()
	mockDB := new(MockExecer)
	mw := AuditLog(mockDB)

	// Mock DB expectation
	mockDB.On("Exec", mock.Anything, mock.MatchedBy(func(sql string) bool {
		return strings.Contains(sql, "INSERT INTO admin_audit_log")
	}), mock.Anything).Return(pgconn.CommandTag{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"foo":"bar"}`))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := mw(func(c echo.Context) error {
		return c.String(http.StatusCreated, "created")
	})

	err := h(c)
	assert.NoError(t, err)

	// Wait a bit for async goroutine
	time.Sleep(100 * time.Millisecond)

	mockDB.AssertExpectations(t)
}

func TestAuditLog_SkipGet(t *testing.T) {
	e := echo.New()
	mockDB := new(MockExecer)
	mw := AuditLog(mockDB)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := h(c)
	assert.NoError(t, err)

	// Should NOT call Exec
	mockDB.AssertNotCalled(t, "Exec")
}
