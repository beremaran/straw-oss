package control

import (
	"strconv"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
)

// BodyTransportDirection identifies whether a body is a request or response body.
type BodyTransportDirection string

// Body transport directions.
const (
	BodyTransportDirectionRequest  BodyTransportDirection = "request"
	BodyTransportDirectionResponse BodyTransportDirection = "response"
)

// BodyTransportKind is one of the Section 18 large-body transports.
type BodyTransportKind string

// Large-body transports from docs/planning/18-large-body-transport-p2.md.
const (
	BodyTransportDataFrames      BodyTransportKind = "data_frames"
	BodyTransportS3BodyRef       BodyTransportKind = "s3_body_ref"
	BodyTransportDirectStreamRef BodyTransportKind = "direct_stream_ref"
)

// BodyTransportSelectionRequest describes a body to route through the Section 18 table.
type BodyTransportSelectionRequest struct {
	Direction        BodyTransportDirection
	SizeBytes        uint64
	InlineLimitBytes uint64
}

// BodyTransportSelection is the chosen transport plus the resolved response-body mode.
type BodyTransportSelection struct {
	Transport        BodyTransportKind
	ResponseBodyMode string
}

// SelectBodyTransport applies the P2 Section 18 transport table. The caller's
// inline limit is already validated against the NATS frame/message payload
// ceiling at Control startup.
func SelectBodyTransport(cfg config.ControlBodyTransportConfig, req BodyTransportSelectionRequest) (BodyTransportSelection, *PipelineError) {
	cfg = cfg.Normalized()
	if req.InlineLimitBytes == 0 {
		req.InlineLimitBytes = cfg.LargeBodyThresholdBytes
	}

	if req.SizeBytes <= cfg.LargeBodyThresholdBytes && req.SizeBytes <= req.InlineLimitBytes {
		return BodyTransportSelection{
			Transport:        BodyTransportDataFrames,
			ResponseBodyMode: cfg.ResponseBodyMode,
		}, nil
	}

	if cfg.ObjectStorage.Enabled {
		return BodyTransportSelection{
			Transport:        BodyTransportS3BodyRef,
			ResponseBodyMode: cfg.ResponseBodyMode,
		}, nil
	}

	if cfg.DirectStream.Enabled {
		return BodyTransportSelection{
			Transport:        BodyTransportDirectStreamRef,
			ResponseBodyMode: cfg.ResponseBodyMode,
		}, nil
	}

	return BodyTransportSelection{}, bodyTooLargeTransportError(req.Direction, req.InlineLimitBytes)
}

// ValidateBodyRefFrame rejects BodyRef frames whose transport is disabled by
// config or whose reference is unusable, mapping either to body_ref_unavailable.
func ValidateBodyRefFrame(cfg config.ControlBodyTransportConfig, frame *strawpb.BodyRefFrame) *PipelineError {
	if frame == nil {
		return &PipelineError{Code: ProtocolError}
	}

	cfg = cfg.Normalized()

	switch ref := frame.GetRef().(type) {
	case *strawpb.BodyRefFrame_S3:
		if !s3RefUsable(cfg, ref.S3) {
			return bodyRefUnavailableError(BodyTransportDirectionRequest, BodyTransportS3BodyRef)
		}
	case *strawpb.BodyRefFrame_DirectStream:
		if !directStreamRefUsable(cfg, ref.DirectStream) {
			return bodyRefUnavailableError(BodyTransportDirectionRequest, BodyTransportDirectStreamRef)
		}
	default:
		return &PipelineError{Code: ProtocolError}
	}

	return nil
}

func s3RefUsable(cfg config.ControlBodyTransportConfig, ref *strawpb.S3BodyRef) bool {
	return cfg.ObjectStorage.Enabled && ref != nil && (ref.GetObjectKey() != "" || ref.GetSignedUrl() != "")
}

func directStreamRefUsable(cfg config.ControlBodyTransportConfig, ref *strawpb.DirectStreamRef) bool {
	return cfg.DirectStream.Enabled && ref != nil && ref.GetEndpoint() != "" && ref.GetStreamId() != ""
}

func bodyTooLargeTransportError(direction BodyTransportDirection, limitBytes uint64) *PipelineError {
	return &PipelineError{
		Code: BodyTooLarge,
		Details: map[string]string{
			errorDetailDirectionKey:  string(direction),
			errorDetailLimitBytesKey: strconv.FormatUint(limitBytes, 10),
		},
	}
}

func bodyRefUnavailableError(direction BodyTransportDirection, transport BodyTransportKind) *PipelineError {
	return &PipelineError{
		Code: BodyRefUnavailable,
		Details: map[string]string{
			errorDetailDirectionKey: string(direction),
			"transport":             string(transport),
		},
	}
}
