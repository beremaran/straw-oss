package control

import (
	"math"
	"strconv"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func routeError(code string) *PipelineError {
	switch code {
	case RouteErrNoMatch:
		return &PipelineError{Code: RouteNoMatch}
	case RouteErrStickyUnavailable:
		return &PipelineError{Code: StickySessionUnavailable}
	case RouteErrCapacityExhausted:
		return &PipelineError{Code: ExecutorCapacityExhausted}
	case RouteErrUnsupportedFingerprint:
		return &PipelineError{Code: UnsupportedFingerprint}
	default:
		return &PipelineError{Code: RouteUnavailable}
	}
}

func validationPipelineError(verr *ValidationError) *PipelineError {
	code := ErrorCodeFromName(verr.Code)
	if code == 0 {
		code = InvalidRequest
	}

	return &PipelineError{Code: code, Message: verr.Message, Details: verr.Details}
}

func assignRejectError(code strawpb.AssignAckCode) *PipelineError {
	if code == strawpb.AssignAckCode_ASSIGN_ACK_REJECTED_CAPACITY {
		return &PipelineError{Code: ExecutorCapacityExhausted}
	}

	return &PipelineError{Code: RouteUnavailable}
}

func canFallbackBeforeRequestStart(code ErrorCode) bool {
	return code == AssignmentTimeout || code == ExecutorCapacityExhausted || code == RouteUnavailable
}

func errorFramePipelineError(code strawpb.ErrorCode, frame *strawpb.ErrorFrame) *PipelineError {
	perr := &PipelineError{
		Code:         ErrorCode(code),
		Message:      frame.GetMessage(),
		RetryAfterMs: retryAfterMs(frame.GetRetryAfterMs()),
		Details:      frame.GetDetails(),
	}

	if frame.TimeoutType != nil {
		perr.TimeoutType = timeoutTypeName(frame.GetTimeoutType())
	}

	return perr
}

func retryAfterMs(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}

	return int64(v)
}

func frameChunkSize(bodyLen int, limit uint64) int {
	if limit == 0 {
		return bodyLen
	}

	n, err := strconv.Atoi(strconv.FormatUint(limit, 10))
	if err != nil || n > bodyLen {
		return bodyLen
	}

	return n
}
