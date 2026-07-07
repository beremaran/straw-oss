package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	errMITMLeafKMSProviderRequired = errors.New("mitm leaf kms provider is required")
	errMITMLeafKMSKeyIDRequired    = errors.New("mitm leaf kms key id is required")
	errMITMLeafKMSProviderUnsafe   = errors.New("mitm leaf kms provider must not be plaintext or static-key")
	errMITMLeafBundleAADIncomplete = errors.New("mitm leaf bundle aad is incomplete")
)

// MITMLeafBundleKMSProvider is the minimal boundary task 04 uses to encrypt
// generated leaf bundles before they leave Control memory.
type MITMLeafBundleKMSProvider interface {
	EncryptMITMLeafBundle(ctx context.Context, keyID string, aad MITMLeafBundleAAD, plaintext []byte) (MITMLeafBundleEnvelope, error)
	DecryptMITMLeafBundle(ctx context.Context, envelope MITMLeafBundleEnvelope, aad MITMLeafBundleAAD) ([]byte, error)
}

// MITMLeafBundleProviderConfig is the validated provider/key selection from
// control.server config. It is intentionally not an encrypting provider.
type MITMLeafBundleProviderConfig struct {
	ProviderName string
	KeyID        string
}

// MITMLeafBundleEnvelope is the encrypted stored form of a generated MITM leaf
// certificate bundle.
type MITMLeafBundleEnvelope struct {
	ProviderName string            `json:"provider_name"`
	KeyID        string            `json:"key_id"`
	KeyVersion   string            `json:"key_version"`
	Nonce        []byte            `json:"nonce,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Ciphertext   []byte            `json:"ciphertext"`
}

// MITMLeafBundleAAD scopes encrypted leaf bundles to one tenant, deployment,
// normalized SNI, and CA identity/version.
type MITMLeafBundleAAD struct {
	TenantID      string `json:"tenant_id"`
	DeploymentID  string `json:"deployment_id"`
	NormalizedSNI string `json:"normalized_sni"`
	CAIdentity    string `json:"ca_identity"`
	CAVersion     string `json:"ca_version"`
}

// NewMITMLeafBundleProviderConfig validates the KMS-compatible provider name
// and key id without constructing a vendor-specific KMS client.
func NewMITMLeafBundleProviderConfig(providerName, keyID string) (MITMLeafBundleProviderConfig, error) {
	providerName = strings.TrimSpace(providerName)
	keyID = strings.TrimSpace(keyID)

	if providerName == "" {
		return MITMLeafBundleProviderConfig{}, errMITMLeafKMSProviderRequired
	}

	if keyID == "" {
		return MITMLeafBundleProviderConfig{}, errMITMLeafKMSKeyIDRequired
	}

	switch strings.ToLower(providerName) {
	case "plaintext", "static", "static-key", "deployment-key":
		return MITMLeafBundleProviderConfig{}, errMITMLeafKMSProviderUnsafe
	}

	return MITMLeafBundleProviderConfig{ProviderName: providerName, KeyID: keyID}, nil
}

// Bytes returns the canonical JSON AAD bytes passed to the configured provider.
func (a MITMLeafBundleAAD) Bytes() ([]byte, error) {
	if a.TenantID == "" || a.DeploymentID == "" || a.NormalizedSNI == "" || a.CAIdentity == "" || a.CAVersion == "" {
		return nil, errMITMLeafBundleAADIncomplete
	}

	raw, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("marshal mitm leaf bundle aad: %w", err)
	}

	return raw, nil
}
