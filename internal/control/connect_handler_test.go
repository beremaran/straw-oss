package control

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConnectTargetValidationNormalizesHostPort(t *testing.T) {
	t.Parallel()

	u, verr := validateConnectTarget("Example.COM.:443")
	if verr != nil {
		t.Fatalf("validateConnectTarget() error = %v", verr)
	}
	if got := u.String(); got != "connect://example.com:443" {
		t.Fatalf("target URL = %q, want connect://example.com:443", got)
	}
}

func TestConnectTargetValidationRejectsMissingPort(t *testing.T) {
	t.Parallel()

	_, verr := validateConnectTarget("example.com")
	if verr == nil {
		t.Fatal("validateConnectTarget() error = nil, want missing-port validation error")
	}
}

func TestConnectHandlerRejectsNonConnectBeforeDispatch(t *testing.T) {
	t.Parallel()

	h, token, dispatcher := newTestConnectHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com:443", nil)
	req.Header.Set("Proxy-Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestConnectHandlerRequiresProxyAuthorization(t *testing.T) {
	t.Parallel()

	h, _, dispatcher := newTestConnectHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodConnect, "http://example.com:443", nil)
	req.Host = "example.com:443"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := w.Header().Get("Proxy-Authenticate"); got != "Bearer" {
		t.Fatalf("Proxy-Authenticate = %q, want Bearer", got)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func newTestConnectHandler(t *testing.T) (*ConnectHandler, string, *captureTunnelDispatcher) {
	t.Helper()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	err = store.Create(context.Background(), APIKeyRecord{
		ID:         "key_connect",
		ScopeType:  ScopeTenant,
		TenantID:   "ten_connect",
		Role:       RoleRequester,
		Prefix:     generated.Prefix,
		SecretHash: HashAPIKeySecret(generated.Secret, pepper),
		Status:     APIKeyStatusActive,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	dispatcher := &captureTunnelDispatcher{}
	h := NewConnectHandler(NewAuthenticator(store, pepper))
	h.SetDispatcher(dispatcher)

	return h, generated.Secret, dispatcher
}

type captureTunnelDispatcher struct {
	calls int
}

func (d *captureTunnelDispatcher) Dispatch(context.Context, DispatchInput) (SuccessResponse, *PipelineError) {
	return SuccessResponse{}, &PipelineError{Code: ControlInternalError}
}

func (d *captureTunnelDispatcher) DispatchTunnel(context.Context, DispatchInput, io.ReadWriter) (SuccessResponse, *PipelineError) {
	d.calls++

	return SuccessResponse{}, nil
}
