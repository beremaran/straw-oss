package control

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

func (d *DefaultRequestDispatcher) assignRequest(in DispatchInput, route RouteOutcome, configVersion uint64, deadline time.Time) *strawpb.AssignRequest {
	return &strawpb.AssignRequest{
		Mode:                       requestMode(in.Request),
		DeadlineUnixMs:             deadline.UnixMilli(),
		ExpectedUploadBytes:        expectedUploadBytes(in.Request),
		SelectedRouteId:            route.RuleID,
		SelectedPoolId:             route.PoolID,
		SelectedExecutorId:         route.WorkerID,
		Replayable:                 in.Request.Replayable,
		Attempt:                    defaultRequestAttempt,
		PolicyVersion:              strconv.FormatUint(configVersion, 10),
		InitialUploadCreditBytes:   d.opts.InitialUploadCreditBytes,
		InitialDownloadCreditBytes: d.opts.InitialDownloadCreditBytes,
		MaxInflightUploadBytes:     d.opts.MaxInflightUploadBytes,
		MaxInflightDownloadBytes:   d.opts.MaxInflightDownloadBytes,
	}
}

func requestMode(req *ValidatedRequest) strawpb.RequestMode {
	if req != nil && req.IngressType == IngressTypeConnect {
		return strawpb.RequestMode_REQUEST_MODE_RAW_TUNNEL
	}

	return strawpb.RequestMode_REQUEST_MODE_DECODED_HTTP
}

func expectedUploadBytes(req *ValidatedRequest) int64 {
	if req != nil && req.IngressType == IngressTypeConnect {
		return 0
	}

	if req != nil && req.BodyReader != nil {
		return req.BodySizeBytes
	}

	if req != nil && req.BodyRef != nil {
		return req.BodySizeBytes
	}

	return int64(len(req.BodyData))
}

func (d *DefaultRequestDispatcher) deadline(req *ValidatedRequest, snapshot config.Snapshot) time.Time {
	timeoutMs := req.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = effectiveDefaultTimeout(d.opts.MaxTimeoutMs, snapshot.DefaultTimeoutMs)
	}

	if timeoutMs == 0 {
		timeoutMs = defaultRequestTimeoutFallback
	}

	return d.opts.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
}

func effectiveDefaultTimeout(staticMax, deploymentDefault uint64) uint64 {
	if deploymentDefault == 0 {
		deploymentDefault = defaultRequestTimeoutMS
	}

	if staticMax != 0 && deploymentDefault > staticMax {
		return staticMax
	}

	return deploymentDefault
}

func timeoutTypeName(t strawpb.TimeoutType) string {
	name := strings.TrimPrefix(t.String(), "TIMEOUT_TYPE_")
	name = strings.ToLower(name)

	return name
}

func wireFingerprint(profile string) string {
	if profile == baselineFingerprintProfileName {
		return ""
	}

	return profile
}

func headersToProto(headers []HeaderPair) []*strawpb.Header {
	out := make([]*strawpb.Header, 0, len(headers))
	for _, h := range headers {
		value, err := base64.StdEncoding.DecodeString(h.Value)
		if err != nil {
			continue
		}

		out = append(out, &strawpb.Header{Name: h.Name, Value: value})
	}

	return out
}

func headersFromProto(headers []*strawpb.Header) []HeaderPair {
	out := make([]HeaderPair, 0, len(headers))
	for _, h := range headers {
		out = append(out, HeaderPair{Name: h.GetName(), Value: base64.StdEncoding.EncodeToString(h.GetValue())})
	}

	return out
}

func decodeDispatchFrame(raw []byte, protocolMinor uint32) *strawpb.StreamFrame {
	env, err := natsx.UnmarshalEnvelope(raw)
	if err != nil || env.GetProtocolMajor() != ProtocolMajor || !dispatchEnvelopeMinorCompatible(env.GetProtocolMinor(), protocolMinor) {
		return nil
	}

	return env.GetStreamFrame()
}

func dispatchEnvelopeMinorCompatible(actual, negotiated uint32) bool {
	// Published minor-1 workers omitted protocol_minor from assignment replies
	// and response envelopes. Keep that direct-only compatibility exception;
	// minor 2 and future negotiated versions remain exact.
	return actual == negotiated || actual == 0 && negotiated == 1
}

func routeFailure(candidates CandidateSource, workerID string) {
	recorder, ok := candidates.(interface{ RecordFailure(workerID string) })
	if ok {
		recorder.RecordFailure(workerID)
	}
}

func millisSince(start, end time.Time) int64 {
	return end.Sub(start).Milliseconds()
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
