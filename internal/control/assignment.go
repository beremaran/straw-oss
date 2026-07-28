package control

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type routeExecution struct {
	policy        *DestinationPolicyResult
	upstreamProxy *strawpb.UpstreamProxyInstruction
}

func (d *DefaultRequestDispatcher) resolveRouteExecution(in DispatchInput, snapshot config.Snapshot, route RouteOutcome, rawTunnel bool) (routeExecution, *PipelineError) {
	requestedFingerprint := in.Request.Fingerprint
	if rawTunnel {
		requestedFingerprint = ""
	}

	policy, verr := ResolveDestinationPolicy(DestinationPolicyRequest{
		Snapshot:                    snapshot,
		TargetURL:                   in.Request.URL,
		RequestedFingerprintProfile: requestedFingerprint,
		MaxInjectedHeaderBytes:      d.opts.MaxFrameDataBytes,
		UpstreamProxyEnabled:        route.UpstreamProxyID != "",
		UpstreamProxyTrusted:        route.TrustedRemoteResolution,
	})
	if verr != nil {
		return routeExecution{}, validationPipelineError(verr)
	}

	execution := routeExecution{policy: policy}
	if rawTunnel {
		policy.InjectionOperations = nil
		policy.FingerprintProfile = ""
	}

	if route.UpstreamProxyID != "" {
		execution.upstreamProxy = &strawpb.UpstreamProxyInstruction{
			UpstreamProxyId:   route.UpstreamProxyID,
			ProviderSessionId: deriveProviderSessionID(in.Identity.DeploymentID, route, in.Request.Routing.Country, in.Request.Routing.Region, in.Request.Routing.IPType, in.Request.Routing.StickySessionID),
			Country:           in.Request.Routing.Country,
			Region:            in.Request.Routing.Region,
			IpType:            in.Request.Routing.IPType,
		}
	}

	return execution, nil
}

func (d *DefaultRequestDispatcher) executeAttemptOrFallback(ctx context.Context, in DispatchInput, route RouteOutcome, snapshot config.Snapshot, deadline time.Time) (dispatchResult, int64, *PipelineError, RouteOutcome) {
	execution, perr := d.resolveRouteExecution(in, snapshot, route, false)
	if perr != nil {
		return dispatchResult{}, 0, perr, route
	}

	result, assignmentMs, perr := d.executeAttempt(ctx, in, route, execution, snapshot.ConfigVersion, deadline)
	if perr == nil || !canFallbackBeforeRequestStart(perr.Code) {
		return result, assignmentMs, perr, route
	}

	routeFailure(d.opts.Workers, route.WorkerID)

	fallback := d.routeWithWorkers(in, snapshot, excludeWorkers{base: d.opts.Workers, workerID: route.WorkerID})
	if !fallback.OK {
		return result, assignmentMs, perr, route
	}

	fallbackExecution, fallbackPlanErr := d.resolveRouteExecution(in, snapshot, fallback, false)
	if fallbackPlanErr != nil {
		return result, assignmentMs, fallbackPlanErr, fallback
	}

	fallbackResult, fallbackAssignmentMs, fallbackErr := d.executeAttempt(ctx, in, fallback, fallbackExecution, snapshot.ConfigVersion, deadline)

	return fallbackResult, assignmentMs + fallbackAssignmentMs, fallbackErr, fallback
}

type excludeWorkers struct {
	base     CandidateSource
	workerID string
}

func (e excludeWorkers) CandidatesForPool(deploymentID, poolID string) []PoolCandidate {
	candidates := e.base.CandidatesForPool(deploymentID, poolID)

	out := candidates[:0]
	for _, candidate := range candidates {
		if candidate.WorkerID != e.workerID {
			out = append(out, candidate)
		}
	}

	return out
}

// poolPoliciesFromSnapshot converts the deployment snapshot's executor pools
// into the flat list StaticPoolPolicyProvider expects.
func poolPoliciesFromSnapshot(deploymentID string, pools []config.ExecutorPool) []PoolPolicy {
	out := make([]PoolPolicy, 0, len(pools))
	for _, p := range pools {
		policy := PoolPolicy{
			DeploymentID:         deploymentID,
			PoolID:               p.ID,
			Enabled:              p.Enabled,
			ExecutorType:         p.ExecutorType,
			Tags:                 append([]string(nil), p.Tags...),
			AllowDegradedWorkers: p.AllowDegradedWorkers,
			AllowedCountries:     p.AllowedCountries,
			AllowedRegions:       p.AllowedRegions,
			AllowedIPTypes:       p.AllowedIPTypes,
		}
		if p.UpstreamProxy != nil {
			policy.UpstreamProxyID = p.UpstreamProxy.ID
			policy.TrustedRemoteResolution = p.UpstreamProxy.TrustedRemoteResolution
		}

		out = append(out, policy)
	}

	return out
}

// executeAttempt runs one assignment-and-stream attempt, recording the
// straw_assignment_duration_seconds histogram (docs/public/architecture.md) over the
// full attempt regardless of outcome.
func (d *DefaultRequestDispatcher) executeAttempt(ctx context.Context, in DispatchInput, route RouteOutcome, execution routeExecution, configVersion uint64, deadline time.Time) (dispatchResult, int64, *PipelineError) {
	result, assignmentMs, perr := d.executeAttemptUnmeasured(ctx, in, route, execution, configVersion, deadline)
	d.opts.Metrics.ObserveAssignment(time.Duration(assignmentMs) * time.Millisecond)

	return result, assignmentMs, perr
}

func (d *DefaultRequestDispatcher) executeAttemptUnmeasured(ctx context.Context, in DispatchInput, route RouteOutcome, execution routeExecution, configVersion uint64, deadline time.Time) (dispatchResult, int64, *PipelineError) {
	assignmentStarted := d.opts.Now()
	in.ProtocolMinor = route.ProtocolMinor

	if d.opts.NATS == nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}
	}

	c2eSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionControlToExecutor)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}
	}

	e2cSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionExecutorToControl)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}
	}

	frames := make(chan *strawpb.StreamFrame, defaultRequestFrameBuffer)

	sub, err := d.opts.NATS.Subscribe(e2cSubject, func(msg *nats.Msg) {
		frames <- decodeDispatchFrame(msg.Data, route.ProtocolMinor)
	})
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}
	}

	defer func() { _ = sub.Unsubscribe() }()

	err = d.opts.NATS.Flush()
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}
	}

	assign := d.assignRequest(in, route, configVersion, deadline)

	ack, perr := d.requestAssign(route.AssignSubject, in, assign, route.ProtocolMinor, deadline)
	if perr != nil {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), perr
	}

	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), assignRejectError(ack.GetCode())
	}

	assignmentMs := millisSince(assignmentStarted, d.opts.Now())

	nextSeq, err := d.sendRequestStart(ctx, c2eSubject, in, route, execution, configVersion, deadline)
	if err != nil {
		return dispatchResult{}, assignmentMs, requestStreamPipelineError(err)
	}

	upload := d.startStreamingRequestBody(ctx, c2eSubject, in, deadline, nextSeq)
	result, perr := d.readResponse(ctx, frames, route, deadline, c2eSubject, in, nextSeq, upload)

	return result, assignmentMs, perr
}

func (d *DefaultRequestDispatcher) executeRawAttempt(ctx context.Context, in DispatchInput, route RouteOutcome, execution routeExecution, configVersion uint64, deadline time.Time, w http.ResponseWriter) (dispatchResult, int64, *PipelineError, bool) {
	result, assignmentMs, perr, wroteHeader := d.executeRawAttemptUnmeasured(ctx, in, route, execution, configVersion, deadline, w)
	d.opts.Metrics.ObserveAssignment(time.Duration(assignmentMs) * time.Millisecond)

	return result, assignmentMs, perr, wroteHeader
}

// executeRawAttemptOrFallback gives absolute-form proxy requests the same
// assignment-time worker exclusion behavior as REST. A retry is possible only
// before the first upstream response header is written to the client.
func (d *DefaultRequestDispatcher) executeRawAttemptOrFallback(ctx context.Context, in DispatchInput, route RouteOutcome, snapshot config.Snapshot, deadline time.Time, w http.ResponseWriter) (dispatchResult, int64, *PipelineError, bool, RouteOutcome) {
	execution, perr := d.resolveRouteExecution(in, snapshot, route, false)
	if perr != nil {
		return dispatchResult{}, 0, perr, false, route
	}

	result, assignmentMs, perr, wroteHeader := d.executeRawAttempt(ctx, in, route, execution, snapshot.ConfigVersion, deadline, w)
	if perr == nil || wroteHeader || !canFallbackBeforeRequestStart(perr.Code) {
		return result, assignmentMs, perr, wroteHeader, route
	}

	routeFailure(d.opts.Workers, route.WorkerID)

	fallback := d.routeWithWorkers(in, snapshot, excludeWorkers{base: d.opts.Workers, workerID: route.WorkerID})
	if !fallback.OK {
		return result, assignmentMs, perr, wroteHeader, route
	}

	fallbackExecution, fallbackPlanErr := d.resolveRouteExecution(in, snapshot, fallback, false)
	if fallbackPlanErr != nil {
		return result, assignmentMs, fallbackPlanErr, wroteHeader, fallback
	}

	fallbackResult, fallbackAssignmentMs, fallbackErr, fallbackWroteHeader := d.executeRawAttempt(ctx, in, fallback, fallbackExecution, snapshot.ConfigVersion, deadline, w)

	return fallbackResult, assignmentMs + fallbackAssignmentMs, fallbackErr, fallbackWroteHeader, fallback
}

func (d *DefaultRequestDispatcher) executeRawAttemptUnmeasured(ctx context.Context, in DispatchInput, route RouteOutcome, execution routeExecution, configVersion uint64, deadline time.Time, w http.ResponseWriter) (dispatchResult, int64, *PipelineError, bool) {
	assignmentStarted := d.opts.Now()
	in.ProtocolMinor = route.ProtocolMinor

	if d.opts.NATS == nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}, false
	}

	c2eSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionControlToExecutor)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}, false
	}

	e2cSubject, err := natsx.StreamSubject(in.RequestID, route.WorkerID, route.SessionID, natsx.DirectionExecutorToControl)
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: ControlInternalError}, false
	}

	frames := make(chan *strawpb.StreamFrame, defaultRequestFrameBuffer)

	sub, err := d.opts.NATS.Subscribe(e2cSubject, func(msg *nats.Msg) {
		frames <- decodeDispatchFrame(msg.Data, route.ProtocolMinor)
	})
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}, false
	}

	defer func() { _ = sub.Unsubscribe() }()

	err = d.opts.NATS.Flush()
	if err != nil {
		return dispatchResult{}, 0, &PipelineError{Code: TransportUnavailable}, false
	}

	assign := d.assignRequest(in, route, configVersion, deadline)

	ack, perr := d.requestAssign(route.AssignSubject, in, assign, route.ProtocolMinor, deadline)
	if perr != nil {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), perr, false
	}

	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), assignRejectError(ack.GetCode()), false
	}

	assignmentMs := millisSince(assignmentStarted, d.opts.Now())

	nextSeq, err := d.sendRequestStart(ctx, c2eSubject, in, route, execution, configVersion, deadline)
	if err != nil {
		return dispatchResult{}, assignmentMs, requestStreamPipelineError(err), false
	}

	upload := d.startStreamingRequestBody(ctx, c2eSubject, in, deadline, nextSeq)
	result, perr, wroteHeader := d.streamRawResponse(ctx, frames, route, deadline, c2eSubject, in, nextSeq, upload, w)

	return result, assignmentMs, perr, wroteHeader
}

func (d *DefaultRequestDispatcher) requestAssign(subject string, in DispatchInput, assign *strawpb.AssignRequest, protocolMinor uint32, deadline time.Time) (*strawpb.AssignAck, *PipelineError) {
	env := &strawpb.Envelope{
		RequestId:      in.RequestID,
		DeploymentId:   in.Identity.DeploymentID,
		DeadlineUnixMs: deadline.UnixMilli(),
		ProtocolMajor:  ProtocolMajor,
		ProtocolMinor:  protocolMinor,
		Attempt:        defaultRequestAttempt,
		Payload:        &strawpb.Envelope_AssignRequest{AssignRequest: assign},
	}

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		return nil, &PipelineError{Code: ControlInternalError}
	}

	timeout := time.Until(ackDeadline(d.opts.Now(), d.opts.AssignmentAckTimeout, deadline))
	if timeout <= 0 {
		return nil, &PipelineError{Code: AssignmentTimeout, TimeoutType: timeoutTypeTotalDeadline}
	}

	natsStarted := d.opts.Now()
	msg, err := d.opts.NATS.Request(subject, raw, timeout)
	d.opts.Metrics.ObserveNATSRequest(d.opts.Now().Sub(natsStarted))

	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			timeoutType := timeoutTypeAssignment
			if !d.opts.Now().Before(deadline) {
				timeoutType = timeoutTypeTotalDeadline
			}

			d.opts.Metrics.IncNATSError(errorCodeLabel(AssignmentTimeout))

			return nil, &PipelineError{Code: AssignmentTimeout, TimeoutType: timeoutType}
		}

		d.opts.Metrics.IncNATSError(errorCodeLabel(TransportUnavailable))

		return nil, &PipelineError{Code: TransportUnavailable}
	}

	reply, err := natsx.UnmarshalEnvelope(msg.Data)
	if err != nil || reply.GetAssignAck() == nil || reply.GetProtocolMajor() != ProtocolMajor || !dispatchEnvelopeMinorCompatible(reply.GetProtocolMinor(), protocolMinor) {
		d.opts.Metrics.IncNATSError(errorCodeLabel(ProtocolError))

		return nil, &PipelineError{Code: ProtocolError}
	}

	return reply.GetAssignAck(), nil
}

// sendRequestStart publishes RequestStart and either a receipt reference or
// inline body DataFrames on the c2e
// subject and returns the next c2e stream_seq (i.e. the seq a subsequent
// CancelFrame should use).
func (d *DefaultRequestDispatcher) sendRequestStart(_ context.Context, subject string, in DispatchInput, route RouteOutcome, execution routeExecution, configVersion uint64, deadline time.Time) (uint64, error) {
	start := &strawpb.RequestStart{
		Mode:                   requestMode(in.Request),
		Method:                 in.Request.Method,
		Url:                    in.Request.URL.String(),
		Headers:                headersToProto(in.Request.Headers),
		SelectedRouteId:        route.RuleID,
		SelectedPoolId:         route.PoolID,
		DeadlineUnixMs:         deadline.UnixMilli(),
		Replayable:             in.Request.Replayable,
		PayloadCaptureDecision: in.Request.CaptureDecision,
		FingerprintInstruction: wireFingerprint(execution.policy.FingerprintProfile),
		InjectionOperations:    execution.policy.InjectionOperations,
		RedirectPolicy:         strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		DestinationPolicy:      execution.policy.Policy,
		PolicyVersion:          strconv.FormatUint(configVersion, 10),
		UpstreamProxy:          execution.upstreamProxy,
	}

	seq := uint64(1)

	err := d.publishFrame(subject, in, deadline, &strawpb.StreamFrame{
		StreamSeq: seq,
		Attempt:   defaultRequestAttempt,
		Payload:   &strawpb.StreamFrame_RequestStart{RequestStart: start},
	})
	if err != nil {
		return 0, err
	}

	if in.Request.BodyRef != nil {
		seq++

		err = d.publishFrame(subject, in, deadline, &strawpb.StreamFrame{StreamSeq: seq, Attempt: defaultRequestAttempt, Payload: &strawpb.StreamFrame_BodyRef{BodyRef: in.Request.BodyRef}})
		if err != nil {
			return 0, err
		}
	} else if in.Request.BodyReader == nil {
		seq, err = d.sendRequestBody(subject, in, deadline, seq)
		if err != nil {
			return 0, err
		}
	}

	err = d.opts.NATS.Flush()
	if err != nil {
		return 0, fmt.Errorf("flush request stream: %w", err)
	}

	return seq + 1, nil
}
