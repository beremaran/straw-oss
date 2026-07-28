package control

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/natsx"
	"github.com/beremaran/straw-oss/internal/receipt"
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
	defaultRequestTimeoutMS       = 60000
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
	// ProtocolMinor is set per selected route before assignment/streaming.
	ProtocolMinor uint32
}

// PipelineError is a canonical dispatch failure.
type PipelineError struct {
	Code           ErrorCode
	Message        string
	Details        map[string]string
	RetryAfterMs   int64
	TimeoutType    string
	UpstreamStatus *uint32
	// RoutingMs/AssignmentMs/EgressMs/TotalMs carry whatever partial phase
	// timing the dispatcher measured before the failure, so a failed
	RoutingMs                  int64
	AssignmentMs               int64
	EgressMs                   int64
	TotalMs                    int64
	SelectedFingerprintProfile string
	ExecutedFingerprintProfile string
}

// RequestDispatcherOptions wires the Control request pipeline.
type RequestDispatcherOptions struct {
	ConfigCache                *ConfigCache
	Workers                    CandidateSource
	Sticky                     StickyBackend
	NATS                       *natsx.Connection
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
	// InFlight tracks request-scoped cancellation and client disconnects.
	InFlight *InFlightRegistry
	// Metrics records Prometheus request and transport series.
	Metrics *Metrics
	// Receipts stores opt-in response bodies without retaining them in Control memory.
	Receipts *receipt.Service
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
// (docs/public/architecture.md) around the attempt.
func (d *DefaultRequestDispatcher) Dispatch(ctx context.Context, in DispatchInput) (SuccessResponse, *PipelineError) {
	started := d.opts.Now()

	d.opts.Metrics.IncActiveRequests()
	defer d.opts.Metrics.DecActiveRequests()

	resp, perr := d.dispatch(ctx, in, started)

	var code ErrorCode
	if perr != nil {
		code = perr.Code
	}

	d.opts.Metrics.ObserveRequest(errorCodeLabel(code), d.opts.Now().Sub(started))

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

	d.opts.Metrics.ObserveRequest(errorCodeLabel(code), d.opts.Now().Sub(started))

	return resp, perr, wroteHeader
}

// DispatchTunnel runs the Control pipeline for one raw CONNECT byte stream.
// The CONNECT success response is written only after Egress has opened the
// validated upstream socket.
func (d *DefaultRequestDispatcher) DispatchTunnel(ctx context.Context, in DispatchInput, rw io.ReadWriter) (SuccessResponse, *PipelineError) {
	started := d.opts.Now()

	d.opts.Metrics.IncActiveRequests()
	defer d.opts.Metrics.DecActiveRequests()

	resp, perr := d.dispatchTunnel(ctx, in, rw, started)

	var code ErrorCode
	if perr != nil {
		code = perr.Code
	}

	d.opts.Metrics.ObserveRequest(errorCodeLabel(code), d.opts.Now().Sub(started))

	return resp, perr
}

func (d *DefaultRequestDispatcher) dispatch(ctx context.Context, in DispatchInput, started time.Time) (SuccessResponse, *PipelineError) {
	if in.Request == nil || d.opts.ConfigCache == nil || d.opts.Workers == nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	d.opts.InFlight.Register(ctx, in.RequestID, in.Identity.DeploymentID, cancel)
	defer d.opts.InFlight.Deregister(ctx, in.RequestID)

	snapshot := d.opts.ConfigCache.Snapshot()

	routeStart := d.opts.Now()
	route := d.route(in, snapshot)
	routeEnd := d.opts.Now()
	routingMs := millisSince(routeStart, routeEnd)
	d.opts.Metrics.ObserveRouting(routeEnd.Sub(routeStart))

	if !route.OK {
		return SuccessResponse{}, d.withTiming(routeError(route.ErrorCode), routingMs, 0, started)
	}

	deadline := d.deadline(in.Request, snapshot)

	result, assignmentMs, perr, usedRoute := d.executeAttemptOrFallback(ctx, in, route, snapshot, deadline)

	if perr != nil {
		perr = d.withTiming(perr, routingMs, assignmentMs, started)
		perr.EgressMs = result.egressMs
		setProfileFields(perr, result)

		return SuccessResponse{}, perr
	}

	return d.finalizeDispatch(ctx, in, snapshot, result, usedRoute, routingMs, assignmentMs, started)
}

// finalizeDispatch builds the buffered response envelope.
func (d *DefaultRequestDispatcher) finalizeDispatch(_ context.Context, in DispatchInput, _ config.Snapshot, result dispatchResult, _ RouteOutcome, routingMs, assignmentMs int64, started time.Time) (SuccessResponse, *PipelineError) {
	resp := successFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now()))
	setProfileFieldsOnResponse(&resp, result)

	return resp, nil
}

func setProfileFields(perr *PipelineError, result dispatchResult) {
	perr.SelectedFingerprintProfile = result.selectedFingerprintProfile
	perr.ExecutedFingerprintProfile = result.executedFingerprintProfile
}

func setProfileFieldsOnResponse(resp *SuccessResponse, result dispatchResult) {
	resp.SelectedFingerprintProfile = result.selectedFingerprintProfile
	resp.ExecutedFingerprintProfile = result.executedFingerprintProfile
}

func (d *DefaultRequestDispatcher) dispatchRaw(ctx context.Context, in DispatchInput, w http.ResponseWriter, started time.Time) (SuccessResponse, *PipelineError, bool) {
	if in.Request == nil || d.opts.ConfigCache == nil || d.opts.Workers == nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started), false
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	d.opts.InFlight.Register(ctx, in.RequestID, in.Identity.DeploymentID, cancel)
	defer d.opts.InFlight.Deregister(ctx, in.RequestID)

	snapshot := d.opts.ConfigCache.Snapshot()

	routeStart := d.opts.Now()
	route := d.route(in, snapshot)
	routeEnd := d.opts.Now()
	routingMs := millisSince(routeStart, routeEnd)
	d.opts.Metrics.ObserveRouting(routeEnd.Sub(routeStart))

	if !route.OK {
		return SuccessResponse{}, d.withTiming(routeError(route.ErrorCode), routingMs, 0, started), false
	}

	deadline := d.deadline(in.Request, snapshot)

	result, assignmentMs, perr, wroteHeader, usedRoute := d.executeRawAttemptOrFallback(ctx, in, route, snapshot, deadline, w)

	return d.finalizeRawDispatch(ctx, in, snapshot, usedRoute, result, perr, routingMs, assignmentMs, started, wroteHeader)
}

func (d *DefaultRequestDispatcher) dispatchTunnel(ctx context.Context, in DispatchInput, rw io.ReadWriter, started time.Time) (SuccessResponse, *PipelineError) {
	if in.Request == nil || rw == nil || d.opts.ConfigCache == nil || d.opts.Workers == nil {
		return SuccessResponse{}, d.withTiming(&PipelineError{Code: ControlInternalError}, 0, 0, started)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	d.opts.InFlight.Register(ctx, in.RequestID, in.Identity.DeploymentID, cancel)
	defer d.opts.InFlight.Deregister(ctx, in.RequestID)

	snapshot := d.opts.ConfigCache.Snapshot()

	routeStart := d.opts.Now()
	route := d.route(in, snapshot)
	routeEnd := d.opts.Now()
	routingMs := millisSince(routeStart, routeEnd)
	d.opts.Metrics.ObserveRouting(routeEnd.Sub(routeStart))

	if !route.OK {
		return SuccessResponse{}, d.withTiming(routeError(route.ErrorCode), routingMs, 0, started)
	}

	deadline := d.deadline(in.Request, snapshot)
	in.Request.BodyReader = io.NopCloser(rw)

	result, assignmentMs, perr, _ := d.executeTunnelAttemptOrFallback(ctx, in, route, snapshot, deadline, rw)
	if perr != nil {
		perr = d.withTiming(perr, routingMs, assignmentMs, started)
		perr.EgressMs = result.egressMs
		resp := rawSuccessFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now()))

		return resp, perr
	}

	resp := rawSuccessFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now()))

	return resp, nil
}

// finalizeRawDispatch mirrors finalizeDispatch for the raw-response path and
// attaches routing and timing details to the result.
func (d *DefaultRequestDispatcher) finalizeRawDispatch(_ context.Context, in DispatchInput, _ config.Snapshot, _ RouteOutcome, result dispatchResult, perr *PipelineError, routingMs, assignmentMs int64, started time.Time, wroteHeader bool) (SuccessResponse, *PipelineError, bool) {
	if perr != nil {
		perr = d.withTiming(perr, routingMs, assignmentMs, started)
		perr.EgressMs = result.egressMs
		setProfileFields(perr, result)

		resp := rawSuccessFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now()))
		setProfileFieldsOnResponse(&resp, result)

		return resp, perr, wroteHeader
	}

	resp := rawSuccessFromDispatch(in.RequestID, result, routingMs, assignmentMs, millisSince(started, d.opts.Now()))
	setProfileFieldsOnResponse(&resp, result)

	return resp, nil, wroteHeader
}

// withTiming annotates perr with whatever partial phase timing the dispatcher measured before the failure.
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
		Body:      responseBodyFromDispatch(result),
		Timing: RequestTiming{
			RoutingMs:    routingMs,
			AssignmentMs: assignmentMs,
			EgressMs:     result.egressMs,
			TotalMs:      totalMs,
		},
		ResponseSizeBytes:          result.size,
		SelectedFingerprintProfile: result.selectedFingerprintProfile,
		ExecutedFingerprintProfile: result.executedFingerprintProfile,
	}
}

func responseBodyFromDispatch(result dispatchResult) ResponseBody {
	if result.responseReceiptID != "" {
		return ResponseBody{Mode: bodyModeReceipt, ReceiptID: result.responseReceiptID, SizeBytes: result.size, SHA256Hex: result.responseReceiptSHA256}
	}

	return ResponseBody{
		Mode:       bodyModeInlineBase64,
		DataBase64: base64.StdEncoding.EncodeToString(result.body),
		Truncated:  false,
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
		ResponseSizeBytes:          result.size,
		SelectedFingerprintProfile: result.selectedFingerprintProfile,
		ExecutedFingerprintProfile: result.executedFingerprintProfile,
	}
}

func (d *DefaultRequestDispatcher) route(in DispatchInput, snapshot config.Snapshot) RouteOutcome {
	return d.routeWithWorkers(in, snapshot, d.opts.Workers)
}

func (d *DefaultRequestDispatcher) routeWithWorkers(in DispatchInput, snapshot config.Snapshot, workers CandidateSource) RouteOutcome {
	router := NewRouter(
		snapshotRules{deploymentID: in.Identity.DeploymentID, rules: snapshot.RoutingRules},
		NewStaticPoolPolicyProvider(poolPoliciesFromSnapshot(in.Identity.DeploymentID, snapshot.ExecutorPools)),
		workers,
		d.opts.Sticky,
		d.opts.Now,
	)

	return router.Evaluate(RouteRequest{
		DeploymentID:       in.Identity.DeploymentID,
		Tags:               in.Request.Routing.Tags,
		Country:            in.Request.Routing.Country,
		Region:             in.Request.Routing.Region,
		IPType:             in.Request.Routing.IPType,
		IngressType:        in.Request.IngressType,
		TargetHost:         strings.ToLower(in.Request.URL.Hostname()),
		StickySessionID:    in.Request.Routing.StickySessionID,
		FingerprintProfile: in.Request.Fingerprint,
	})
}
