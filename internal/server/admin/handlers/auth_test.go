package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw/internal/server/admin/handlers"
	adminauth "github.com/beremaran/straw/internal/service/auth"
)

func TestAuthHandler_SSOStart_Validation(t *testing.T) {
	// Simple test to ensure redirect_uri validation works
	authHandler := handlers.NewAuthHandler(&adminauth.AdminService{})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/management/auth/sso/google/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	// Create a minimal router to extract path values if we used Go 1.22 routing
	mux := http.NewServeMux()
	mux.HandleFunc("GET /management/auth/sso/{provider}/start", authHandler.HandleSSOStart)
	mux.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestAuthHandler_SSOCallback_Validation(t *testing.T) {
	authHandler := handlers.NewAuthHandler(&adminauth.AdminService{})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/management/auth/sso/google/callback", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /management/auth/sso/{provider}/callback", authHandler.HandleSSOCallback)
	mux.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}
