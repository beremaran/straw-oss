package control

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// keySecretBytes is the number of random bytes used for the key secret body.
// 32 bytes = 256 bits of entropy, exceeding the 128-bit minimum and meeting
// the 192/256-bit preferred range from docs/planning/27-security-controls.md.
const keySecretBytes = 32

// keyPrefixRunes is the number of characters of the generated key (including
// the "sk_live_" literal) that are stored and shown as the visible prefix
// used for candidate lookup.
const keyPrefixRunes = 12

// keyLiteralPrefix is prepended to every generated API key.
const keyLiteralPrefix = "sk_live_"

// ErrMalformedAPIKey is returned when a presented API key does not match
// the expected shape (too short to contain a prefix).
var ErrMalformedAPIKey = errors.New("malformed api key")

// GeneratedAPIKey holds the plaintext key material produced at creation
// time. The plaintext Secret is shown to the caller exactly once; it is
// never persisted.
type GeneratedAPIKey struct {
	// Secret is the full plaintext key (e.g. "sk_live_abcd...").
	Secret string
	// Prefix is the visible, non-secret prefix used for lookup.
	Prefix string
}

// GenerateAPIKey creates a new high-entropy API key. The returned Secret
// must be surfaced to the caller once and never logged or stored.
func GenerateAPIKey() (GeneratedAPIKey, error) {
	raw := make([]byte, keySecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return GeneratedAPIKey{}, err
	}

	body := base64.RawURLEncoding.EncodeToString(raw)
	secret := keyLiteralPrefix + body

	prefix, err := ExtractKeyPrefix(secret)
	if err != nil {
		return GeneratedAPIKey{}, err
	}

	return GeneratedAPIKey{Secret: secret, Prefix: prefix}, nil
}

// ExtractKeyPrefix returns the visible lookup prefix for a full API key.
func ExtractKeyPrefix(fullKey string) (string, error) {
	if len(fullKey) < keyPrefixRunes {
		return "", ErrMalformedAPIKey
	}

	return fullKey[:keyPrefixRunes], nil
}

// HashAPIKeySecret computes the server-side hash for a full plaintext key
// using HMAC-SHA256 with an optional server-side pepper. The result is
// hex-encoded for storage.
func HashAPIKeySecret(fullKey string, pepper []byte) string {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(fullKey))

	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyAPIKeySecret reports whether the plaintext key matches the stored
// hash, using a constant-time comparison to avoid timing side channels.
func VerifyAPIKeySecret(fullKey, storedHash string, pepper []byte) bool {
	computed := HashAPIKeySecret(fullKey, pepper)
	// Decode both to bytes for subtle.ConstantTimeCompare; hex is safe to
	// decode with equal-length assumptions checked first.
	if len(computed) != len(storedHash) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// BearerToken extracts the raw token from an Authorization header value of
// the form "Bearer <token>". It returns an error if the header is missing
// or malformed.
func BearerToken(authorizationHeader string) (string, error) {
	const schemePrefix = "Bearer "

	if authorizationHeader == "" {
		return "", errors.New("missing authorization header")
	}

	if !strings.HasPrefix(authorizationHeader, schemePrefix) {
		return "", errors.New("authorization header must use Bearer scheme")
	}

	token := strings.TrimPrefix(authorizationHeader, schemePrefix)
	if token == "" {
		return "", errors.New("empty bearer token")
	}

	return token, nil
}
