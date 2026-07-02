package control

import (
	"context"
	"testing"
)

func TestBootstrapFromEnvCreatesFirstSystemAdmin(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	bootstrapKey := "sk_live_bootstrapadminkeymaterial"

	id, created, err := BootstrapFromEnv(context.Background(), store, bootstrapKey, pepper)
	if err != nil {
		t.Fatalf("BootstrapFromEnv() error = %v", err)
	}
	if !created {
		t.Fatal("BootstrapFromEnv() created = false, want true")
	}
	if id == "" {
		t.Fatal("BootstrapFromEnv() returned empty id")
	}

	auth := NewAuthenticator(store, pepper)
	identity, err := auth.Authenticate(context.Background(), "Bearer "+bootstrapKey)
	if err != nil {
		t.Fatalf("Authenticate() with bootstrap key error = %v", err)
	}
	if identity.Role != RoleSystemAdmin || identity.ScopeType != ScopePlatform {
		t.Fatalf("bootstrap identity = %+v, want platform system_admin", identity)
	}
}

func TestBootstrapFromEnvNoopWhenAdminExists(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")

	_, created, err := BootstrapFromEnv(context.Background(), store, "sk_live_firstbootstrapkey", pepper)
	if err != nil || !created {
		t.Fatalf("first bootstrap: created=%v err=%v", created, err)
	}

	_, created, err = BootstrapFromEnv(context.Background(), store, "sk_live_secondbootstrapkey", pepper)
	if err != nil {
		t.Fatalf("second bootstrap error = %v", err)
	}
	if created {
		t.Fatal("second bootstrap should be a no-op once a system_admin key exists")
	}

	// The second bootstrap key must not have been created/usable.
	auth := NewAuthenticator(store, pepper)
	if _, err := auth.Authenticate(context.Background(), "Bearer sk_live_secondbootstrapkey"); !errors.Is(err, ErrAuthFailure) {
		t.Fatalf("second bootstrap key should not authenticate, err = %v", err)
	}
}

func TestBootstrapFromEnvNoopWhenUnset(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	id, created, err := BootstrapFromEnv(context.Background(), store, "", nil)
	if err != nil {
		t.Fatalf("BootstrapFromEnv() error = %v", err)
	}
	if created || id != "" {
		t.Fatalf("BootstrapFromEnv() with empty key: created=%v id=%q, want false/empty", created, id)
	}
}
