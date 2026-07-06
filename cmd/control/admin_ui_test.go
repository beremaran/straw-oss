package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	routeAdminUI    = "/admin/"
	adminUISmokeURL = "http://127.0.0.1:18080/admin/?__smoke=1"
)

func TestAdminUIServesUsableFirstScreen(t *testing.T) {
	mux := http.NewServeMux()
	serveAdminUIRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, routeAdminUI, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/ status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Straw Admin",
		"Requests",
		"Workers",
		"Audit",
		"Tenants",
		"Routes",
		"Deny rules",
		"Injection",
		"API key",
		"/api/v1/telemetry/requests",
		"/api/v1/admin/workers",
		"/api/v1/config/changes",
		routeConfigTenants,
		"/api/v1/config/routing-rules",
		"/api/v1/config/deny-rules",
		"/api/v1/config/injection-policies",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("admin UI missing %q", want)
		}
	}
}

func TestAdminUIRedirectAndRedactionGuardrails(t *testing.T) {
	mux := http.NewServeMux()
	serveAdminUIRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /admin status = %d, want 301", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != routeAdminUI {
		t.Fatalf("Location = %q, want /admin/", got)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, routeAdminUI, nil))
	body := rec.Body.String()
	for _, field := range []string{"session_id", "assign_subject", "register_subject", "heartbeat_subject", "nats_subjects", "selected_executor"} {
		if !strings.Contains(body, `"`+field+`"`) {
			t.Fatalf("admin UI does not suppress %s", field)
		}
	}
}

func TestAdminUIBrowserSmoke(t *testing.T) {
	mux := http.NewServeMux()
	serveAdminUIRoutes(mux)
	serveAdminUISmokeAPI(mux)

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:18080")
	if err != nil {
		t.Skipf("admin UI smoke port unavailable: %v", err)
	}

	server := httptest.NewUnstartedServer(mux)
	server.Listener = ln
	server.Start()
	t.Cleanup(server.Close)

	out, err := dumpAdminUIDOM(t)
	if err != nil {
		t.Fatalf("chrome smoke failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `data-smoke="pass"`) {
		t.Fatalf("admin UI smoke did not pass:\n%s", out)
	}
}

func dumpAdminUIDOM(t *testing.T) ([]byte, error) {
	t.Helper()

	_, err := os.Stat("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
	if err == nil {
		return exec.CommandContext(context.Background(), "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"--headless=new",
			"--disable-gpu",
			"--no-sandbox",
			"--virtual-time-budget=5000",
			"--dump-dom",
			adminUISmokeURL,
		).CombinedOutput()
	}

	_, err = os.Stat("/Applications/Chromium.app/Contents/MacOS/Chromium")
	if err == nil {
		return exec.CommandContext(context.Background(), "/Applications/Chromium.app/Contents/MacOS/Chromium",
			"--headless=new",
			"--disable-gpu",
			"--no-sandbox",
			"--virtual-time-budget=5000",
			"--dump-dom",
			adminUISmokeURL,
		).CombinedOutput()
	}

	t.Skip("Chrome/Chromium not available for browser smoke test")

	return nil, nil
}

func serveAdminUISmokeAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/telemetry/requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"request_id":"req_1","client_status":200,"method":"GET","target_host":"example.com","timing":{"total_ms":12}}]}`))
	})
	mux.HandleFunc("GET /api/v1/telemetry/requests/req_1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req_1","client_status":200,"method":"GET","target_host":"example.com","selected_executor":"secret","attempts":[{"attempt":1,"client_status":200}]}`))
	})
	mux.HandleFunc("GET /api/v1/admin/workers", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"worker_id":"worker_1","runtime_state":"ready","health":"healthy","active_requests":1,"available_capacity":9,"session_id":"session_secret","assign_subject":"subject_secret"}]`))
	})
	mux.HandleFunc("GET /api/v1/config/changes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"created_at":"2026-07-06T00:00:00Z","resource_type":"routing_rule","resource_id":"route_1","action":"upsert","actor_type":"api_key"}]`))
	})
	mux.HandleFunc("GET /api/v1/config/tenants", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"ten_1","name":"Tenant A","status":"active","default_timeout_ms":1000,"max_timeout_ms":2000,"config_version":1}]`))
	})
	mux.HandleFunc("GET /api/v1/config/routing-rules", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rules":[{"id":"route_1","tenant_id":"ten_1","priority":1,"enabled":true,"target_pool_id":"pool_1","config_version":1}]}`))
	})
	mux.HandleFunc("GET /api/v1/config/deny-rules", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rules":[{"id":"deny_1","tenant_id":"ten_1","enabled":true,"type":"host","value":"blocked.example","action":"deny"}]}`))
	})
	mux.HandleFunc("GET /api/v1/config/injection-policies", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"policies":[{"id":"inject_1","tenant_id":"ten_1","enabled":true,"max_operations":4,"config_version":1}]}`))
	})
}
