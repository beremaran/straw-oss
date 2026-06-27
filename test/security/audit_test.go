package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server"
	"github.com/beremaran/straw/internal/server/handlers"
	"github.com/beremaran/straw/internal/service/auth"
	"github.com/beremaran/straw/internal/service/filter"
	"github.com/beremaran/straw/internal/service/orchestrator"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/internal/service/session"
	"github.com/stretchr/testify/assert"
)

func TestSecurity_SSRFProtection(t *testing.T) {

	reqJSON := `{"url": "http://127.0.0.1:8080/admin", "method": "GET"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/request", strings.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h := handlers.NewRelayHandler(nil, nil, nil, nil, nil)

	h.Handle(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid target url")
	assert.Contains(t, rec.Body.String(), "private ip")
}

func TestSecurity_BodyLimit(t *testing.T) {

	cfg := config.ServerConfig{}
	cfg.MaxBodySize = "10B"
	cfg.HTTPPort = 0

	s := server.New(cfg, &auth.Service{}, &session.Service{}, &router.Matcher{}, &ratelimit.RateLimiter{}, &filter.Service{}, &orchestrator.RetryExecutor{})

	largeBody := strings.Repeat("A", 100)
	req := httptest.NewRequest(http.MethodPost, "/v1/request", strings.NewReader(largeBody))
	rec := httptest.NewRecorder()

	s.GetMux().ServeHTTP(rec, req)

	rec3 := httptest.NewRecorder()
	s.GetHandler().ServeHTTP(rec3, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec3.Code)
}
