package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
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
func BootstrapFromEnv(ctx context.Context, store APIKeyStore, bootstrapKey string, pepper []byte) (id string, created bool, err error) {
	if bootstrapKey == "" {
		return "", false, nil
	}

	count, err := store.CountPlatformSystemAdmins(ctx)
	if err != nil {
		return "", false, err
	}

	if count > 0 {
		return "", false, nil
	}

	prefix, err := ExtractKeyPrefix(bootstrapKey)
	if err != nil {
		return "", false, fmt.Errorf("bootstrap key too short: %w", err)
	}

	keyID, err := newRandomID("key")
	if err != nil {
		return "", false, err
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
	if err := store.Create(ctx, record); err != nil {
		return "", false, err
	}

	return keyID, true, nil
}

// newRandomID builds an opaque, non-guessable resource ID with the given
// prefix (e.g. "key_", "wcred_", "ten_").
func newRandomID(kind string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s_%s", kind, hex.EncodeToString(raw)), nil
}
