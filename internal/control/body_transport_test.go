package control

import (
	"testing"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
)

const (
	bodyTransportTestThreshold = 1024
	bodyTransportTestInline    = 1024
)

func TestSelectBodyTransportMatchesSection18Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config.ControlBodyTransportConfig
		size     uint64
		want     BodyTransportKind
		wantCode ErrorCode
	}{
		{name: "small data frame", size: bodyTransportTestThreshold, want: BodyTransportDataFrames},
		{
			name: "large s3",
			cfg: config.ControlBodyTransportConfig{
				ObjectStorage: config.BodyObjectStorageConfig{Enabled: true},
			},
			size: bodyTransportTestThreshold + 1,
			want: BodyTransportS3BodyRef,
		},
		{
			name: "large direct stream",
			cfg: config.ControlBodyTransportConfig{
				DirectStream: config.BodyDirectStreamConfig{Enabled: true},
			},
			size: bodyTransportTestThreshold + 1,
			want: BodyTransportDirectStreamRef,
		},
		{name: "large disabled", size: bodyTransportTestThreshold + 1, wantCode: BodyTooLarge},
		{
			name: "object storage before direct stream",
			cfg: config.ControlBodyTransportConfig{
				ObjectStorage: config.BodyObjectStorageConfig{Enabled: true},
				DirectStream:  config.BodyDirectStreamConfig{Enabled: true},
			},
			size: bodyTransportTestThreshold + 1,
			want: BodyTransportS3BodyRef,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.LargeBodyThresholdBytes = bodyTransportTestThreshold

			got, perr := SelectBodyTransport(cfg, BodyTransportSelectionRequest{
				Direction:        BodyTransportDirectionRequest,
				SizeBytes:        tt.size,
				InlineLimitBytes: bodyTransportTestInline,
			})
			if tt.wantCode != 0 {
				if perr == nil || perr.Code != tt.wantCode {
					t.Fatalf("SelectBodyTransport() error = %#v, want %v", perr, tt.wantCode)
				}

				return
			}
			if perr != nil {
				t.Fatalf("SelectBodyTransport() error = %#v", perr)
			}
			if got.Transport != tt.want {
				t.Fatalf("transport = %q, want %q", got.Transport, tt.want)
			}
			if got.ResponseBodyMode != config.BodyResponseModeStreamThroughControlTeeObjectStorage {
				t.Fatalf("response mode = %q, want resolved mode", got.ResponseBodyMode)
			}
		})
	}
}

func TestValidateBodyRefFrameRejectsDisabledOrUnavailableVariants(t *testing.T) {
	t.Parallel()

	s3Frame := &strawpb.BodyRefFrame{
		Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{ObjectKey: "tenant/ten/request/req/request/nonce"}},
	}
	directFrame := &strawpb.BodyRefFrame{
		Ref: &strawpb.BodyRefFrame_DirectStream{
			DirectStream: &strawpb.DirectStreamRef{Endpoint: "http://stream.test", StreamId: "stream_123"},
		},
	}

	if perr := ValidateBodyRefFrame(config.ControlBodyTransportConfig{}, s3Frame); perr == nil || perr.Code != BodyRefUnavailable {
		t.Fatalf("ValidateBodyRefFrame(disabled s3) = %#v, want body_ref_unavailable", perr)
	}
	if perr := ValidateBodyRefFrame(config.ControlBodyTransportConfig{}, directFrame); perr == nil || perr.Code != BodyRefUnavailable {
		t.Fatalf("ValidateBodyRefFrame(disabled direct) = %#v, want body_ref_unavailable", perr)
	}

	cfg := config.ControlBodyTransportConfig{
		ObjectStorage: config.BodyObjectStorageConfig{Enabled: true},
		DirectStream:  config.BodyDirectStreamConfig{Enabled: true},
	}
	if perr := ValidateBodyRefFrame(cfg, s3Frame); perr != nil {
		t.Fatalf("ValidateBodyRefFrame(enabled s3) = %#v", perr)
	}
	if perr := ValidateBodyRefFrame(cfg, directFrame); perr != nil {
		t.Fatalf("ValidateBodyRefFrame(enabled direct) = %#v", perr)
	}
	if perr := ValidateBodyRefFrame(cfg, &strawpb.BodyRefFrame{}); perr == nil || perr.Code != ProtocolError {
		t.Fatalf("ValidateBodyRefFrame(empty ref) = %#v, want protocol_error", perr)
	}
}
