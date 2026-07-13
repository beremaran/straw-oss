package egress

import (
	"crypto/ed25519"
	"slices"
	"testing"
)

func TestFingerprintProfileRegistrationAdvertisesCapabilities(t *testing.T) {
	t.Parallel()

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	caps := Capabilities{SupportedFingerprintProfiles: SupportedFingerprintProfiles()}
	req, err := BuildRegisterRequest(Identity{
		WorkerID:     "worker-1",
		CredentialID: "wcred_1",
		ExecutorType: "egress",
		PrivateKey:   priv,
	}, caps)
	if err != nil {
		t.Fatalf("BuildRegisterRequest: %v", err)
	}

	want := SupportedFingerprintProfiles()
	caps.SupportedFingerprintProfiles[0] = "mutated_after_build"
	if got := req.GetSupportedFingerprintProfiles(); !slices.Equal(got, want) {
		t.Fatalf("advertised fingerprint profiles = %v, want complete catalogue", got)
	}
}
