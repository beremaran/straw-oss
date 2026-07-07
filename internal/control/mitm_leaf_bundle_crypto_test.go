package control

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

const (
	testMITMLeafKMSProvider = "test-kms"
	testMITMLeafTenantID    = "ten_123"
)

var (
	errFakeMITMLeafUnknownKey = errors.New("unknown key")
	errFakeMITMLeafAADMism    = errors.New("aad mismatch")
	errFakeMITMLeafAuth       = errors.New("ciphertext authentication failed")
)

func TestMITMLeafBundleFakeProviderRoundTripAndAAD(t *testing.T) {
	t.Parallel()

	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
	aad := testMITMLeafBundleAAD()
	plaintext := []byte("-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----")

	env, err := provider.EncryptMITMLeafBundle(context.Background(), testKeyA, aad, plaintext)
	if err != nil {
		t.Fatalf("EncryptMITMLeafBundle() error = %v", err)
	}
	if bytes.Contains(env.Ciphertext, []byte("PRIVATE KEY")) || bytes.Contains(env.Ciphertext, []byte("secret")) {
		t.Fatalf("ciphertext contains plaintext private key material: %q", env.Ciphertext)
	}

	got, err := provider.DecryptMITMLeafBundle(context.Background(), env, aad)
	if err != nil {
		t.Fatalf("DecryptMITMLeafBundle() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypt = %q, want %q", got, plaintext)
	}

	for _, tt := range []struct {
		name string
		edit func(*MITMLeafBundleAAD)
	}{
		{name: "tenant", edit: func(a *MITMLeafBundleAAD) { a.TenantID = "other-tenant" }},
		{name: "deployment", edit: func(a *MITMLeafBundleAAD) { a.DeploymentID = "other-deployment" }},
		{name: "sni", edit: func(a *MITMLeafBundleAAD) { a.NormalizedSNI = "other.example" }},
		{name: "ca identity", edit: func(a *MITMLeafBundleAAD) { a.CAIdentity = "other-ca" }},
		{name: "ca version", edit: func(a *MITMLeafBundleAAD) { a.CAVersion = "v2" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			changed := testMITMLeafBundleAAD()
			tt.edit(&changed)
			_, err := provider.DecryptMITMLeafBundle(context.Background(), env, changed)
			if err == nil {
				t.Fatal("DecryptMITMLeafBundle() AAD mismatch error = nil")
			}
		})
	}
}

func TestMITMLeafBundleFakeProviderRotationOverlap(t *testing.T) {
	t.Parallel()

	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{
		"old": []byte("old-secret"),
		"new": []byte("new-secret"),
	})
	aad := testMITMLeafBundleAAD()

	oldEnv, err := provider.EncryptMITMLeafBundle(context.Background(), "old", aad, []byte("bundle"))
	if err != nil {
		t.Fatalf("EncryptMITMLeafBundle(old) error = %v", err)
	}
	_, err = provider.DecryptMITMLeafBundle(context.Background(), oldEnv, aad)
	if err != nil {
		t.Fatalf("DecryptMITMLeafBundle(old overlap) error = %v", err)
	}

	delete(provider.keys, "old")
	_, err = provider.DecryptMITMLeafBundle(context.Background(), oldEnv, aad)
	if err == nil {
		t.Fatal("DecryptMITMLeafBundle(disabled old key) error = nil")
	}
}

func TestMITMLeafBundleProviderConfigRejectsUnsafeProviders(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"plaintext", "static-key", "deployment-key"} {
		_, err := NewMITMLeafBundleProviderConfig(name, "key")
		if !errors.Is(err, errMITMLeafKMSProviderUnsafe) {
			t.Fatalf("NewMITMLeafBundleProviderConfig(%q) error = %v, want unsafe provider", name, err)
		}
	}

	cfg, err := NewMITMLeafBundleProviderConfig("aws-kms", "arn:test")
	if err != nil {
		t.Fatalf("NewMITMLeafBundleProviderConfig() error = %v", err)
	}
	if cfg.ProviderName != "aws-kms" || cfg.KeyID != "arn:test" {
		t.Fatalf("config = %+v", cfg)
	}
}

type fakeMITMLeafBundleProvider struct {
	name string
	keys map[string][]byte
}

func newFakeMITMLeafBundleProvider(name string, keys map[string][]byte) *fakeMITMLeafBundleProvider {
	return &fakeMITMLeafBundleProvider{name: name, keys: keys}
}

func (p *fakeMITMLeafBundleProvider) EncryptMITMLeafBundle(_ context.Context, keyID string, aad MITMLeafBundleAAD, plaintext []byte) (MITMLeafBundleEnvelope, error) {
	key, ok := p.keys[keyID]
	if !ok {
		return MITMLeafBundleEnvelope{}, errFakeMITMLeafUnknownKey
	}
	aadBytes, err := aad.Bytes()
	if err != nil {
		return MITMLeafBundleEnvelope{}, err
	}
	nonce := make([]byte, 12)
	_, err = rand.Read(nonce)
	if err != nil {
		return MITMLeafBundleEnvelope{}, err
	}

	ciphertext := xorWithStream(plaintext, key, nonce, aadBytes)
	tag := hmac.New(sha256.New, key)
	tag.Write(aadBytes)
	tag.Write(nonce)
	tag.Write(ciphertext)

	return MITMLeafBundleEnvelope{
		ProviderName: p.name,
		KeyID:        keyID,
		KeyVersion:   keyID,
		Nonce:        nonce,
		Metadata:     map[string]string{"aad_sha256": hashHex(aadBytes), "tag": hex.EncodeToString(tag.Sum(nil))},
		Ciphertext:   ciphertext,
	}, nil
}

func (p *fakeMITMLeafBundleProvider) DecryptMITMLeafBundle(_ context.Context, env MITMLeafBundleEnvelope, aad MITMLeafBundleAAD) ([]byte, error) {
	key, ok := p.keys[env.KeyVersion]
	if !ok {
		return nil, errFakeMITMLeafUnknownKey
	}
	aadBytes, err := aad.Bytes()
	if err != nil {
		return nil, err
	}
	if env.Metadata["aad_sha256"] != hashHex(aadBytes) {
		return nil, errFakeMITMLeafAADMism
	}

	tag := hmac.New(sha256.New, key)
	tag.Write(aadBytes)
	tag.Write(env.Nonce)
	tag.Write(env.Ciphertext)
	if env.Metadata["tag"] != hex.EncodeToString(tag.Sum(nil)) {
		return nil, errFakeMITMLeafAuth
	}

	return xorWithStream(env.Ciphertext, key, env.Nonce, aadBytes), nil
}

func hashHex(raw []byte) string {
	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:])
}

func xorWithStream(in, key, nonce, aad []byte) []byte {
	out := make([]byte, len(in))
	var offset int
	for block := 0; offset < len(in); block++ {
		hash := sha256.New()
		hash.Write(key)
		hash.Write(nonce)
		hash.Write(aad)
		hash.Write([]byte{byte(block >> 8), byte(block)})
		stream := hash.Sum(nil)
		for _, b := range stream {
			if offset == len(in) {
				break
			}
			out[offset] = in[offset] ^ b
			offset++
		}
	}

	return out
}

func testMITMLeafBundleAAD() MITMLeafBundleAAD {
	return MITMLeafBundleAAD{
		TenantID:      testMITMLeafTenantID,
		DeploymentID:  "dep_123",
		NormalizedSNI: testExampleHost,
		CAIdentity:    "ca_123",
		CAVersion:     "v1",
	}
}
