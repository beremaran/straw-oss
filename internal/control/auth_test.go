package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- API key generation / hashing ----

func TestGenerateAPIKeyEntropyAndPrefix(t *testing.T) {
	t.Parallel()

	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	if !strings.HasPrefix(generated.Secret, "sk_live_") {
		t.Fatalf("secret = %q, want sk_live_ prefix", generated.Secret)
	}
	// 32 random bytes base64url-encoded (no padding) is 43 chars; plus the
	// 8-char literal prefix, comfortably over 128 bits of entropy.
	if len(generated.Secret) < 40 {
		t.Fatalf("secret length = %d, want >= 40 (>=128 bits entropy)", len(generated.Secret))
	}
	if generated.Prefix != generated.Secret[:keyPrefixRunes] {
		t.Fatalf("prefix = %q, want first %d chars of secret", generated.Prefix, keyPrefixRunes)
	}

	second, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() second call error = %v", err)
	}
	if second.Secret == generated.Secret {
		t.Fatal("two generated keys must not collide")
	}
}

func TestHashAndVerifyAPIKeySecret(t *testing.T) {
	t.Parallel()

	pepper := []byte("pepper")
	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	hash := HashAPIKeySecret(generated.Secret, pepper)

	if !VerifyAPIKeySecret(generated.Secret, hash, pepper) {
		t.Fatal("VerifyAPIKeySecret() = false for correct secret, want true")
	}
	if VerifyAPIKeySecret("sk_live_wrongsecret", hash, pepper) {
		t.Fatal("VerifyAPIKeySecret() = true for wrong secret, want false")
	}
	if VerifyAPIKeySecret(generated.Secret, hash, []byte("different-pepper")) {
		t.Fatal("VerifyAPIKeySecret() = true with wrong pepper, want false")
	}
}

func TestHashAPIKeySecretNeverEqualsPlaintext(t *testing.T) {
	t.Parallel()

	generated, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	hash := HashAPIKeySecret(generated.Secret, nil)
	if hash == generated.Secret {
		t.Fatal("hash must never equal plaintext secret")
	}
}

// ---- prefix collision handling ----

func TestAuthenticatorHandlesPrefixCollisions(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")

	// Two distinct keys that happen to share the first keyPrefixRunes
	// characters (the visible lookup prefix) but differ afterward. Real
	// generated keys essentially never collide (256 bits of entropy), so
	// the collision is constructed directly to exercise the "check all
	// candidates sharing a prefix" path deterministically.
	sharedPrefix := "sk_live_AAAA"
	secretA := sharedPrefix + "restOfSecretA1111111111"
	secretB := sharedPrefix + "restOfSecretB2222222222"

	mustCreate(t, store, APIKeyRecord{
		ID: "key_a", ScopeType: ScopeTenant, TenantID: "ten_a", Role: RoleRequester,
		Prefix: sharedPrefix, SecretHash: HashAPIKeySecret(secretA, pepper), Status: APIKeyStatusActive, CreatedAt: time.Now(),
	})
	mustCreate(t, store, APIKeyRecord{
		ID: "key_b", ScopeType: ScopeTenant, TenantID: "ten_b", Role: RoleViewer,
		Prefix: sharedPrefix, SecretHash: HashAPIKeySecret(secretB, pepper), Status: APIKeyStatusActive, CreatedAt: time.Now(),
	})

	auth := NewAuthenticator(store, pepper)

	identity, err := auth.Authenticate(context.Background(), "Bearer "+secretB)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if identity.APIKeyID != "key_b" {
		t.Fatalf("resolved key = %q, want key_b (candidate scan must check all prefix matches)", identity.APIKeyID)
	}
}

func mustCreate(t *testing.T, store APIKeyStore, record APIKeyRecord) {
	t.Helper()
	if err := store.Create(context.Background(), record); err != nil {
		t.Fatalf("store.Create() error = %v", err)
	}
}

// ---- Authenticator behavior ----

func TestAuthenticateRejectsMissingHeader(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator(NewInMemoryAPIKeyStore(), nil)
	_, err := auth.Authenticate(context.Background(), "")
	if err != ErrAuthFailure {
		t.Fatalf("Authenticate() error = %v, want ErrAuthFailure", err)
	}
}

func TestAuthenticateRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator(NewInMemoryAPIKeyStore(), nil)
	_, err := auth.Authenticate(context.Background(), "Bearer sk_live_doesnotexist")
	if err != ErrAuthFailure {
		t.Fatalf("Authenticate() error = %v, want ErrAuthFailure", err)
	}
}

func TestAuthenticateRejectsRevokedKey(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	gen, _ := GenerateAPIKey()
	mustCreate(t, store, APIKeyRecord{
		ID: "key_1", ScopeType: ScopeTenant, TenantID: "ten_a", Role: RoleRequester,
		Prefix: gen.Prefix, SecretHash: HashAPIKeySecret(gen.Secret, pepper), Status: APIKeyStatusActive, CreatedAt: time.Now(),
	})
	auth := NewAuthenticator(store, pepper)

	if _, err := auth.Authenticate(context.Background(), "Bearer "+gen.Secret); err != nil {
		t.Fatalf("Authenticate() before revoke error = %v", err)
	}

	if _, err := store.Revoke(context.Background(), "key_1", time.Now()); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	if _, err := auth.Authenticate(context.Background(), "Bearer "+gen.Secret); err != ErrAuthFailure {
		t.Fatalf("Authenticate() after revoke error = %v, want ErrAuthFailure (revocation takes effect)", err)
	}
}

// ---- RBAC ----

func TestCanExecuteDataPlane(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity Identity
		want     bool
	}{
		{"platform system_admin cannot execute", Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, false},
		{"tenant requester can execute", Identity{ScopeType: ScopeTenant, Role: RoleRequester}, true},
		{"tenant tenant_admin can execute", Identity{ScopeType: ScopeTenant, Role: RoleTenantAdmin}, true},
		{"tenant viewer cannot execute", Identity{ScopeType: ScopeTenant, Role: RoleViewer}, false},
		{"tenant operator defaults to no execution in P0", Identity{ScopeType: ScopeTenant, Role: RoleOperator}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanExecuteDataPlane(tt.identity); got != tt.want {
				t.Errorf("CanExecuteDataPlane() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---- Platform key cannot execute data-plane requests (integration
// through the actual HTTP handler) ----

func TestPlatformKeyCannotExecuteDataPlaneRequest(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	gen, _ := GenerateAPIKey()
	mustCreate(t, store, APIKeyRecord{
		ID: "key_platform", ScopeType: ScopePlatform, Role: RoleSystemAdmin,
		Prefix: gen.Prefix, SecretHash: HashAPIKeySecret(gen.Secret, pepper), Status: APIKeyStatusActive, CreatedAt: time.Now(),
	})
	auth := NewAuthenticator(store, pepper)
	h := NewRequestHandler(1_048_576, 1_048_576, 120_000, auth)

	payload := `{"method":"GET","url":"https://example.com/path"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+gen.Secret)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	var errResp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != "insufficient_permissions" {
		t.Fatalf("code = %q, want %q", errResp.Code, "insufficient_permissions")
	}
}

func TestUnauthenticatedRequestRejected(t *testing.T) {
	t.Parallel()

	auth := NewAuthenticator(NewInMemoryAPIKeyStore(), nil)
	h := NewRequestHandler(1_048_576, 1_048_576, 120_000, auth)

	payload := `{"method":"GET","url":"https://example.com/path"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	var errResp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != "auth_failure" {
		t.Fatalf("code = %q, want %q", errResp.Code, "auth_failure")
	}
}

func TestViewerCannotExecuteDataPlaneRequest(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	gen, _ := GenerateAPIKey()
	mustCreate(t, store, APIKeyRecord{
		ID: "key_viewer", ScopeType: ScopeTenant, TenantID: "ten_a", Role: RoleViewer,
		Prefix: gen.Prefix, SecretHash: HashAPIKeySecret(gen.Secret, pepper), Status: APIKeyStatusActive, CreatedAt: time.Now(),
	})
	auth := NewAuthenticator(store, pepper)
	h := NewRequestHandler(1_048_576, 1_048_576, 120_000, auth)

	payload := `{"method":"GET","url":"https://example.com/path"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/requests", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+gen.Secret)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}
