package strawpb_test

import (
	"testing"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
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

func TestValidateRejectsUnknownEnums(t *testing.T) {
	t.Run("error response", func(t *testing.T) {
		msg := &strawpb.ErrorResponse{
			Category: strawpb.ErrorCategory(99),
			Code:     strawpb.ErrorCode(99),
		}

		if err := msg.Validate(); err == nil {
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

		if err := msg.Validate(); err == nil {
			t.Fatal("expected unknown request enums to be rejected")
		}
	})
}
