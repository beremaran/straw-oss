package control

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const (
	defaultAssignmentAckTimeout   = 2 * time.Second
	defaultInitialStreamCredit    = 8 << 20
	defaultMaxInflightStreamBytes = 16 << 20
	defaultFrameIdleTimeout       = 15 * time.Second
	defaultRequestFrameBuffer     = 32
	defaultRequestAttempt         = uint32(1)
	defaultPayloadCaptureDecision = "none"
	defaultMaxFrameDataBytes      = 1048576
	defaultRequestTimeoutFallback = 120000
	responseFrameCheckInterval    = 100 * time.Millisecond
	timeoutTypeAssignment         = "assignment_timeout"
	timeoutTypeTotalDeadline      = "total_deadline_timeout"
	timeoutTypeIdle               = "idle_timeout"
)

// RequestDispatcher executes a validated decoded HTTP request.
type RequestDispatcher interface {
	Dispatch(ctx context.Context, in DispatchInput) (SuccessResponse, *PipelineError)
}

// RawResponseDispatcher streams a decoded request's upstream response directly
// to an HTTP client without the REST JSON envelope.
type RawResponseDispatcher interface {
	DispatchRaw(ctx context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool)
}

// TunnelDispatcher carries raw CONNECT tunnel bytes over the request stream.
type TunnelDispatcher interface {
	DispatchTunnel(ctx context.Context, in DispatchInput, rw io.ReadWriter) (SuccessResponse, *PipelineError)
}

// DispatchInput is the authenticated, validated request context.
type DispatchInput struct {
	RequestID string
	Identity  Identity
	Request   *ValidatedRequest
}

// PipelineError is a canonical dispatch failure.
type PipelineError struct {
	Code         ErrorCode
	Message      string
	Details      map[string]string
	RetryAfterMs int64
	TimeoutType  string
	// RoutingMs/AssignmentMs/EgressMs/TotalMs carry whatever partial phase
	// timing the dispatcher measured before the failure, so a failed
	// request_events row (docs/tasks/p0/32) still reports the real elapsed
	// time instead of zeros.
	RoutingMs    int64
	AssignmentMs int64
	EgressMs     int64
	TotalMs      int64
}

// RequestDispatcherOptions wires the Control request pipeline.
type RequestDispatcherOptions struct {
	ConfigCache                *ConfigCache
	Workers                    CandidateSource
	Sticky                     StickyBackend
	NATS                       *natsx.Connection
	RateLimitAdmission         *RateLimitAdmission
	QuotaAdmission             *QuotaAdmission
	MaxInlineResponseBodyBytes uint64
	MaxFrameDataBytes          uint64
	MaxTimeoutMs               uint64
	AssignmentAckTimeout       time.Duration
	InitialUploadCreditBytes   uint64
	InitialDownloadCreditBytes uint64
	MaxInflightUploadBytes     uint64
	MaxInflightDownloadBytes   uint64
	FrameIdleTimeout           time.Duration
	Now                        func() time.Time
	// InFlight registers each dispatched request's cancel function so an
	// admin cancel (docs/tasks/p0/27) can reach it. Optional: nil disables
	// admin cancellation without affecting client-disconnect/deadline
	// cancellation, which is driven directly by ctx.
	InFlight *InFlightRegistry
	// Metrics records the P0 Prometheus series (docs/planning/23). Optional:
	// nil disables instrumentation.
	Metrics *Metrics
}

// DefaultRequestDispatcher is the P0 Control dispatch pipeline.
type DefaultRequestDispatcher struct {
	opts RequestDispatcherOptions
}

// NewDefaultRequestDispatcher builds the real Control request dispatcher.
func NewDefaultRequestDispatcher(opts RequestDispatcherOptions) *DefaultRequestDispatcher {
	if opts.AssignmentAckTimeout == 0 {
		opts.AssignmentAckTimeout = defaultAssignmentAckTimeout
	}

	if opts.InitialUploadCreditBytes == 0 {
		opts.InitialUploadCreditBytes = defaultInitialStreamCredit
	}

	if opts.InitialDownloadCreditBytes == 0 {
		opts.InitialDownloadCreditBytes = defaultInitialStreamCredit
	}

	if opts.MaxInflightUploadBytes == 0 {
		opts.MaxInflightUploadBytes = defaultMaxInflightStreamBytes
	}

	if opts.MaxInflightDownloadBytes == 0 {
		opts.MaxInflightDownloadBytes = defaultMaxInflightStreamBytes
	}

	if opts.MaxFrameDataBytes == 0 {
		opts.MaxFrameDataBytes = defaultMaxFrameDataBytes
	}

	if opts.FrameIdleTimeout == 0 {
		opts.FrameIdleTimeout = defaultFrameIdleTimeout
	}

	if opts.Sticky == nil {
		opts.Sticky = NewStickyStore(opts.Now)
	}

	if opts.Now == nil {
		opts.Now = time.Now
	}

	return &DefaultRequestDispatcher{opts: opts}
}

// Dispatch runs admission, routing, assignment, streaming, and response
// buffering for one REST request, recording the straw_active_requests,
// straw_requests_total, and straw_request_duration_seconds metrics
// (docs/planning/23) around the attempt.
func (d *DefaultRequestDispatcher) Dispatch(ctx context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	started := d.opts.Now()

	d.opts.Metrics.IncActiveRequests()
	defer d.opts.Metrics.DecActiveRequests()

	resp, perr := d.dispatch(ctx, in, started)

	var code ErrorCode
	if perr != nil {
		code = perr.Code
	}

	d.opts.Metrics.ObserveRequest(in.Identity.TenantID, errorCodeLabel(code), d.opts.Now().Sub(started))

	return resp, perr
}

// DispatchRaw runs the same Control pipeline as Dispatch, but writes upstream
// response headers and DataFrames to w as they arrive. It returns whether any
// upstream response header has been written, so callers do not try to render a
// second HTTP error after a partial raw response.
func (d *DefaultRequestDispatcher) DispatchRaw(ctx context.Context, in DispatchInput, w http.ResponseWriter) (SuccessResponse, *PipelineError, bool) {
	started := d.opts.Now()

	d.opts.Metrics.IncActiveRequests()
	defer d.opts.Metrics.DecActiveRequests()

	resp, perr, wroteHeader := d.dispatchRaw(ctx, in, w, started)

	var code ErrorCode
	if perr != nil {
		code = perr.Code
	}

	d.opts.Metrics.ObserveRequest(in.Identity.TenantID, errorCodeLabel(code), d.opts.Now().Sub(started))

	return resp, perr, wroteHeader
}

func (d *DefaultRequestDispatcher) dispatch(ctx context.Context, in DispatchInput, started time.Time) (SuccessResponse, *PipelineError) {
	if in.Request == nil || d.opts.ConfigCache == nil || d.opts.Workers == nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	d.opts.InFlight.Register(ctx, in.RequestID, in.Identity.TenantID, cancel)
	defer d.opts.InFlight.Deregister(ctx, in.RequestID)

	snapshot, err := d.opts.ConfigCache.Snapshot(ctx, in.Identity.TenantID)
	if err != nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started)
	}

	if perr := d.admit(ctx, in, snapshot); perr != nil {
		return SuccessResponse{}, d.withTiming(perr, 0, 0, started)
	}

	routeStart := d.opts.Now()
	route := d.route(in, snapshot)
	routeEnd := d.opts.Now()
	routingMs := millisSince(routeStart, routeEnd)
	d.opts.Metrics.ObserveRouting(routeEnd.Sub(routeStart))

	if !route.OK {
		return SuccessResponse{}, d.withTiming(routeError(route.ErrorCode), routingMs, 0, started)
	}

	policy, verr := ResolveDestinationPolicy(DestinationPolicyRequest{
		Snapshot:                    snapshot,
		TargetURL:                   in.Request.URL,
		RequestedFingerprintProfile: in.Request.Fingerprint,
		MaxInjectedHeaderBytes:      d.opts.MaxFrameDataBytes,
		UpstreamProxyEnabled:        false,
		UpstreamProxyTrusted:        false,
	})
	if verr != nil {
		return SuccessResponse{}, d.withTiming(validationPipelineError(verr), routingMs, 0, started)
	}

	deadline := d.deadline(in.Request, snapshot)

	result, assignmentMs, perr := d.executeAttemptOrFallback(ctx, in, route, snapshot, policy, deadline)

	if perr != nil {
		perr = d.withTiming(perr, routingMs, assignmentMs, started)
		perr.EgressMs = result.egressMs

		return SuccessResponse{}, perr
	}

	if d.opts.QuotaAdmission != nil {
		_ = d.opts.QuotaAdmission.RecordSuccess(ctx, quotaFromSnapshot(in.Identity.TenantID, snapshot.Quota))
		_ = d.opts.QuotaAdmission.AddBandwidth(ctx, in.Identity.TenantID, int64(len(in.Request.BodyData)+len(result.body)))
	}

	return successFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now())), nil
}

func (d *DefaultRequestDispatcher) dispatchRaw(ctx context.Context, in DispatchInput, w http.ResponseWriter, started time.Time) (SuccessResponse, *PipelineError, bool) {
	if in.Request == nil || d.opts.ConfigCache == nil || d.opts.Workers == nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started), false
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	d.opts.InFlight.Register(ctx, in.RequestID, in.Identity.TenantID, cancel)
	defer d.opts.InFlight.Deregister(ctx, in.RequestID)

	snapshot, err := d.opts.ConfigCache.Snapshot(ctx, in.Identity.TenantID)
	if err != nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started), false
	}

	if perr := d.admit(ctx, in, snapshot); perr != nil {
		return SuccessResponse{}, d.withTiming(perr, 0, 0, started), false
	}

	routeStart := d.opts.Now()
	route := d.route(in, snapshot)
	routeEnd := d.opts.Now()
	routingMs := millisSince(routeStart, routeEnd)
	d.opts.Metrics.ObserveRouting(routeEnd.Sub(routeStart))

	if !route.OK {
		return SuccessResponse{}, d.withTiming(routeError(route.ErrorCode), routingMs, 0, started), false
	}

	policy, verr := ResolveDestinationPolicy(DestinationPolicyRequest{
		Snapshot:                    snapshot,
		TargetURL:                   in.Request.URL,
		RequestedFingerprintProfile: in.Request.Fingerprint,
		MaxInjectedHeaderBytes:      d.opts.MaxFrameDataBytes,
		UpstreamProxyEnabled:        false,
		UpstreamProxyTrusted:        false,
	})
	if verr != nil {
		return SuccessResponse{}, d.withTiming(validationPipelineError(verr), routingMs, 0, started), false
	}

	deadline := d.deadline(in.Request, snapshot)

	result, assignmentMs, perr, wroteHeader := d.executeRawAttempt(ctx, in, route, policy, snapshot.ConfigVersion, deadline, w)
	if perr != nil {
		perr = d.withTiming(perr, routingMs, assignmentMs, started)
		perr.EgressMs = result.egressMs

		return rawSuccessFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now())), perr, wroteHeader
	}

	if d.opts.QuotaAdmission != nil {
		_ = d.opts.QuotaAdmission.RecordSuccess(ctx, quotaFromSnapshot(in.Identity.TenantID, snapshot.Quota))
		_ = d.opts.QuotaAdmission.AddBandwidth(ctx, in.Identity.TenantID, int64(len(in.Request.BodyData))+int64FromUint64(result.size))
	}

	return rawSuccessFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now())), nil, wroteHeader
}

// withTiming annotates perr with whatever partial phase timing the
// dispatcher measured before the failure (docs/tasks/p0/32), so a failed
// request_events row reports real elapsed time instead of zeros.
func (d *DefaultRequestDispatcher) withTiming(perr *PipelineError, routingMs, assignmentMs int64, started time.Time) *PipelineError {
	perr.RoutingMs = routingMs
	perr.AssignmentMs = assignmentMs
	perr.TotalMs = millisSince(started, d.opts.Now())

	return perr
}

func successFromDispatch(requestID string, result dispatchResult, routingMs, assignmentMs, totalMs int64) SuccessResponse {
	return SuccessResponse{
		RequestID: requestID,
		Status:    int(result.status),
		Headers:   headersFromProto(result.headers),
		Body: ResponseBody{
			Mode:       "inline_base64",
			DataBase64: base64.StdEncoding.EncodeToString(result.body),
			Truncated:  false,
		},
		Timing: RequestTiming{
			RoutingMs:    routingMs,
			AssignmentMs: assignmentMs,
			EgressMs:     result.egressMs,
			TotalMs:      totalMs,
		},
		ResponseSizeBytes: uint64(len(result.body)),
	}
}

func rawSuccessFromDispatch(requestID string, result dispatchResult, routingMs, assignmentMs, totalMs int64) SuccessResponse {
	return SuccessResponse{
		RequestID: requestID,
		Status:    int(result.status),
		Headers:   headersFromProto(result.headers),
		Timing: RequestTiming{
			RoutingMs:    routingMs,
			AssignmentMs: assignmentMs,
			EgressMs:     result.egressMs,
			TotalMs:      totalMs,
		},
		ResponseSizeBytes: result.size,
	}
}

func (d *DefaultRequestDispatcher) admit(ctx context.Context, in DispatchInput, snapshot config.TenantSnapshot) *PipelineError {
	if d.opts.RateLimitAdmission != nil {
		decision := d.opts.RateLimitAdmission.Check(ctx, rateLimitFromSnapshot(in.Identity.TenantID, snapshot.RateLimits), RateLimitRequest{
			TenantID:   in.Identity.TenantID,
			APIKeyID:   in.Identity.APIKeyID,
			TargetHost: strings.ToLower(in.Request.URL.Hostname()),
			IPType:     in.Request.Routing.IPType,
		})
		if !decision.Allowed {
			return &PipelineError{Code: RateLimitExceeded, RetryAfterMs: decision.RetryAfterMs}
		}
	}

	if d.opts.QuotaAdmission != nil {
		decision := d.opts.QuotaAdmission.CheckAdmission(ctx, quotaFromSnapshot(in.Identity.TenantID, snapshot.Quota))
		if !decision.Allowed {
			details := map[string]string{}
			if decision.Reason != "" {
				details["reason"] = decision.Reason
			}

			return &PipelineError{Code: QuotaExhausted, Details: details}
		}
	}

	return nil
}

func (d *DefaultRequestDispatcher) route(in DispatchInput, snapshot config.TenantSnapshot) RouteOutcome {
	return d.routeWithWorkers(in, snapshot, d.opts.Workers)
}

func (d *DefaultRequestDispatcher) routeWithWorkers(in DispatchInput, snapshot config.TenantSnapshot, workers CandidateSource) RouteOutcome {
	router := NewRouter(
		snapshotRules{tenantID: in.Identity.TenantID, rules: snapshot.RoutingRules},
		NewStaticPoolPolicyProvider(poolPoliciesFromSnapshot(in.Identity.TenantID, snapshot.ExecutorPools)),
		workers,
		d.opts.Sticky,
		d.opts.Now,
	)

	return router.Evaluate(RouteRequest{
		TenantID:        in.Identity.TenantID,
		Tags:            in.Request.Routing.Tags,
		Country:         in.Request.Routing.Country,
		Region:          in.Request.Routing.Region,
		IPType:          in.Request.Routing.IPType,
		IngressType:     in.Request.IngressType,
		TargetHost:      strings.ToLower(in.Request.URL.Hostname()),
		StickySessionID: in.Request.StickySessionID,
	})
}

func (d *DefaultRequestDispatcher) executeAttemptOrFallback(ctx context.Context, in DispatchInput, route RouteOutcome, snapshot config.TenantSnapshot, policy *DestinationPolicyResult, deadline time.Time) (dispatchResult, int64, *PipelineError) {
	result, assignmentMs, perr := d.executeAttempt(ctx, in, route, policy, snapshot.ConfigVersion, deadline)
	if perr == nil || !canFallbackBeforeRequestStart(perr.Code) {
		return result, assignmentMs, perr
	}

	routeFailure(d.opts.Workers, route.WorkerID)

	fallback := d.routeWithWorkers(in, snapshot, excludeWorkers{base: d.opts.Workers, workerID: route.WorkerID})
	if !fallback.OK {
		return result, assignmentMs, perr
	}

	fallbackResult, fallbackAssignmentMs, fallbackErr := d.executeAttempt(ctx, in, fallback, policy, snapshot.ConfigVersion, deadline)

	return fallbackResult, assignmentMs + fallbackAssignmentMs, fallbackErr
}

type excludeWorkers struct {
	base     CandidateSource
	workerID string
}

func (e excludeWorkers) CandidatesForPool(tenantID, poolID string) []PoolCandidate {
	candidates := e.base.CandidatesForPool(tenantID, poolID)

	out := candidates[:0]
	for _, candidate := range candidates {
		if candidate.WorkerID != e.workerID {
			out = append(out, candidate)
		}
	}

	return out
}

// poolPoliciesFromSnapshot converts a tenant snapshot's executor pools into
// the flat PoolPolicy list StaticPoolPolicyProvider expects, so degraded-pool
// routing policy (docs/planning/10) is sourced from the pools config admins
// manage through /api/v1/config/executor-pools (docs/tasks/p0/30) instead of a
// nil provider.
func poolPoliciesFromSnapshot(tenantID string, pools []config.ExecutorPool) []PoolPolicy {
	out := make([]PoolPolicy, 0, len(pools))
	for _, p := range pools {
		out = append(out, PoolPolicy{
			TenantID:             tenantID,
			PoolID:               p.ID,
			AllowDegradedWorkers: p.AllowDegradedWorkers,
			AllowedCountries:     p.AllowedCountries,
			AllowedRegions:       p.AllowedRegions,
			AllowedIPTypes:       p.AllowedIPTypes,
		})
	}

	return out
}

// executeAttempt runs one assignment-and-stream attempt, recording the
// straw_assignment_duration_seconds histogram (docs/planning/23) over the
// full attempt regardless of outcome.
func (d *DefaultRequestDispatcher) executeAttempt(ctx context.Context, in DispatchInput, route RouteOutcome, policy *DestinationPolicyResult, configVersion uint64, deadline time.Time) (dispatchResult, int64, *PipelineError) {
	result, assignmentMs, perr := d.executeAttemptUnmeasured(ctx, in, route, policy, configVersion, deadline)
	d.opts.Metrics.ObserveAssignment(time.Duration(assignmentMs) * time.Millisecond)

	return result, assignmentMs, perr
}

func (d *DefaultRequestDispatcher) executeAttemptUnmeasured(ctx context.Context, in DispatchInput, route RouteOutcome, policy *DestinationPolicyResult, configVersion uint64, deadline time.Time) (dispatchResult, int64, *PipelineError) {
	assignmentStarted := d.opts.Now()

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
		frames <- decodeDispatchFrame(msg.Data)
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

	ack, perr := d.requestAssign(route.AssignSubject, in, assign, deadline)
	if perr != nil {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), perr
	}

	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), assignRejectError(ack.GetCode())
	}

	assignmentMs := millisSince(assignmentStarted, d.opts.Now())

	nextSeq, err := d.sendRequestStart(c2eSubject, in, route, policy, configVersion, deadline)
	if err != nil {
		return dispatchResult{}, assignmentMs, &PipelineError{Code: TransportUnavailable}
	}

	result, perr := d.readResponse(ctx, frames, route, deadline, c2eSubject, in, nextSeq)

	return result, assignmentMs, perr
}

func (d *DefaultRequestDispatcher) executeRawAttempt(ctx context.Context, in DispatchInput, route RouteOutcome, policy *DestinationPolicyResult, configVersion uint64, deadline time.Time, w http.ResponseWriter) (dispatchResult, int64, *PipelineError, bool) {
	result, assignmentMs, perr, wroteHeader := d.executeRawAttemptUnmeasured(ctx, in, route, policy, configVersion, deadline, w)
	d.opts.Metrics.ObserveAssignment(time.Duration(assignmentMs) * time.Millisecond)

	return result, assignmentMs, perr, wroteHeader
}

func (d *DefaultRequestDispatcher) executeRawAttemptUnmeasured(ctx context.Context, in DispatchInput, route RouteOutcome, policy *DestinationPolicyResult, configVersion uint64, deadline time.Time, w http.ResponseWriter) (dispatchResult, int64, *PipelineError, bool) {
	assignmentStarted := d.opts.Now()

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
		frames <- decodeDispatchFrame(msg.Data)
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

	ack, perr := d.requestAssign(route.AssignSubject, in, assign, deadline)
	if perr != nil {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), perr, false
	}

	if ack.GetCode() != strawpb.AssignAckCode_ASSIGN_ACK_ACCEPTED {
		return dispatchResult{}, millisSince(assignmentStarted, d.opts.Now()), assignRejectError(ack.GetCode()), false
	}

	assignmentMs := millisSince(assignmentStarted, d.opts.Now())

	nextSeq, err := d.sendRequestStart(c2eSubject, in, route, policy, configVersion, deadline)
	if err != nil {
		return dispatchResult{}, assignmentMs, &PipelineError{Code: TransportUnavailable}, false
	}

	result, perr, wroteHeader := d.streamRawResponse(ctx, frames, route, deadline, c2eSubject, in, nextSeq, w)

	return result, assignmentMs, perr, wroteHeader
}

func (d *DefaultRequestDispatcher) requestAssign(subject string, in DispatchInput, assign *strawpb.AssignRequest, deadline time.Time) (*strawpb.AssignAck, *PipelineError) {
	env := &strawpb.Envelope{
		RequestId:      in.RequestID,
		TenantId:       in.Identity.TenantID,
		DeadlineUnixMs: deadline.UnixMilli(),
		ProtocolMajor:  ProtocolMajor,
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
	if err != nil || reply.GetAssignAck() == nil {
		d.opts.Metrics.IncNATSError(errorCodeLabel(ProtocolError))

		return nil, &PipelineError{Code: ProtocolError}
	}

	return reply.GetAssignAck(), nil
}

// sendRequestStart publishes RequestStart and inline body DataFrames on the c2e
// subject and returns the next c2e stream_seq (i.e. the seq a subsequent
// CancelFrame should use).
func (d *DefaultRequestDispatcher) sendRequestStart(subject string, in DispatchInput, route RouteOutcome, policy *DestinationPolicyResult, configVersion uint64, deadline time.Time) (uint64, error) {
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
		FingerprintInstruction: wireFingerprint(policy.FingerprintProfile),
		InjectionOperations:    policy.InjectionOperations,
		RedirectPolicy:         strawpb.RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		DestinationPolicy:      policy.Policy,
		PolicyVersion:          strconv.FormatUint(configVersion, 10),
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

	offset := uint64(0)

	body := in.Request.BodyData
	for len(body) > 0 {
		seq++
		n := frameChunkSize(len(body), d.opts.MaxFrameDataBytes)
		chunk := body[:n]
		body = body[n:]

		err = d.publishFrame(subject, in, deadline, &strawpb.StreamFrame{
			StreamSeq: seq,
			Attempt:   defaultRequestAttempt,
			Payload:   &strawpb.StreamFrame_Data{Data: &strawpb.DataFrame{Offset: offset, Data: chunk}},
		})
		if err != nil {
			return 0, err
		}

		offset += uint64(len(chunk))
	}

	err = d.opts.NATS.Flush()
	if err != nil {
		return 0, fmt.Errorf("flush request stream: %w", err)
	}

	return seq + 1, nil
}

func (d *DefaultRequestDispatcher) publishFrame(subject string, in DispatchInput, deadline time.Time, frame *strawpb.StreamFrame) error {
	env := &strawpb.Envelope{
		RequestId:      in.RequestID,
		TenantId:       in.Identity.TenantID,
		DeadlineUnixMs: deadline.UnixMilli(),
		ProtocolMajor:  ProtocolMajor,
		Attempt:        defaultRequestAttempt,
		Payload:        &strawpb.Envelope_StreamFrame{StreamFrame: frame},
	}

	raw, err := natsx.MarshalEnvelope(env)
	if err != nil {
		return fmt.Errorf("marshal stream frame: %w", err)
	}

	err = d.opts.NATS.Publish(subject, raw)
	if err != nil {
		return fmt.Errorf("publish stream frame: %w", err)
	}

	return nil
}

func (d *DefaultRequestDispatcher) readResponse(ctx context.Context, frames <-chan *strawpb.StreamFrame, route RouteOutcome, deadline time.Time, c2eSubject string, in DispatchInput, c2eSeq uint64) (dispatchResult, *PipelineError) {
	validator := natsx.NewStreamValidator(defaultRequestAttempt, d.initialDownloadCredit(), d.opts.FrameIdleTimeout, d.opts.Now)
	result := dispatchResult{status: http.StatusOK}
	egressStarted := time.Time{}

	ticker := time.NewTicker(responseFrameCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.sendCancel(c2eSubject, in, deadline, c2eSeq, "client_cancelled")

			return dispatchResult{}, &PipelineError{Code: Cancelled}
		case <-time.After(time.Until(deadline)):
			d.sendCancel(c2eSubject, in, deadline, c2eSeq, "deadline_exceeded")

			return dispatchResult{}, &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeTotalDeadline}
		case <-ticker.C:
			if perr := d.responseStreamTick(validator, route, c2eSubject, in, deadline, c2eSeq); perr != nil {
				return dispatchResult{}, perr
			}
		case frame, ok := <-frames:
			if !ok {
				return dispatchResult{}, d.streamLost(route, WorkerDisconnected)
			}

			done, perr := d.acceptResponseFrame(validator, frame, route, &result, &egressStarted, c2eSubject, in, deadline, &c2eSeq)
			if perr != nil {
				return dispatchResult{}, perr
			}

			if done {
				return result, nil
			}
		}
	}
}

func (d *DefaultRequestDispatcher) streamRawResponse(ctx context.Context, frames <-chan *strawpb.StreamFrame, route RouteOutcome, deadline time.Time, c2eSubject string, in DispatchInput, c2eSeq uint64, w http.ResponseWriter) (dispatchResult, *PipelineError, bool) {
	validator := natsx.NewStreamValidator(defaultRequestAttempt, d.initialDownloadCredit(), d.opts.FrameIdleTimeout, d.opts.Now)
	state := rawResponseStreamState{
		dispatcher: d,
		route:      route,
		deadline:   deadline,
		c2eSubject: c2eSubject,
		in:         in,
		c2eSeq:     c2eSeq,
		w:          w,
		result:     dispatchResult{status: http.StatusOK},
	}

	ticker := time.NewTicker(responseFrameCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.sendCancel(c2eSubject, in, deadline, state.c2eSeq, "client_cancelled")

			return state.result, &PipelineError{Code: Cancelled}, state.wroteHeader
		case <-time.After(time.Until(deadline)):
			d.sendCancel(c2eSubject, in, deadline, state.c2eSeq, "deadline_exceeded")

			return state.result, &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeTotalDeadline}, state.wroteHeader
		case <-ticker.C:
			if perr := d.responseStreamTick(validator, route, c2eSubject, in, deadline, state.c2eSeq); perr != nil {
				return state.result, perr, state.wroteHeader
			}
		case frame, ok := <-frames:
			if !ok {
				return state.result, d.streamLost(route, WorkerDisconnected), state.wroteHeader
			}

			done, perr := state.acceptValidated(frame, validator)
			if done || perr != nil {
				return state.result, perr, state.wroteHeader
			}
		}
	}
}

type rawResponseStreamState struct {
	dispatcher  *DefaultRequestDispatcher
	route       RouteOutcome
	deadline    time.Time
	c2eSubject  string
	in          DispatchInput
	c2eSeq      uint64
	w           http.ResponseWriter
	result      dispatchResult
	egressStart time.Time
	wroteHeader bool
}

func (s *rawResponseStreamState) acceptValidated(frame *strawpb.StreamFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	ok, done, perr := acceptedResponseFrame(validator.Accept(frame))
	if !ok || done {
		return true, perr
	}

	return s.accept(frame, validator)
}

func (s *rawResponseStreamState) accept(frame *strawpb.StreamFrame, validator *natsx.StreamValidator) (bool, *PipelineError) {
	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_OutboundStart:
		s.egressStart = s.dispatcher.opts.Now()
	case *strawpb.StreamFrame_ResponseStart:
		s.result.status = p.ResponseStart.GetStatus()
		s.result.headers = p.ResponseStart.GetHeaders()
		writeRawResponseStart(s.w, s.result.status, s.result.headers)

		s.wroteHeader = true
	case *strawpb.StreamFrame_Data:
		return false, s.writeData(p.Data, validator)
	case *strawpb.StreamFrame_Trailers:
		if s.wroteHeader {
			writeRawTrailers(s.w, p.Trailers.GetHeaders())
		}
	case *strawpb.StreamFrame_Error:
		return true, s.dispatcher.executorError(s.route, p.Error)
	case *strawpb.StreamFrame_Cancelled:
		return true, &PipelineError{Code: Cancelled}
	case *strawpb.StreamFrame_End:
		s.result.egressMs = egressMillis(s.egressStart, s.dispatcher.opts.Now())

		return true, nil
	default:
		routeFailure(s.dispatcher.opts.Workers, s.route.WorkerID)

		return true, &PipelineError{Code: ProtocolError}
	}

	return false, nil
}

func (s *rawResponseStreamState) writeData(data *strawpb.DataFrame, validator *natsx.StreamValidator) *PipelineError {
	if !s.wroteHeader {
		routeFailure(s.dispatcher.opts.Workers, s.route.WorkerID)

		return &PipelineError{Code: ProtocolError}
	}

	n, err := s.w.Write(data.GetData())
	if err != nil {
		s.dispatcher.sendCancel(s.c2eSubject, s.in, s.deadline, s.c2eSeq, "client_disconnect")

		return &PipelineError{Code: Cancelled}
	}

	written := uint64FromInt(n)
	s.result.size += written

	flushRawResponse(s.w)
	s.c2eSeq = s.dispatcher.sendDownloadCredit(s.c2eSubject, s.in, s.deadline, s.c2eSeq, written)
	validator.GrantCredit(written)

	return nil
}

// sendCancel publishes a best-effort CancelFrame to the c2e subject.
// Cancellation is best-effort per docs/planning/09; errors are silently
// dropped.
func (d *DefaultRequestDispatcher) sendCancel(c2eSubject string, in DispatchInput, deadline time.Time, seq uint64, reason string) {
	if d.opts.NATS == nil {
		return
	}

	_ = d.publishFrame(c2eSubject, in, deadline, &strawpb.StreamFrame{
		StreamSeq: seq,
		Attempt:   defaultRequestAttempt,
		Payload:   &strawpb.StreamFrame_Cancel{Cancel: &strawpb.CancelFrame{Reason: reason}},
	})
}

func (d *DefaultRequestDispatcher) sendDownloadCredit(c2eSubject string, in DispatchInput, deadline time.Time, seq, bytes uint64) uint64 {
	if bytes == 0 || d.opts.NATS == nil {
		return seq
	}

	_ = d.publishFrame(c2eSubject, in, deadline, &strawpb.StreamFrame{
		StreamSeq: seq,
		Attempt:   defaultRequestAttempt,
		Payload:   &strawpb.StreamFrame_Credit{Credit: &strawpb.CreditFrame{DownloadCreditBytes: bytes}},
	})
	_ = d.opts.NATS.Flush()

	return seq + 1
}

func (d *DefaultRequestDispatcher) initialDownloadCredit() uint64 {
	return min(d.opts.InitialDownloadCreditBytes, d.opts.MaxInflightDownloadBytes)
}

func (d *DefaultRequestDispatcher) acceptResponseFrame(validator *natsx.StreamValidator, frame *strawpb.StreamFrame, route RouteOutcome, result *dispatchResult, egressStarted *time.Time, c2eSubject string, in DispatchInput, deadline time.Time, c2eSeq *uint64) (bool, *PipelineError) {
	ok, done, perr := acceptedResponseFrame(validator.Accept(frame))
	if !ok || done {
		return done, perr
	}

	if handled, perr := d.acceptResponseProgress(frame, result, egressStarted, validator, c2eSubject, in, deadline, c2eSeq); handled {
		return false, perr
	}

	return d.acceptResponseTerminal(frame, route, result, egressStarted)
}

func (d *DefaultRequestDispatcher) acceptResponseProgress(frame *strawpb.StreamFrame, result *dispatchResult, egressStarted *time.Time, validator *natsx.StreamValidator, c2eSubject string, in DispatchInput, deadline time.Time, c2eSeq *uint64) (bool, *PipelineError) {
	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_OutboundStart:
		*egressStarted = d.opts.Now()
	case *strawpb.StreamFrame_ResponseStart:
		result.status = p.ResponseStart.GetStatus()
		result.headers = p.ResponseStart.GetHeaders()
	case *strawpb.StreamFrame_Data:
		_, perr := d.acceptResponseData(p.Data, result, validator, c2eSubject, in, deadline, c2eSeq)

		return true, perr
	default:
		return false, nil
	}

	return true, nil
}

func (d *DefaultRequestDispatcher) acceptResponseTerminal(frame *strawpb.StreamFrame, route RouteOutcome, result *dispatchResult, egressStarted *time.Time) (bool, *PipelineError) {
	switch p := frame.GetPayload().(type) {
	case *strawpb.StreamFrame_Error:
		return true, d.executorError(route, p.Error)
	case *strawpb.StreamFrame_Cancelled:
		return true, &PipelineError{Code: Cancelled}
	case *strawpb.StreamFrame_End:
		result.egressMs = egressMillis(*egressStarted, d.opts.Now())

		return true, nil
	case *strawpb.StreamFrame_Trailers:
		return false, nil
	default:
		routeFailure(d.opts.Workers, route.WorkerID)

		return true, &PipelineError{Code: ProtocolError}
	}
}

func acceptedResponseFrame(outcome natsx.FrameOutcome) (bool, bool, *PipelineError) {
	if outcome == natsx.FrameDuplicate || outcome == natsx.FrameAfterTerminal {
		return false, false, nil
	}

	if outcome != natsx.FrameAccepted {
		return false, true, &PipelineError{Code: ProtocolError}
	}

	return true, false, nil
}

func (d *DefaultRequestDispatcher) acceptResponseData(data *strawpb.DataFrame, result *dispatchResult, validator *natsx.StreamValidator, c2eSubject string, in DispatchInput, deadline time.Time, c2eSeq *uint64) (bool, *PipelineError) {
	result.body = append(result.body, data.GetData()...)
	if len(result.body) <= responseBodyLimit(d.opts.MaxInlineResponseBodyBytes) {
		bytes := uint64FromInt(len(data.GetData()))
		*c2eSeq = d.sendDownloadCredit(c2eSubject, in, deadline, *c2eSeq, bytes)
		validator.GrantCredit(bytes)

		return false, nil
	}

	return true, &PipelineError{
		Code: BodyTooLarge,
		Details: map[string]string{
			errorDetailDirectionKey:  "response",
			errorDetailLimitBytesKey: strconv.FormatUint(d.opts.MaxInlineResponseBodyBytes, 10),
		},
	}
}

func (d *DefaultRequestDispatcher) executorError(route RouteOutcome, frame *strawpb.ErrorFrame) *PipelineError {
	code, violation := ValidateExecutorError(frame.GetCode())
	if violation {
		routeFailure(d.opts.Workers, route.WorkerID)
	}

	return errorFramePipelineError(code, frame)
}

func (d *DefaultRequestDispatcher) responseStreamTick(validator *natsx.StreamValidator, route RouteOutcome, c2eSubject string, in DispatchInput, deadline time.Time, cancelSeq uint64) *PipelineError {
	if d.natsDisconnected() {
		return d.streamLost(route, TransportUnavailable)
	}

	if validator.IdleExpired() {
		d.sendCancel(c2eSubject, in, deadline, cancelSeq, "idle_timeout")

		return &PipelineError{Code: TimeoutExceeded, TimeoutType: timeoutTypeIdle}
	}

	return nil
}

func (d *DefaultRequestDispatcher) streamLost(route RouteOutcome, code ErrorCode) *PipelineError {
	routeFailure(d.opts.Workers, route.WorkerID)

	if code == TransportUnavailable {
		d.opts.Metrics.IncNATSError(errorCodeLabel(code))
	}

	return &PipelineError{Code: code}
}

func (d *DefaultRequestDispatcher) natsDisconnected() bool {
	return d.opts.NATS != nil && !d.opts.NATS.IsConnected()
}

func egressMillis(start, end time.Time) int64 {
	if start.IsZero() {
		return 0
	}

	return millisSince(start, end)
}

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

	return int64(len(req.BodyData))
}

func (d *DefaultRequestDispatcher) deadline(req *ValidatedRequest, snapshot config.TenantSnapshot) time.Time {
	timeoutMs := req.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = effectiveDefaultTimeout(d.opts.MaxTimeoutMs, snapshot.DefaultTimeoutMs)
	}

	if timeoutMs == 0 {
		timeoutMs = defaultRequestTimeoutFallback
	}

	return d.opts.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
}

func effectiveDefaultTimeout(staticMax, tenantDefault uint64) uint64 {
	if tenantDefault == 0 {
		tenantDefault = defaultTenantDefaultTimeoutMs
	}

	if staticMax != 0 && tenantDefault > staticMax {
		return staticMax
	}

	return tenantDefault
}

type dispatchResult struct {
	status   uint32
	headers  []*strawpb.Header
	body     []byte
	size     uint64
	egressMs int64
}

type snapshotRules struct {
	tenantID string
	rules    []config.RoutingRule
}

func (s snapshotRules) RulesForTenant(tenantID string) []RoutingRule {
	if tenantID != s.tenantID {
		return nil
	}

	out := make([]RoutingRule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, RoutingRule{
			ID:                      r.ID,
			TenantID:                tenantID,
			Priority:                r.Priority,
			Enabled:                 r.Enabled,
			Match:                   matchFromSnapshot(r.Match),
			TargetPoolID:            r.TargetPoolID,
			StickySessionTTLSeconds: r.StickySessionTTLSeconds,
			AllowStickyFallback:     r.AllowStickyFallback,
		})
	}

	return out
}

func matchFromSnapshot(m config.MatchConditions) MatchConditions {
	return MatchConditions{
		Tags:        append([]string(nil), m.Tags...),
		Country:     m.Country,
		Region:      m.Region,
		IPType:      m.IPType,
		IngressType: m.IngressType,
		TargetHost:  m.TargetHost,
	}
}

func rateLimitFromSnapshot(tenantID string, rules []config.RateLimitRule) RateLimitConfig {
	out := RateLimitConfig{TenantID: tenantID}
	for _, r := range rules {
		out.Limits = append(out.Limits, RateLimitRule{
			Dimension:     RateLimitDimension(r.Dimension),
			Key:           r.Key,
			WindowSeconds: r.WindowSeconds,
			MaxRequests:   r.MaxRequests,
			FailPolicy:    RateLimitFailPolicy(r.FailPolicy),
		})
	}

	return out
}

func quotaFromSnapshot(tenantID string, q config.QuotaConfig) QuotaConfig {
	policy := quotaRequestCountOnSuccess
	if q.CountOnAdmission {
		policy = quotaRequestCountOnAdmission
	}

	maxRequests := q.RequestCountLimit
	maxBandwidth := q.BandwidthBytesLimit

	if !q.Enabled {
		maxRequests = 0
		maxBandwidth = 0
	}

	return QuotaConfig{
		TenantID:           tenantID,
		Period:             quotaPeriodMonthly,
		MaxRequests:        maxRequests,
		MaxBandwidthBytes:  maxBandwidth,
		RequestCountPolicy: policy,
		RedisFailPolicy:    q.FailPolicy,
	}
}

func routeError(code string) *PipelineError {
	switch code {
	case RouteErrNoMatch:
		return &PipelineError{Code: RouteNoMatch}
	case RouteErrStickyUnavailable:
		return &PipelineError{Code: StickySessionUnavailable}
	case RouteErrCapacityExhausted:
		return &PipelineError{Code: ExecutorCapacityExhausted}
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

func responseBodyLimit(limit uint64) int {
	n, err := strconv.Atoi(strconv.FormatUint(limit, 10))
	if err != nil {
		return math.MaxInt
	}

	return n
}

func timeoutTypeName(t strawpb.TimeoutType) string {
	name := strings.TrimPrefix(t.String(), "TIMEOUT_TYPE_")
	name = strings.ToLower(name)

	return name
}

func wireFingerprint(profile string) string {
	if profile == defaultFingerprintProfileName {
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

func writeRawResponseStart(w http.ResponseWriter, status uint32, headers []*strawpb.Header) {
	if stream, ok := w.(interface {
		WriteRawResponseStart(status uint32, headers []*strawpb.Header)
	}); ok {
		stream.WriteRawResponseStart(status, headers)

		return
	}

	for _, h := range headers {
		if !rawResponseHeaderAllowed(h.GetName()) {
			continue
		}

		w.Header().Add(h.GetName(), string(h.GetValue()))
	}

	w.WriteHeader(int(status))
	flushRawResponse(w)
}

func writeRawTrailers(w http.ResponseWriter, trailers []*strawpb.Header) {
	if stream, ok := w.(interface {
		WriteRawTrailers(headers []*strawpb.Header)
	}); ok {
		stream.WriteRawTrailers(trailers)

		return
	}

	for _, h := range trailers {
		if !rawResponseHeaderAllowed(h.GetName()) {
			continue
		}

		w.Header().Add(http.TrailerPrefix+h.GetName(), string(h.GetValue()))
	}

	flushRawResponse(w)
}

func rawResponseHeaderAllowed(name string) bool {
	if !isValidHTTPToken(name) || strings.HasPrefix(strings.ToLower(name), "x-straw-") {
		return false
	}

	switch strings.ToLower(name) {
	case headerNameTransferEncoding, headerNameContentLength, headerNameConnection, headerNameProxyAuthorization:
		return false
	default:
		return true
	}
}

func flushRawResponse(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func decodeDispatchFrame(raw []byte) *strawpb.StreamFrame {
	env, err := natsx.UnmarshalEnvelope(raw)
	if err != nil {
		return nil
	}

	return env.GetStreamFrame()
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

func int64FromUint64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}

	return int64(v)
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
