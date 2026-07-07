package control

import (
	"strconv"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
)

type BodyTransportDirection string

const (
	BodyTransportDirectionRequest  BodyTransportDirection = "request"
	BodyTransportDirectionResponse BodyTransportDirection = "response"
)

type BodyTransportKind string

const (
	BodyTransportDataFrames      BodyTransportKind = "data_frames"
	BodyTransportS3BodyRef       BodyTransportKind = "s3_body_ref"
	BodyTransportDirectStreamRef BodyTransportKind = "direct_stream_ref"
)

type BodyTransportSelectionRequest struct {
	Direction        BodyTransportDirection
	SizeBytes        uint64
	InlineLimitBytes uint64
}

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

func ValidateBodyRefFrame(cfg config.ControlBodyTransportConfig, frame *strawpb.BodyRefFrame) *PipelineError {
	if frame == nil {
		return &PipelineError{Code: ProtocolError}
	}

	cfg = cfg.Normalized()

	switch ref := frame.GetRef().(type) {
	case *strawpb.BodyRefFrame_S3:
		if !cfg.ObjectStorage.Enabled || ref.S3 == nil || (ref.S3.GetObjectKey() == "" && ref.S3.GetSignedUrl() == "") {
			return bodyRefUnavailableError(BodyTransportDirectionRequest, BodyTransportS3BodyRef)
		}
	case *strawpb.BodyRefFrame_DirectStream:
		if !cfg.DirectStream.Enabled || ref.DirectStream == nil || ref.DirectStream.GetEndpoint() == "" || ref.DirectStream.GetStreamId() == "" {
			return bodyRefUnavailableError(BodyTransportDirectionRequest, BodyTransportDirectStreamRef)
		}
	default:
		return &PipelineError{Code: ProtocolError}
	}

	return nil
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
