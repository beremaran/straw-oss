package strawpb_test

import (
	"bytes"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
)

func TestStreamFrameBodyRefCompiles(t *testing.T) {
	frame := &strawpb.StreamFrame{
		Payload: &strawpb.StreamFrame_BodyRef{
			BodyRef: &strawpb.BodyRefFrame{
				ExpectedSizeBytes: 42,
			},
		},
	}

	got := frame.GetBodyRef()
	if got == nil {
		t.Fatal("expected body ref payload")
	}
	if got.ExpectedSizeBytes != 42 {
		t.Fatalf("expected size 42, got %d", got.ExpectedSizeBytes)
	}
}

func TestAssignRequestCreditFieldsExist(t *testing.T) {
	req := &strawpb.AssignRequest{
		InitialUploadCreditBytes:   1,
		InitialDownloadCreditBytes: 2,
		MaxInflightUploadBytes:     3,
		MaxInflightDownloadBytes:   4,
	}

	if req.InitialUploadCreditBytes != 1 || req.InitialDownloadCreditBytes != 2 {
		t.Fatalf("unexpected initial credit fields: %+v", req)
	}
	if req.MaxInflightUploadBytes != 3 || req.MaxInflightDownloadBytes != 4 {
		t.Fatalf("unexpected inflight credit fields: %+v", req)
	}
}

func TestExecutorDelegatedDestinationResolutionKeepsWireNumber(t *testing.T) {
	if got := strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_EXECUTOR_DELEGATED.Number(); got != 3 {
		t.Fatalf("expected executor-delegated resolution wire number 3, got %d", got)
	}
	if !strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_EXECUTOR_DELEGATED.Valid() {
		t.Fatal("expected executor-delegated resolution mode to validate")
	}
}

func TestValidateRejectsUnknownEnums(t *testing.T) {
	t.Run("error response", func(t *testing.T) {
		msg := &strawpb.ErrorResponse{
			Category: strawpb.ErrorCategory(99),
			Code:     strawpb.ErrorCode(99),
		}

		err := msg.Validate()
		if err == nil {
			t.Fatal("expected unknown error enums to be rejected")
		}
	})

	t.Run("request start", func(t *testing.T) {
		msg := &strawpb.RequestStart{
			Mode:              strawpb.RequestMode(99),
			RedirectPolicy:    strawpb.RedirectPolicy(99),
			PolicyVersion:     "v1",
			Method:            "GET",
			Url:               "https://example.invalid/",
			DeadlineUnixMs:    1,
			DestinationPolicy: &strawpb.DestinationPolicy{},
		}

		err := msg.Validate()
		if err == nil {
			t.Fatal("expected unknown request enums to be rejected")
		}
	})
}

func TestFingerprintProfileFieldsAreAdditiveAndRoundTrip(t *testing.T) {
	t.Parallel()

	profiles := requireStringField(t, &strawpb.RegisterRequest{}, "supported_fingerprint_profiles", 19)
	executed := requireStringField(t, &strawpb.OutboundStartFrame{}, "executed_fingerprint_profile", 6)

	legacy := &strawpb.RegisterRequest{
		WorkerId:      "worker-1",
		CredentialId:  "cred-1",
		ProtocolMajor: 1,
	}
	legacyWire, err := proto.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy registration: %v", err)
	}
	if want := []byte{0x0a, 0x08, 'w', 'o', 'r', 'k', 'e', 'r', '-', '1', 0x1a, 0x06, 'c', 'r', 'e', 'd', '-', '1', 0x28, 0x01}; !bytes.Equal(legacyWire, want) {
		t.Fatalf("legacy registration bytes changed: got %x, want %x", legacyWire, want)
	}

	setStringList(t, legacy, profiles, []string{chrome120Profile})
	profileWire, err := proto.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal registration with profile capability: %v", err)
	}
	var decoded strawpb.RegisterRequest
	err = proto.Unmarshal(profileWire, &decoded)
	if err != nil {
		t.Fatalf("unmarshal registration with profile capability: %v", err)
	}
	if got := stringList(&decoded, profiles); !slices.Equal(got, []string{chrome120Profile}) {
		t.Fatalf("supported fingerprint profiles = %v, want [chrome_120]", got)
	}

	outbound := &strawpb.OutboundStartFrame{TargetHost: "example.test", Attempt: 1}
	outbound.ProtoReflect().Set(executed, protoreflect.ValueOfString(chrome120Profile))
	outboundWire, err := proto.Marshal(outbound)
	if err != nil {
		t.Fatalf("marshal outbound start frame: %v", err)
	}
	var decodedOutbound strawpb.OutboundStartFrame
	err = proto.Unmarshal(outboundWire, &decodedOutbound)
	if err != nil {
		t.Fatalf("unmarshal outbound start frame: %v", err)
	}
	if got := decodedOutbound.ProtoReflect().Get(executed).String(); got != chrome120Profile {
		t.Fatalf("executed fingerprint profile = %q, want chrome_120", got)
	}
}

func requireStringField(t *testing.T, message proto.Message, name string, number protoreflect.FieldNumber) protoreflect.FieldDescriptor {
	t.Helper()

	field := message.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(name))
	if field == nil {
		t.Fatalf("%s is missing required field %q", message.ProtoReflect().Descriptor().FullName(), name)
	}
	if field.Number() != number || field.Kind() != protoreflect.StringKind {
		t.Fatalf("%s = number %d kind %s, want number %d string", name, field.Number(), field.Kind(), number)
	}

	return field
}

func setStringList(t *testing.T, message proto.Message, field protoreflect.FieldDescriptor, values []string) {
	t.Helper()

	if !field.IsList() {
		t.Fatalf("%s is not repeated", field.FullName())
	}
	list := message.ProtoReflect().Mutable(field).List()
	list.Truncate(0)
	for _, value := range values {
		list.Append(protoreflect.ValueOfString(value))
	}
}

func stringList(message proto.Message, field protoreflect.FieldDescriptor) []string {
	list := message.ProtoReflect().Get(field).List()
	values := make([]string, list.Len())
	for i := range list.Len() {
		values[i] = list.Get(i).String()
	}

	return values
}
