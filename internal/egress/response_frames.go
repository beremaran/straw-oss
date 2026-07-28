package egress

import (
	"strconv"
	"time"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type frameBuilder struct {
	attempt uint32
	seq     uint64
}

func newFrameBuilder(attempt uint32) *frameBuilder {
	return &frameBuilder{attempt: attempt}
}

func (b *frameBuilder) outboundStart(host string, port uint32, executedProfile, upstreamProxyID string) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload: &strawpb.StreamFrame_OutboundStart{OutboundStart: &strawpb.OutboundStartFrame{
			TargetHost:                 host,
			TargetPort:                 port,
			Attempt:                    b.attempt,
			ExecutedFingerprintProfile: executedProfile,
			UpstreamProxyId:            upstreamProxyID,
			WorkerTimestampMs:          time.Now().UnixMilli(),
		}},
	}
}

// emitOrBatch delivers frame through send when one is provided, otherwise
// returns it as the start of the batched frame slice.
func emitOrBatch(frame *strawpb.StreamFrame, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	if send != nil {
		send(frame)

		return nil
	}

	return []*strawpb.StreamFrame{frame}
}

func emitOrAppend(frames []*strawpb.StreamFrame, frame *strawpb.StreamFrame, send func(*strawpb.StreamFrame)) []*strawpb.StreamFrame {
	if send != nil {
		send(frame)

		return frames
	}

	return append(frames, frame)
}

func responseStatus(status int) (uint32, *executionError) {
	if status < 0 || status > 999 {
		return 0, executorFailure(strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR, executorInternalFact)
	}

	return uint32(status), nil
}

func (b *frameBuilder) responseStart(status uint32, headers []*strawpb.Header) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload: &strawpb.StreamFrame_ResponseStart{ResponseStart: &strawpb.ResponseStart{
			Status:  status,
			Headers: headers,
		}},
	}
}

func (b *frameBuilder) data(offset uint64, data []byte) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: offset, Data: data}},
	}
}

func (b *frameBuilder) trailers(headers []*strawpb.Header) *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_Trailers{Trailers: &strawpb.TrailersFrame{Headers: headers}},
	}
}

func (b *frameBuilder) end() *strawpb.StreamFrame {
	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_End{End: &strawpb.EndFrame{Success: true}},
	}
}

func (b *frameBuilder) error(failure *executionError) *strawpb.StreamFrame {
	errFrame := &strawpb.ErrorFrame{
		Code:    failure.code,
		Details: map[string]string{errorFactDetailKey: failure.fact},
	}
	if failure.timeoutType != strawpb.TimeoutType_TIMEOUT_TYPE_UNSPECIFIED {
		errFrame.TimeoutType = &failure.timeoutType
	}

	if failure.upstreamStatus != nil {
		status := *failure.upstreamStatus
		errFrame.UpstreamStatus = &status
	}

	b.seq++

	return &strawpb.StreamFrame{
		StreamSeq: b.seq,
		Attempt:   b.attempt,
		Payload:   &strawpb.StreamFrame_Error{Error: errFrame},
	}
}

func uint64FromInt(v int) uint64 {
	if v <= 0 {
		return 0
	}

	out, err := strconv.ParseUint(strconv.Itoa(v), 10, 64)
	if err != nil {
		return 0
	}

	return out
}
