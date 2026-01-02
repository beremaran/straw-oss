package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/server"
	"github.com/kwilabs/straw-proxy-server/internal/server/handlers"
	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/kwilabs/straw-proxy-server/internal/service/filter"
	"github.com/kwilabs/straw-proxy-server/internal/service/orchestrator"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestSecurity_SSRFProtection(t *testing.T) {
	// Setup Request with internal IP
	reqJSON := `{"url": "http://127.0.0.1:8080/admin", "method": "GET"}`
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/request", strings.NewReader(reqJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Minimal handler setup
	h := handlers.NewRelayHandler(nil, nil, nil, nil, nil)

	// Execute
	err := h.Handle(c)

	// Assert
	if assert.Error(t, err) {
		he, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusForbidden, he.Code)
		assert.Contains(t, he.Message, "invalid target url")
		assert.Contains(t, he.Message, "private ip")
	}
}

func TestSecurity_BodyLimit(t *testing.T) {
	// Setup Server with Body Limit
	cfg := config.ServerConfig{}
	cfg.MaxBodySize = "10B" // Tiny limit for testing
	cfg.HTTPPort = 0        // Random port

	// Mocks (needed for New server)
	// We can't easily mock New() without spinning up full deps,
	// but we can test the middleware logic by creating a server instance or separate test
	// Actually, Server.New() registers the middleware.

	s := server.New(cfg, &auth.AuthService{}, &session.Service{}, &router.Matcher{}, &ratelimit.RateLimiter{}, &filter.Service{}, &orchestrator.RetryExecutor{})

	// Create a large request
	largeBody := strings.Repeat("A", 100)
	req := httptest.NewRequest(http.MethodPost, "/v1/request", strings.NewReader(largeBody))
	rec := httptest.NewRecorder()

	s.GetEcho().ServeHTTP(rec, req)

	// Expect 413 Payload Too Large
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
