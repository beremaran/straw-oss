package strawpb_test

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const (
	profileCapabilityProtocolMinor uint32 = 1
	chrome120Profile                      = "chrome_120"
	additionalProfile                     = "additional_profile"
)

func TestRegistrationSigningPayloadPreservesLegacyBytesUntilNewMinorCapabilities(t *testing.T) {
	t.Parallel()

	req := signingRequest(0)
	legacyPayload := strawpb.RegistrationSigningPayload(req)
	wantLegacy := []byte("straw.v1.register\nworker-1\ncred-1\negress\n1.0\n5:nonce\n1700000000000")
	if !bytes.Equal(legacyPayload, wantLegacy) {
		t.Fatalf("legacy signing payload = %q, want %q", legacyPayload, wantLegacy)
	}

	profiles := requireStringField(t, req, "supported_fingerprint_profiles", 19)
	setStringList(t, req, profiles, []string{chrome120Profile})
	if got := strawpb.RegistrationSigningPayload(req); !bytes.Equal(got, legacyPayload) {
		t.Fatalf("legacy-minor profile capability changed signing bytes: got %q, want %q", got, legacyPayload)
	}

	req.ProtocolMinor = profileCapabilityProtocolMinor
	if got := strawpb.RegistrationSigningPayload(req); bytes.Equal(got, legacyPayload) {
		t.Fatal("new-minor profile capability did not change signing bytes")
	}
}

func TestRegistrationSignatureCanonicalizesProfileOrderAndRejectsMutation(t *testing.T) {
	t.Parallel()

	first := signingRequest(profileCapabilityProtocolMinor)
	second := signingRequest(profileCapabilityProtocolMinor)
	profiles := requireStringField(t, first, "supported_fingerprint_profiles", 19)
	setStringList(t, first, profiles, []string{chrome120Profile, additionalProfile})
	setStringList(t, second, profiles, []string{additionalProfile, chrome120Profile})

	firstPayload := strawpb.RegistrationSigningPayload(first)
	secondPayload := strawpb.RegistrationSigningPayload(second)
	if !bytes.Equal(firstPayload, secondPayload) {
		t.Fatalf("profile ordering changed deterministic signing payload: %q != %q", firstPayload, secondPayload)
	}

	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	firstSignature := strawpb.SignRegistration(privateKey, first)
	if !bytes.Equal(firstSignature, strawpb.SignRegistration(privateKey, second)) {
		t.Fatal("equivalent capability sets produced different deterministic signatures")
	}
	if !strawpb.VerifyRegistrationSignature(publicKey, second, firstSignature) {
		t.Fatal("signature for an equivalent canonical capability set did not verify")
	}

	setStringList(t, second, profiles, []string{additionalProfile, "mutated_profile"})
	if strawpb.VerifyRegistrationSignature(publicKey, second, firstSignature) {
		t.Fatal("signature remained valid after supported profile mutation")
	}
}

func signingRequest(protocolMinor uint32) *strawpb.RegisterRequest {
	return &strawpb.RegisterRequest{
		WorkerId:       "worker-1",
		CredentialId:   "cred-1",
		ExecutorType:   "egress",
		ProtocolMajor:  1,
		ProtocolMinor:  protocolMinor,
		Nonce:          []byte("nonce"),
		IssuedAtUnixMs: 1_700_000_000_000,
	}
}
