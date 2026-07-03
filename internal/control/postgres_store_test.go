package control

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests exercise the Postgres-backed identity stores against a real
// database. They are skipped unless STRAW_TEST_POSTGRES_DSN points at a
// Postgres instance with migrations/postgres applied. Each test truncates the
// identity tables first, so the suite is idempotent across reruns; because of
// that shared-state reset the tests are intentionally not run in parallel.
const (
	pgTestTenantA      = "11111111-1111-1111-1111-111111111111"
	pgTestTenantB      = "22222222-2222-2222-2222-222222222222"
	pgTestActorType    = "api_key"
	pgTestActionAdd    = "create"
	pgTestPepper       = "test-pepper"
	pgTestExecutorType = "egress"
)

func newIdentityTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("STRAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("STRAW_TEST_POSTGRES_DSN not set")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}

	t.Cleanup(pool.Close)

	_, err = pool.Exec(context.Background(),
		`TRUNCATE tenants, api_keys, worker_credentials, config_audit_source RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate identity tables: %v", err)
	}

	return pool
}

func seedTenant(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()

	err := NewPostgresTenantStore(pool).Create(context.Background(), Tenant{ID: id, Status: TenantStatusActive})
	if err != nil {
		t.Fatalf("seed tenant %s: %v", id, err)
	}
}

func TestPostgresTenantStorePersistence(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresTenantStore(pool)

	tenant := Tenant{ID: pgTestTenantA, Status: TenantStatusActive, CreatedAt: time.Now().UTC()}

	err := store.Create(context.Background(), tenant)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.Get(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID != pgTestTenantA {
		t.Fatalf("id = %q, want %q", got.ID, pgTestTenantA)
	}

	if got.Status != TenantStatusActive {
		t.Fatalf("status = %q, want %q", got.Status, TenantStatusActive)
	}
}

func TestPostgresTenantStoreDuplicateRejected(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresTenantStore(pool)

	tenant := Tenant{ID: pgTestTenantA, Status: TenantStatusActive}

	err := store.Create(context.Background(), tenant)
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	err = store.Create(context.Background(), tenant)
	if err == nil {
		t.Fatal("expected error for duplicate tenant")
	}
}

func TestPostgresAPIKeyStorePersistence(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	pepper := []byte(pgTestPepper)
	store := NewPostgresAPIKeyStore(pool, pepper)

	record := APIKeyRecord{
		ID:         "00000000-0000-0000-0000-000000000010",
		ScopeType:  ScopeTenant,
		TenantID:   pgTestTenantA,
		Role:       RoleRequester,
		Prefix:     "pk_test",
		SecretHash: HashAPIKeySecret("secret123", pepper),
		Status:     APIKeyStatusActive,
	}

	err := store.Create(context.Background(), record)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Prefix != "pk_test" {
		t.Fatalf("prefix = %q, want %q", got.Prefix, "pk_test")
	}

	if got.ScopeType != ScopeTenant {
		t.Fatalf("scope_type = %q, want %q", got.ScopeType, ScopeTenant)
	}

	// TenantID must survive the uuid round-trip: the column is uuid, and a scan
	// that loses it would silently break tenant isolation.
	if got.TenantID != pgTestTenantA {
		t.Fatalf("tenant_id = %q, want %q", got.TenantID, pgTestTenantA)
	}

	if got.RevokedAt != nil {
		t.Fatalf("revoked_at = %v, want nil for active key", got.RevokedAt)
	}
}

func TestPostgresAPIKeyStorePrefixCollision(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)
	seedTenant(t, pool, pgTestTenantB)

	pepper := []byte(pgTestPepper)
	store := NewPostgresAPIKeyStore(pool, pepper)

	key1 := APIKeyRecord{
		ID:         "00000000-0000-0000-0000-000000000011",
		ScopeType:  ScopeTenant,
		TenantID:   pgTestTenantA,
		Role:       RoleRequester,
		Prefix:     "coll",
		SecretHash: HashAPIKeySecret("secret_a", pepper),
		Status:     APIKeyStatusActive,
	}
	key2 := APIKeyRecord{
		ID:         "00000000-0000-0000-0000-000000000012",
		ScopeType:  ScopeTenant,
		TenantID:   pgTestTenantB,
		Role:       RoleRequester,
		Prefix:     "coll",
		SecretHash: HashAPIKeySecret("secret_b", pepper),
		Status:     APIKeyStatusActive,
	}

	err := store.Create(context.Background(), key1)
	if err != nil {
		t.Fatalf("Create key1 error = %v", err)
	}

	err = store.Create(context.Background(), key2)
	if err != nil {
		t.Fatalf("Create key2 error = %v", err)
	}

	candidates, err := store.FindByPrefix(context.Background(), "coll")
	if err != nil {
		t.Fatalf("FindByPrefix() error = %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("found %d candidates, want 2", len(candidates))
	}

	// Both keys, and their distinct tenant scopes, must round-trip.
	tenants := make(map[string]string)
	for _, c := range candidates {
		tenants[c.ID] = c.TenantID
	}

	if tenants["00000000-0000-0000-0000-000000000011"] != pgTestTenantA {
		t.Errorf("key1 tenant = %q, want %q", tenants["00000000-0000-0000-0000-000000000011"], pgTestTenantA)
	}

	if tenants["00000000-0000-0000-0000-000000000012"] != pgTestTenantB {
		t.Errorf("key2 tenant = %q, want %q", tenants["00000000-0000-0000-0000-000000000012"], pgTestTenantB)
	}
}

func TestPostgresAPIKeyStoreRevocation(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	pepper := []byte(pgTestPepper)
	store := NewPostgresAPIKeyStore(pool, pepper)

	record := APIKeyRecord{
		ID:         "00000000-0000-0000-0000-000000000013",
		ScopeType:  ScopeTenant,
		TenantID:   pgTestTenantA,
		Role:       RoleRequester,
		Prefix:     "revoke",
		SecretHash: HashAPIKeySecret("secret_revoke", pepper),
		Status:     APIKeyStatusActive,
	}

	err := store.Create(context.Background(), record)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	candidates, err := store.FindByPrefix(context.Background(), "revoke")
	if err != nil {
		t.Fatalf("FindByPrefix() before revoke error = %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("found %d candidates before revoke, want 1", len(candidates))
	}

	revokedAt := time.Now().UTC().Truncate(time.Microsecond)

	revoked, err := store.Revoke(context.Background(), record.ID, revokedAt)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	if revoked.Status != APIKeyStatusRevoked {
		t.Fatalf("revoked status = %q, want %q", revoked.Status, APIKeyStatusRevoked)
	}

	if revoked.RevokedAt == nil || !revoked.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revoked_at = %v, want %v", revoked.RevokedAt, revokedAt)
	}

	// FindByPrefix returns active keys only.
	candidates, err = store.FindByPrefix(context.Background(), "revoke")
	if err != nil {
		t.Fatalf("FindByPrefix() after revoke error = %v", err)
	}

	if len(candidates) != 0 {
		t.Fatalf("found %d candidates after revoke, want 0", len(candidates))
	}

	// The persisted revoked_at must read back, not reset to zero time.
	got, err := store.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Get() after revoke error = %v", err)
	}

	if got.RevokedAt == nil || !got.RevokedAt.Equal(revokedAt) {
		t.Fatalf("persisted revoked_at = %v, want %v", got.RevokedAt, revokedAt)
	}
}

func TestPostgresAPIKeyStorePlatformCannotExecuteRequests(t *testing.T) {
	pool := newIdentityTestPool(t)

	pepper := []byte(pgTestPepper)
	store := NewPostgresAPIKeyStore(pool, pepper)

	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}

	record := APIKeyRecord{
		ID:         "00000000-0000-0000-0000-000000000014",
		ScopeType:  ScopePlatform,
		Role:       RoleSystemAdmin,
		Prefix:     gen.Prefix,
		SecretHash: HashAPIKeySecret(gen.Secret, pepper),
		Status:     APIKeyStatusActive,
	}

	err = store.Create(context.Background(), record)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	auth := NewAuthenticator(store, pepper)

	identity, err := auth.Authenticate(context.Background(), "Bearer "+gen.Secret)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	if !identity.IsPlatform() {
		t.Fatal("expected platform-scoped identity")
	}

	if CanExecuteDataPlane(identity) {
		t.Fatal("platform-scoped key must not execute data-plane requests")
	}
}

func TestPostgresWorkerCredentialStoreSingleTenant(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresWorkerCredentialStore(pool)

	cred := WorkerCredential{
		ID:                     "00000000-0000-0000-0000-000000000020",
		Status:                 WorkerCredentialStatusActive,
		ExecutorType:           pgTestExecutorType,
		PublicKeyEd25519Base64: "YWJjZA==",
		TenantScope:            []string{pgTestTenantA},
		AllowedPools:           []AllowedPool{{TenantID: pgTestTenantA, PoolID: "pool_1"}},
	}

	err := store.Create(context.Background(), cred)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := store.ListTenant(context.Background(), pgTestTenantA)
	if err != nil {
		t.Fatalf("ListTenant(A) error = %v", err)
	}

	if len(list) != 1 || list[0].ID != cred.ID {
		t.Fatalf("ListTenant(A) = %+v, want single credential %q", list, cred.ID)
	}

	// Single-tenant P0 scope: a credential scoped to tenant A must not surface
	// for tenant B.
	list, err = store.ListTenant(context.Background(), pgTestTenantB)
	if err != nil {
		t.Fatalf("ListTenant(B) error = %v", err)
	}

	if len(list) != 0 {
		t.Fatalf("ListTenant(B) = %d credentials, want 0", len(list))
	}
}

func TestPostgresWorkerCredentialStoreRevocation(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresWorkerCredentialStore(pool)

	cred := WorkerCredential{
		ID:                     "00000000-0000-0000-0000-000000000021",
		Status:                 WorkerCredentialStatusActive,
		ExecutorType:           pgTestExecutorType,
		PublicKeyEd25519Base64: "YWJjZA==",
		TenantScope:            []string{pgTestTenantA},
	}

	err := store.Create(context.Background(), cred)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	revoked, err := store.Revoke(context.Background(), cred.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	if revoked.Status != WorkerCredentialStatusRevoked {
		t.Fatalf("status = %q, want %q", revoked.Status, WorkerCredentialStatusRevoked)
	}

	// Revoked credentials drop out of the active tenant listing.
	list, err := store.ListTenant(context.Background(), pgTestTenantA)
	if err != nil {
		t.Fatalf("ListTenant() error = %v", err)
	}

	if len(list) != 0 {
		t.Fatalf("ListTenant() = %d credentials after revoke, want 0", len(list))
	}
}

func TestPostgresAuditStoreActorRecords(t *testing.T) {
	pool := newIdentityTestPool(t)
	seedTenant(t, pool, pgTestTenantA)

	store := NewPostgresAuditStore(pool)

	// Platform-scoped record (NULL tenant_id).
	platformRecord := AuditRecord{
		ActorType:    pgTestActorType,
		ActorID:      "key_platform_admin",
		ResourceType: "platform_api_key",
		ResourceID:   "00000000-0000-0000-0000-000000000014",
		Action:       pgTestActionAdd,
	}

	err := store.Record(context.Background(), platformRecord)
	if err != nil {
		t.Fatalf("Record(platform) error = %v", err)
	}

	// Tenant-scoped record.
	tenantRecord := AuditRecord{
		TenantID:     pgTestTenantA,
		ActorType:    pgTestActorType,
		ActorID:      "key_tenant_admin",
		ResourceType: "tenant_api_key",
		ResourceID:   "00000000-0000-0000-0000-000000000015",
		Action:       pgTestActionAdd,
	}

	err = store.Record(context.Background(), tenantRecord)
	if err != nil {
		t.Fatalf("Record(tenant) error = %v", err)
	}

	records, err := store.ListTenant(context.Background(), pgTestTenantA)
	if err != nil {
		t.Fatalf("ListTenant(A) error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("ListTenant(A) = %d records, want 1 (platform record must not leak)", len(records))
	}

	got := records[0]
	if got.TenantID != pgTestTenantA || got.ActorType != pgTestActorType || got.Action != pgTestActionAdd {
		t.Fatalf("record = %+v, want tenant=%q actor=%q action=%q", got, pgTestTenantA, pgTestActorType, pgTestActionAdd)
	}

	// The platform record persisted under NULL tenant_id, keeping it out of the
	// per-tenant view.
	var platformCount int

	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM config_audit_source WHERE tenant_id IS NULL`).Scan(&platformCount)
	if err != nil {
		t.Fatalf("count platform records: %v", err)
	}

	if platformCount != 1 {
		t.Fatalf("platform (NULL tenant) records = %d, want 1", platformCount)
	}
}

func TestPostgresAPIKeyStoreBootstrapCount(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresAPIKeyStore(pool, []byte(pgTestPepper))

	count, err := store.CountPlatformSystemAdmins(context.Background())
	if err != nil {
		t.Fatalf("CountPlatformSystemAdmins() error = %v", err)
	}

	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}

	record := APIKeyRecord{
		ID:         "00000000-0000-0000-0000-000000000030",
		ScopeType:  ScopePlatform,
		Role:       RoleSystemAdmin,
		Prefix:     "boot",
		SecretHash: HashAPIKeySecret("bootstrap_key", []byte(pgTestPepper)),
		Status:     APIKeyStatusActive,
	}

	err = store.Create(context.Background(), record)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	count, err = store.CountPlatformSystemAdmins(context.Background())
	if err != nil {
		t.Fatalf("CountPlatformSystemAdmins() after create error = %v", err)
	}

	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}
