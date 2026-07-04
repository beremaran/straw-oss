package control

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

const resourceIDBytes = 16

// RFC 4122 version/variant bit patterns for a version 4 (random) UUID.
const (
	uuidVersionByte  = 6
	uuidVersionMask  = 0x0f
	uuidVersion4Bits = 0x40
	uuidVariantByte  = 8
	uuidVariantMask  = 0x3f
	uuidVariantBits  = 0x80
)

// BootstrapSystemAdminEnvVar is the environment variable Control reads on
// startup to seed the first platform system_admin API key. This is the P0
// bootstrap flow required by docs/planning/06 and
// docs/planning/26-config-management-api-surface.md: "The first platform
// key is bootstrapped through seed data, migration fixture, or environment
// bootstrap."
//
// If set, its value is treated as the plaintext key material for the first
// system_admin key and is hashed (never stored in plaintext). If unset and
// no active platform system_admin key already exists, BootstrapFromEnv is a
// no-op — operators must seed a key through a migration fixture or a
// one-off administrative insert before any platform-scoped operation is
// possible. This intentionally avoids printing or generating a key that
// would need to be logged.
const BootstrapSystemAdminEnvVar = "STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY"

// BootstrapFromEnv seeds the first platform system_admin API key from the
// value of bootstrapKey (typically read from BootstrapSystemAdminEnvVar by
// the caller) if, and only if, no active platform system_admin key exists
// yet. It returns the generated key ID and whether a key was created.
//
// This is idempotent-safe to call on every Control startup: once a
// system_admin key exists (whether created by this bootstrap or by a later
// /platform-api-keys call), subsequent calls are no-ops.
func BootstrapFromEnv(ctx context.Context, store APIKeyStore, bootstrapKey string, pepper []byte) (string, bool, error) {
	if bootstrapKey == "" {
		return "", false, nil
	}

	count, err := store.CountPlatformSystemAdmins(ctx)
	if err != nil {
		return "", false, fmt.Errorf("count platform system admins: %w", err)
	}

	if count > 0 {
		return "", false, nil
	}

	prefix, err := ExtractKeyPrefix(bootstrapKey)
	if err != nil {
		return "", false, fmt.Errorf("bootstrap key too short: %w", err)
	}

	keyID, err := newResourceID()
	if err != nil {
		return "", false, fmt.Errorf("generate bootstrap key id: %w", err)
	}

	record := APIKeyRecord{
		ID:            keyID,
		ScopeType:     ScopePlatform,
		Role:          RoleSystemAdmin,
		Prefix:        prefix,
		SecretHash:    HashAPIKeySecret(bootstrapKey, pepper),
		Status:        APIKeyStatusActive,
		CreatedAt:     time.Now().UTC(),
		ConfigVersion: 0,
	}

	err = store.Create(ctx, record)
	if err != nil {
		return "", false, fmt.Errorf("create bootstrap api key: %w", err)
	}

	return keyID, true, nil
}

// DevWorkerIDEnvVar and DevWorkerPublicEd25519EnvVar are the environment
// variables Control reads on startup to seed a dev worker credential
// matching a persistent egress identity key, so a docker-compose worker can
// register without a manual admin API call. Dev-only: production
// deployments provision worker credentials through the
// `/worker-credentials` admin API instead.
const (
	DevWorkerIDEnvVar            = "STRAW_BOOTSTRAP_WORKER_CREDENTIAL_ID"
	DevWorkerPublicEd25519EnvVar = "STRAW_BOOTSTRAP_WORKER_PUBLIC_KEY_ED25519_BASE64"
	devWorkerTenantScope         = "dev"
	devWorkerExecutorEgress      = "egress"
)

// BootstrapWorkerCredentialFromEnv seeds a single worker credential from
// credentialID/publicKeyBase64 (typically read from
// DevWorkerIDEnvVar/DevWorkerPublicEd25519EnvVar by
// the caller) if, and only if, no credential with that ID already exists. It
// returns whether a credential was created.
//
// This is idempotent-safe to call on every Control startup, mirroring
// BootstrapFromEnv: once the credential exists (whether created by this
// bootstrap or by a later /worker-credentials call), subsequent calls are
// no-ops. The seeded credential's TenantScope is a fixed "dev" placeholder;
// registration does not check tenant scope (only routing/eligibility does),
// so this is sufficient to let a worker register against the seeded
// credential without requiring a real provisioned tenant.
func BootstrapWorkerCredentialFromEnv(ctx context.Context, store WorkerCredentialStore, credentialID, publicKeyBase64 string) (bool, error) {
	if credentialID == "" || publicKeyBase64 == "" {
		return false, nil
	}

	_, err := store.Get(ctx, credentialID)
	if err == nil {
		return false, nil
	}

	if !errors.Is(err, ErrWorkerCredentialNotFound) {
		return false, fmt.Errorf("get worker credential: %w", err)
	}

	now := time.Now().UTC()
	record := WorkerCredential{
		ID:                     credentialID,
		Status:                 WorkerCredentialStatusActive,
		ExecutorType:           devWorkerExecutorEgress,
		PublicKeyEd25519Base64: publicKeyBase64,
		TenantScope:            []string{devWorkerTenantScope},
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	err = store.Create(ctx, record)
	if err != nil {
		return false, fmt.Errorf("create bootstrap worker credential: %w", err)
	}

	return true, nil
}

// newResourceID builds an opaque, non-guessable resource ID as an RFC 4122
// version 4 UUID string. The identity tables (tenants, api_keys,
// worker_credentials) use uuid primary keys, so IDs must be uuid-formatted to
// persist. See migrations/postgres/0001_init.sql.
func newResourceID() (string, error) {
	var raw [resourceIDBytes]byte

	_, err := rand.Read(raw[:])
	if err != nil {
		return "", fmt.Errorf("generate resource id: %w", err)
	}

	raw[uuidVersionByte] = (raw[uuidVersionByte] & uuidVersionMask) | uuidVersion4Bits
	raw[uuidVariantByte] = (raw[uuidVariantByte] & uuidVariantMask) | uuidVariantBits

	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}
