package control

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/beremaran/straw/v2/internal/postgresx"
	"github.com/beremaran/straw/v2/migrations"
)

// These tests exercise the Postgres-backed identity stores against a real
// database. They are skipped unless STRAW_TEST_POSTGRES_DSN points at a
// reachable Postgres instance; the harness applies migrations itself, so the
// target database may be empty. Each test truncates the identity tables first,
// so the suite is idempotent across reruns; because of that shared-state reset
// the tests are intentionally not run in parallel.
//
// Because the harness truncates tables, it refuses to run against a database
// that is not explicitly designated for tests: the DSN's database name must
// end in _test (e.g. straw_test). This guard exists because a run pointed at
// the compose stack's live straw database once wiped its seeded state and
// leaked test fixtures into it. See deploy/docker/README.md for the
// sanctioned local test-database setup.
const (
	pgTestTenantA       = "11111111-1111-1111-1111-111111111111"
	pgTestTenantB       = "22222222-2222-2222-2222-222222222222"
	pgTestActorType     = "api_key"
	pgTestActionAdd     = "create"
	pgTestPepper        = "test-pepper"
	pgTestExecutorType  = "egress"
	pgTestTenantRenamed = "Renamed"
	pgTestPublicKey     = "YWJjZA=="
	pgTestFastTag       = "fast"
	pgTestRegionAUEast  = "au-east-1"
)

// checkTestDatabaseDSN rejects any DSN whose database name does not end in
// _test. The harness truncates identity tables, so it must never run against
// a database that is in real use. Failing (not skipping) keeps a
// misconfigured CI run loud instead of silently green-with-skips.
func checkTestDatabaseDSN(dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse STRAW_TEST_POSTGRES_DSN: %w", err)
	}

	db := cfg.ConnConfig.Database
	if !strings.HasSuffix(db, "_test") {
		return fmt.Errorf(
			"STRAW_TEST_POSTGRES_DSN targets database %q; the harness truncates tables, so it only runs against a database whose name ends in _test (e.g. straw_test) — see deploy/docker/README.md for setup",
			db)
	}

	return nil
}

func newIdentityTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("STRAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("STRAW_TEST_POSTGRES_DSN not set")
	}

	guardErr := checkTestDatabaseDSN(dsn)
	if guardErr != nil {
		t.Fatalf("refusing to run Postgres-backed tests: %v", guardErr)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}

	t.Cleanup(pool.Close)

	// Bootstrap the schema first so the harness works against a fresh, empty
	// database (only STRAW_TEST_POSTGRES_DSN is required). ApplyMigrations is
	// idempotent, so this is a no-op when the schema already exists; without it
	// the TRUNCATE below fails on a clean database with "relation ... does not
	// exist".
	err = postgresx.ApplyMigrations(context.Background(), pool, migrations.Postgres)
	if err != nil {
		t.Fatalf("bootstrap migrations: %v", err)
	}

	_, err = pool.Exec(context.Background(),
		`TRUNCATE tenants, worker_admin_state, worker_credentials, api_keys, config_audit_source RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate control tables: %v", err)
	}

	// The CASCADE above also empties fingerprint_profiles (it references
	// tenants), so re-apply the idempotent migrations to restore the seeded
	// built-in global profiles the production binary always has.
	err = postgresx.ApplyMigrations(context.Background(), pool, migrations.Postgres)
	if err != nil {
		t.Fatalf("reapply migrations: %v", err)
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

func TestPostgresTenantStoreUpdatePersistsNameAndCeiling(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresTenantStore(pool)
	ctx := context.Background()

	tenant := Tenant{ID: pgTestTenantA, Name: "Original", Status: TenantStatusActive}

	err := store.Create(ctx, tenant)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := store.Update(ctx, Tenant{
		ID:                   pgTestTenantA,
		Name:                 pgTestTenantRenamed,
		Status:               TenantStatusSuspended,
		DefaultTimeoutMs:     5000,
		MaxTimeoutMs:         9000,
		MetadataQueryStorage: MetadataStorageHash,
		MetadataPathStorage:  MetadataStorageDrop,
		RateLimitCeiling:     &RateLimitCeiling{WindowSeconds: 60, MaxRequests: 6000},
	}, 0)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.Name != pgTestTenantRenamed || updated.Status != TenantStatusSuspended {
		t.Fatalf("got name=%q status=%q, want name=Renamed status=suspended", updated.Name, updated.Status)
	}
	if updated.DefaultTimeoutMs != 5000 || updated.MaxTimeoutMs != 9000 ||
		updated.MetadataQueryStorage != MetadataStorageHash || updated.MetadataPathStorage != MetadataStorageDrop {
		t.Fatalf("tenant P0 fields = %+v, want timeout/storage policy persisted", updated)
	}

	if updated.RateLimitCeiling == nil || updated.RateLimitCeiling.WindowSeconds != 60 || updated.RateLimitCeiling.MaxRequests != 6000 {
		t.Fatalf("rate_limit_ceiling = %+v, want {60 6000}", updated.RateLimitCeiling)
	}

	if updated.ConfigVersion != 1 {
		t.Fatalf("config_version = %d, want 1", updated.ConfigVersion)
	}

	got, err := store.Get(ctx, pgTestTenantA)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Name != pgTestTenantRenamed || got.DefaultTimeoutMs != 5000 || got.MetadataPathStorage != MetadataStorageDrop || got.RateLimitCeiling == nil {
		t.Fatalf("reloaded tenant = %+v, want persisted P0 fields", got)
	}
}

func TestPostgresTenantStoreUpdateVersionConflict(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresTenantStore(pool)
	ctx := context.Background()

	err := store.Create(ctx, Tenant{ID: pgTestTenantA, Name: "T", Status: TenantStatusActive})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = store.Update(ctx, Tenant{ID: pgTestTenantA, Name: "T2", Status: TenantStatusActive}, 5)
	if !errors.Is(err, ErrTenantVersionConflict) {
		t.Fatalf("Update() error = %v, want ErrTenantVersionConflict", err)
	}
}

func TestPostgresTenantStoreUpdateMissingTenantNotFound(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresTenantStore(pool)

	_, err := store.Update(context.Background(), Tenant{ID: pgTestTenantA, Name: "T", Status: TenantStatusActive}, 0)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("Update() error = %v, want ErrTenantNotFound", err)
	}
}

func TestPostgresTenantStoreSoftDeleteThenUpdateNotFound(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresTenantStore(pool)
	ctx := context.Background()

	err := store.Create(ctx, Tenant{ID: pgTestTenantA, Name: "T", Status: TenantStatusActive})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	deleted, err := store.SoftDelete(ctx, pgTestTenantA)
	if err != nil {
		t.Fatalf("SoftDelete() error = %v", err)
	}

	if deleted.Status != TenantStatusDeleted || deleted.DeletedAt == nil {
		t.Fatalf("deleted tenant = %+v, want status=deleted and deleted_at set", deleted)
	}

	_, err = store.SoftDelete(ctx, pgTestTenantA)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("second SoftDelete() error = %v, want ErrTenantNotFound", err)
	}

	_, err = store.Update(ctx, Tenant{ID: pgTestTenantA, Name: "T", Status: TenantStatusActive}, 1)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("Update() on deleted tenant error = %v, want ErrTenantNotFound", err)
	}
}

func TestPostgresTenantStoreListOrdersByCreatedAtDesc(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresTenantStore(pool)
	ctx := context.Background()

	err := store.Create(ctx, Tenant{ID: pgTestTenantA, Name: "A", Status: TenantStatusActive})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	err = store.Create(ctx, Tenant{ID: pgTestTenantB, Name: "B", Status: TenantStatusActive})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.List(ctx, 50, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(got) != 2 || got[0].ID != pgTestTenantB || got[1].ID != pgTestTenantA {
		t.Fatalf("List() = %+v, want [B, A]", got)
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
		PublicKeyEd25519Base64: pgTestPublicKey,
		TenantScope:            []string{pgTestTenantA},
		AllowedPools:           []AllowedPool{{TenantID: pgTestTenantA, PoolID: "pool_1"}},
		AllowedCapabilities: WorkerCapabilities{
			Tags:                  []string{pgTestFastTag},
			Countries:             []string{"AU"},
			Regions:               []string{pgTestRegionAUEast},
			IPTypes:               []string{ipTypeDatacenter},
			SupportedIngressModes: []string{IngressTypeREST, IngressTypeHTTPProxy},
		},
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
	if got := list[0].AllowedCapabilities; len(got.SupportedIngressModes) != 2 || got.SupportedIngressModes[0] != IngressTypeREST || got.SupportedIngressModes[1] != IngressTypeHTTPProxy {
		t.Fatalf("allowed capabilities = %+v, want ingress modes rest/http_proxy", got)
	}

	got, err := store.Get(context.Background(), cred.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(got.AllowedCapabilities.SupportedIngressModes) != 2 || got.AllowedCapabilities.SupportedIngressModes[1] != IngressTypeHTTPProxy {
		t.Fatalf("Get().AllowedCapabilities = %+v, want persisted ingress modes", got.AllowedCapabilities)
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
		PublicKeyEd25519Base64: pgTestPublicKey,
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

func TestPostgresWorkerCredentialStoreRevokeMissingNotFound(t *testing.T) {
	pool := newIdentityTestPool(t)
	store := NewPostgresWorkerCredentialStore(pool)

	// Revoking a credential that was never created must map to
	// ErrWorkerCredentialNotFound (so the handler returns 404, not 500),
	// mirroring Get's ErrNoRows handling.
	_, err := store.Revoke(context.Background(), "00000000-0000-0000-0000-000000000099", time.Now().UTC())
	if !errors.Is(err, ErrWorkerCredentialNotFound) {
		t.Fatalf("Revoke(missing) error = %v, want ErrWorkerCredentialNotFound", err)
	}

	// An already-revoked credential (UPDATE ... WHERE status='active' matches
	// zero rows) must also surface as not found rather than an opaque wrap.
	cred := WorkerCredential{
		ID:                     "00000000-0000-0000-0000-000000000022",
		Status:                 WorkerCredentialStatusActive,
		ExecutorType:           pgTestExecutorType,
		PublicKeyEd25519Base64: pgTestPublicKey,
		TenantScope:            []string{pgTestTenantA},
	}

	err = store.Create(context.Background(), cred)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = store.Revoke(context.Background(), cred.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("first Revoke() error = %v", err)
	}

	_, err = store.Revoke(context.Background(), cred.ID, time.Now().UTC())
	if !errors.Is(err, ErrWorkerCredentialNotFound) {
		t.Fatalf("second Revoke() error = %v, want ErrWorkerCredentialNotFound", err)
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
