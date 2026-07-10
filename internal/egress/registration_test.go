package egress

import (
	"crypto/ed25519"
	"slices"
	"testing"
)

func TestBuildRegisterRequestAdvertisesFingerprintProfiles(t *testing.T) {
	t.Parallel()

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	caps := Capabilities{SupportedFingerprintProfiles: []string{chrome120FingerprintProfile}}
	req, err := BuildRegisterRequest(Identity{
		WorkerID:     "worker-1",
		CredentialID: "wcred_1",
		ExecutorType: "egress",
		PrivateKey:   priv,
	}, caps)
	if err != nil {
		t.Fatalf("BuildRegisterRequest: %v", err)
	}

	caps.SupportedFingerprintProfiles[0] = "mutated_after_build"
	if got := req.GetSupportedFingerprintProfiles(); !slices.Equal(got, []string{chrome120FingerprintProfile}) {
		t.Fatalf("advertised fingerprint profiles = %v, want [chrome_120]", got)
	}
}
