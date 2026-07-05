package control

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBootstrapFromEnvCreatesFirstSystemAdmin(t *testing.T) {
	t.Parallel()

	store := NewInMemoryAPIKeyStore()
	pepper := []byte("pepper")
	bootstrapKey := strings.Join([]string{"sk_live", "bootstrapadminkeymaterial"}, "_")

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

	_, created, err := BootstrapFromEnv(context.Background(), store, "sk_live_REDACTED", pepper)
	if err != nil || !created {
		t.Fatalf("first bootstrap: created=%v err=%v", created, err)
	}

	_, created, err = BootstrapFromEnv(context.Background(), store, "sk_live_REDACTED", pepper)
	if err != nil {
		t.Fatalf("second bootstrap error = %v", err)
	}
	if created {
		t.Fatal("second bootstrap should be a no-op once a system_admin key exists")
	}

	// The second bootstrap key must not have been created/usable.
	auth := NewAuthenticator(store, pepper)
	_, err = auth.Authenticate(context.Background(), "Bearer sk_live_REDACTED")
	if !errors.Is(err, ErrAuthFailure) {
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

func TestBootstrapDevTenantIdempotent(t *testing.T) {
	t.Parallel()

	store := NewInMemoryTenantStore()
	const id = "00000000-0000-4000-8000-0000000000de"

	created, err := BootstrapDevTenant(context.Background(), store, id)
	if err != nil || !created {
		t.Fatalf("first seed: created=%v err=%v", created, err)
	}

	created, err = BootstrapDevTenant(context.Background(), store, id)
	if err != nil || created {
		t.Fatalf("second seed should be a no-op: created=%v err=%v", created, err)
	}

	tenant, err := store.Get(context.Background(), id)
	if err != nil || tenant.Status != TenantStatusActive {
		t.Fatalf("dev tenant = %+v err=%v, want active", tenant, err)
	}
}

func TestBootstrapDevRoutingRuleIdempotent(t *testing.T) {
	t.Parallel()

	store := NewInMemoryRoutingRuleStore()
	const tenantID, poolID = "ten_dev", "pool_dev"

	created, err := BootstrapDevRoutingRule(context.Background(), store, tenantID, poolID)
	if err != nil || !created {
		t.Fatalf("first seed: created=%v err=%v", created, err)
	}

	// On restart the rule already exists; the expectedVersion=0 upsert must be
	// treated as an idempotent no-op, not a fatal version conflict.
	created, err = BootstrapDevRoutingRule(context.Background(), store, tenantID, poolID)
	if err != nil || created {
		t.Fatalf("second seed should be a no-op: created=%v err=%v", created, err)
	}
}

func TestBootstrapWorkerCredentialScopesToDevTenantAndPool(t *testing.T) {
	t.Parallel()

	store := NewInMemoryWorkerCredentialStore()
	const credID, tenantID, poolID = "cred_dev", "ten_dev", "pool_dev"

	created, err := BootstrapWorkerCredentialFromEnv(context.Background(), store, credID, "pubkey",
		[]string{tenantID}, []AllowedPool{{TenantID: tenantID, PoolID: poolID}})
	if err != nil || !created {
		t.Fatalf("seed: created=%v err=%v", created, err)
	}

	cred, err := store.Get(context.Background(), credID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(cred.TenantScope) != 1 || cred.TenantScope[0] != tenantID {
		t.Fatalf("TenantScope = %v, want [%s]", cred.TenantScope, tenantID)
	}
	if len(cred.AllowedPools) != 1 || cred.AllowedPools[0].TenantID != tenantID || cred.AllowedPools[0].PoolID != poolID {
		t.Fatalf("AllowedPools = %+v, want [{%s %s}]", cred.AllowedPools, tenantID, poolID)
	}
}

func TestBootstrapWorkerCredentialDefaultsScopeWhenEmpty(t *testing.T) {
	t.Parallel()

	store := NewInMemoryWorkerCredentialStore()

	_, err := BootstrapWorkerCredentialFromEnv(context.Background(), store, "cred_x", "pubkey", nil, nil)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	cred, err := store.Get(context.Background(), "cred_x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(cred.TenantScope) != 1 || cred.TenantScope[0] != devWorkerTenantScope {
		t.Fatalf("TenantScope = %v, want fallback [%s]", cred.TenantScope, devWorkerTenantScope)
	}
}
