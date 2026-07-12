package strawpb_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
)

type registrationFixture struct {
	SeedHex      string `json:"seed_hex"`
	PublicKeyHex string `json:"public_key_hex"`
	PayloadHex   string `json:"payload_hex"`
	SignatureHex string `json:"signature_hex"`
	Request      struct {
		WorkerID                     string   `json:"worker_id"`
		CredentialID                 string   `json:"credential_id"`
		ExecutorType                 string   `json:"executor_type"`
		ProtocolMajor                uint32   `json:"protocol_major"`
		ProtocolMinor                uint32   `json:"protocol_minor"`
		NonceHex                     string   `json:"nonce_hex"`
		IssuedAtUnixMS               int64    `json:"issued_at_unix_ms"`
		SupportedFingerprintProfiles []string `json:"supported_fingerprint_profiles"`
	} `json:"request"`
}

func TestRegistrationSigningFixture(t *testing.T) {
	var fixture registrationFixture
	readFixture(t, "registration-signing.json", &fixture)
	seed := decodeHex(t, fixture.SeedHex)
	nonce := decodeHex(t, fixture.Request.NonceHex)
	req := &strawpb.RegisterRequest{
		WorkerId: fixture.Request.WorkerID, CredentialId: fixture.Request.CredentialID,
		ExecutorType: fixture.Request.ExecutorType, ProtocolMajor: fixture.Request.ProtocolMajor,
		ProtocolMinor: fixture.Request.ProtocolMinor, Nonce: nonce,
		IssuedAtUnixMs:               fixture.Request.IssuedAtUnixMS,
		SupportedFingerprintProfiles: fixture.Request.SupportedFingerprintProfiles,
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	if got, want := strawpb.RegistrationSigningPayload(req), decodeHex(t, fixture.PayloadHex); !bytes.Equal(got, want) {
		t.Fatalf("signing payload = %x, want %x", got, want)
	}
	if got, want := privateKey.Public().(ed25519.PublicKey), decodeHex(t, fixture.PublicKeyHex); !bytes.Equal(got, want) {
		t.Fatalf("public key = %x, want %x", got, want)
	}
	signature := strawpb.SignRegistration(privateKey, req)
	if want := decodeHex(t, fixture.SignatureHex); !bytes.Equal(signature, want) {
		t.Fatalf("signature = %x, want %x", signature, want)
	}
	if !strawpb.VerifyRegistrationSignature(privateKey.Public().(ed25519.PublicKey), req, signature) {
		t.Fatal("fixture signature did not verify")
	}
}

func TestEnvelopeAndUnknownValueFixture(t *testing.T) {
	var fixture struct {
		RequestID            string `json:"request_id"`
		DeploymentID         string `json:"deployment_id"`
		ProtocolMajor        uint32 `json:"protocol_major"`
		ProtocolMinor        uint32 `json:"protocol_minor"`
		Attempt              uint32 `json:"attempt"`
		DeterministicWireHex string `json:"deterministic_wire_hex"`
		UnknownFieldWireHex  string `json:"unknown_field_wire_hex"`
		UnknownEnumNumber    int32  `json:"unknown_enum_number"`
	}
	readFixture(t, "envelope.json", &fixture)
	envelope := &strawpb.Envelope{
		RequestId: fixture.RequestID, DeploymentId: fixture.DeploymentID,
		ProtocolMajor: fixture.ProtocolMajor, ProtocolMinor: fixture.ProtocolMinor, Attempt: fixture.Attempt,
		Payload: &strawpb.Envelope_AssignAck{AssignAck: &strawpb.AssignAck{Code: strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED}},
	}
	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if want := decodeHex(t, fixture.DeterministicWireHex); !bytes.Equal(wire, want) {
		t.Fatalf("wire = %x, want %x", wire, want)
	}
	var decoded strawpb.Envelope
	unknownWire := decodeHex(t, fixture.UnknownFieldWireHex)
	err = proto.Unmarshal(unknownWire, &decoded)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := proto.MarshalOptions{Deterministic: true}.Marshal(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(roundTrip, unknownWire) {
		t.Fatalf("unknown field was not preserved: %x", roundTrip)
	}
	unknownAck := &strawpb.AssignAck{Code: strawpb.AssignAckCode(fixture.UnknownEnumNumber)}
	if unknownAck.Validate() == nil {
		t.Fatal("unknown enum value unexpectedly validated")
	}
}

func readFixture(t *testing.T, name string, target any) {
	t.Helper()
	var (
		data []byte
		err  error
	)
	switch name {
	case "registration-signing.json":
		data, err = os.ReadFile("../../../../conformance/fixtures/v1/registration-signing.json")
	case "envelope.json":
		data, err = os.ReadFile("../../../../conformance/fixtures/v1/envelope.json")
	default:
		t.Fatalf("unknown fixture %q", name)
	}
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	err = json.Unmarshal(data, target)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

func decodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}

	return decoded
}
