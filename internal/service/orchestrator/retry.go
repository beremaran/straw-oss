package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
	"github.com/google/uuid"
)

const (
	DefaultMaxRetries      = 2
	DefaultBaseBackoff     = 100 * time.Millisecond
	DefaultMaxBackoff      = 5 * time.Second
	DefaultBackoffFactor   = 2.0
	DefaultLastExitRetries = 1
	SharedResultSubject    = "results.relay"
)

type AttemptError struct {
	Pool int `json:"pool"`

	Attempt int `json:"attempt"`

	EndpointID string `json:"endpoint_id"`

	Failure FailureType `json:"failure"`

	FailureString string `json:"failure_type"`

	Message string `json:"message"`

	Duration time.Duration `json:"duration"`
}

type RetryResult struct {
	Success bool

	Response *ResultMessage

	FinalPool int

	TotalRetries int

	AttemptErrors []AttemptError
}

type PoolManager interface {
	GetEndpointFromPool(ctx context.Context, rule *domain.RoutingRule, poolTier int, exclude []string) (string, error)

	GetPoolConfig(rule *domain.RoutingRule, poolTier int) *domain.EndpointPool
}

type RetryExecutor struct {
	publisher   *Publisher
	consumer    *Consumer
	poolManager PoolManager
	broker      broker.MessageBroker
	hmacSecret  []byte
	logger      *slog.Logger

	responseChans sync.Map

	baseBackoff   time.Duration
	maxBackoff    time.Duration
	backoffFactor float64
}

type RetryExecutorOption func(*RetryExecutor)

func WithRetryLogger(logger *slog.Logger) RetryExecutorOption {
	return func(r *RetryExecutor) {
		r.logger = logger
	}
}

func WithBackoffConfig(base, max time.Duration, factor float64) RetryExecutorOption {
	return func(r *RetryExecutor) {
		r.baseBackoff = base
		r.maxBackoff = max
		r.backoffFactor = factor
	}
}

func NewRetryExecutor(
	publisher *Publisher,
	consumer *Consumer,
	poolManager PoolManager,
	b broker.MessageBroker,
	hmacSecret []byte,
	opts ...RetryExecutorOption,
) *RetryExecutor {
	r := &RetryExecutor{
		publisher:     publisher,
		consumer:      consumer,
		poolManager:   poolManager,
		broker:        b,
		hmacSecret:    hmacSecret,
		logger:        slog.Default(),
		baseBackoff:   DefaultBaseBackoff,
		maxBackoff:    DefaultMaxBackoff,
		backoffFactor: DefaultBackoffFactor,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *RetryExecutor) Start(ctx context.Context) error {
	return r.broker.Subscribe(ctx, SharedResultSubject, r.handleResult, broker.WithTransient(), broker.WithMaxAckPending(5000))
}

func (r *RetryExecutor) Execute(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	sessionID string,
	preferredEndpointID string,
) (*RetryResult, error) {
	result := &RetryResult{
		AttemptErrors: make([]AttemptError, 0),
	}

	pools := r.getPoolTiers(rule)
	if len(pools) == 0 {
		pools = []int{1}
	}

	r.logger.DebugContext(ctx, "starting retry execution",
		"request_id", req.ID,
		"pools", pools,
		"rule_id", rule.ID,
	)

	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	resultCh := make(chan *ResultMessage, 1)
	r.responseChans.Store(req.ID, resultCh)
	defer r.responseChans.Delete(req.ID)

	var excludedEndpoints []string

	if preferredEndpointID != "" && r.executePreferredEndpoint(ctx, req, rule, result, sessionID, resultCh, preferredEndpointID, &excludedEndpoints) {
		return result, nil
	}

	done, err := r.executePools(ctx, req, rule, result, sessionID, resultCh, pools, &excludedEndpoints)
	if err != nil || done {
		return result, err
	}

	r.logger.WarnContext(ctx, "all pools exhausted",
		"request_id", req.ID,
		"total_retries", result.TotalRetries,
	)

	return result, nil
}

func (r *RetryExecutor) executePreferredEndpoint(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	result *RetryResult,
	sessionID string,
	resultCh <-chan *ResultMessage,
	preferredEndpointID string,
	excludedEndpoints *[]string,
) bool {
	r.logger.DebugContext(ctx, "attempting sticky session endpoint",
		"request_id", req.ID,
		"endpoint_id", preferredEndpointID,
	)

	response, endpointID, err := r.executeAttempt(ctx, req, rule, 1, *excludedEndpoints, sessionID, resultCh, preferredEndpointID)
	if err != nil {
		r.logger.WarnContext(ctx, "sticky endpoint execution error", "error", err)
		*excludedEndpoints = append(*excludedEndpoints, preferredEndpointID)

		return false
	}

	failure := ClassifyFailure(response)
	if failure == FailureNone {
		result.Success = true
		result.Response = response
		result.FinalPool = 1

		return true
	}

	r.logger.InfoContext(ctx, "sticky endpoint failed, falling back to pools",
		"request_id", req.ID,
		"endpoint_id", endpointID,
		"failure", failure.String(),
	)
	r.recordFailure(result, 1, 1, endpointID, failure, 0, response)
	*excludedEndpoints = append(*excludedEndpoints, endpointID)
	ReleaseResultMessage(response)

	return false
}

func (r *RetryExecutor) executePools(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	result *RetryResult,
	sessionID string,
	resultCh <-chan *ResultMessage,
	pools []int,
	excludedEndpoints *[]string,
) (bool, error) {
	for _, poolTier := range pools {
		done, err := r.executePool(ctx, req, rule, result, sessionID, resultCh, poolTier, excludedEndpoints)
		if err != nil || done {
			return done, err
		}

		r.logger.InfoContext(ctx, "pool exhausted, escalating",
			"request_id", req.ID,
			"pool", poolTier,
		)
	}

	return false, nil
}

func (r *RetryExecutor) executePool(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	result *RetryResult,
	sessionID string,
	resultCh <-chan *ResultMessage,
	poolTier int,
	excludedEndpoints *[]string,
) (bool, error) {
	poolConfig := r.poolManager.GetPoolConfig(rule, poolTier)
	maxRetries := r.getMaxRetries(poolConfig, poolTier)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		done, stopPool, err := r.executePoolAttempt(ctx, req, rule, result, sessionID, resultCh, poolTier, attempt, maxRetries, excludedEndpoints)
		if err != nil || done || stopPool {
			return done, err
		}
	}

	return false, nil
}

func (r *RetryExecutor) executePoolAttempt(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	result *RetryResult,
	sessionID string,
	resultCh <-chan *ResultMessage,
	poolTier int,
	attempt int,
	maxRetries int,
	excludedEndpoints *[]string,
) (bool, bool, error) {
	attemptStart := time.Now()
	response, endpointID, err := r.executeAttempt(ctx, req, rule, poolTier, *excludedEndpoints, sessionID, resultCh, "")
	attemptDuration := time.Since(attemptStart)
	if err != nil {
		r.recordInternalFailure(ctx, req, result, poolTier, attempt, endpointID, attemptDuration, err, excludedEndpoints)

		return false, false, nil
	}

	failure := ClassifyFailure(response)
	if failure == FailureNone {
		r.recordSuccess(ctx, req, result, poolTier, attempt, endpointID, response)

		return true, false, nil
	}

	r.recordFailedResponse(ctx, req, result, poolTier, attempt, endpointID, attemptDuration, failure, response, excludedEndpoints)
	if failure.ShouldEscalate() {
		ReleaseResultMessage(response)

		return false, true, nil
	}
	if !failure.ShouldRetry() {
		result.Response = response
		result.FinalPool = poolTier

		return true, false, nil
	}

	ReleaseResultMessage(response)

	return false, false, r.backoffBeforeRetry(ctx, req, failure, attempt, maxRetries)
}

func (r *RetryExecutor) recordInternalFailure(
	ctx context.Context,
	req *protocol.Request,
	result *RetryResult,
	poolTier int,
	attempt int,
	endpointID string,
	duration time.Duration,
	err error,
	excludedEndpoints *[]string,
) {
	result.AttemptErrors = append(result.AttemptErrors, AttemptError{
		Pool:          poolTier,
		Attempt:       attempt,
		EndpointID:    endpointID,
		Failure:       FailureInternal,
		FailureString: FailureInternal.String(),
		Message:       err.Error(),
		Duration:      duration,
	})
	result.TotalRetries++
	if endpointID != "" {
		*excludedEndpoints = append(*excludedEndpoints, endpointID)
	}

	r.logger.WarnContext(ctx, "attempt failed with error",
		"request_id", req.ID,
		"pool", poolTier,
		"attempt", attempt,
		"endpoint", endpointID,
		"error", err,
	)
}

func (r *RetryExecutor) recordSuccess(
	ctx context.Context,
	req *protocol.Request,
	result *RetryResult,
	poolTier int,
	attempt int,
	endpointID string,
	response *ResultMessage,
) {
	result.Success = true
	result.Response = response
	result.FinalPool = poolTier

	r.logger.DebugContext(ctx, "request succeeded",
		"request_id", req.ID,
		"pool", poolTier,
		"attempt", attempt,
		"endpoint", endpointID,
		"total_retries", result.TotalRetries,
	)
}

func (r *RetryExecutor) recordFailedResponse(
	ctx context.Context,
	req *protocol.Request,
	result *RetryResult,
	poolTier int,
	attempt int,
	endpointID string,
	duration time.Duration,
	failure FailureType,
	response *ResultMessage,
	excludedEndpoints *[]string,
) {
	r.recordFailure(result, poolTier, attempt, endpointID, failure, duration, response)
	*excludedEndpoints = append(*excludedEndpoints, endpointID)

	r.logger.WarnContext(ctx, "attempt failed",
		"request_id", req.ID,
		"pool", poolTier,
		"attempt", attempt,
		"endpoint", endpointID,
		"failure", failure.String(),
		"status_code", response.StatusCode,
	)
	if failure.ShouldEscalate() {
		r.logger.InfoContext(ctx, "immediate escalation triggered",
			"request_id", req.ID,
			"pool", poolTier,
			"failure", failure.String(),
		)
	}
}

func (r *RetryExecutor) recordFailure(
	result *RetryResult,
	poolTier int,
	attempt int,
	endpointID string,
	failure FailureType,
	duration time.Duration,
	response *ResultMessage,
) {
	result.AttemptErrors = append(result.AttemptErrors, AttemptError{
		Pool:          poolTier,
		Attempt:       attempt,
		EndpointID:    endpointID,
		Failure:       failure,
		FailureString: failure.String(),
		Message:       r.getFailureMessage(response),
		Duration:      duration,
	})
	result.TotalRetries++
}

func (r *RetryExecutor) backoffBeforeRetry(
	ctx context.Context,
	req *protocol.Request,
	failure FailureType,
	attempt int,
	maxRetries int,
) error {
	if !failure.RequiresBackoff() || attempt >= maxRetries {
		return nil
	}

	backoff := r.calculateBackoff(attempt)
	r.logger.DebugContext(ctx, "applying backoff before retry",
		"request_id", req.ID,
		"backoff", backoff,
	)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff):
		return nil
	}
}

func (r *RetryExecutor) executeAttempt(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	poolTier int,
	excludedEndpoints []string,
	sessionID string,
	resultCh <-chan *ResultMessage,
	specificEndpointID string,
) (*ResultMessage, string, error) {
	var endpointID string
	var err error

	if specificEndpointID != "" {
		endpointID = specificEndpointID
	} else {
		endpointID, err = r.poolManager.GetEndpointFromPool(ctx, rule, poolTier, excludedEndpoints)
		if err != nil {
			return nil, "", fmt.Errorf("failed to select endpoint from pool %d: %w", poolTier, err)
		}
	}

	_, err = r.publisher.Publish(ctx, req, rule, sessionID, endpointID, SharedResultSubject)
	if err != nil {
		return nil, endpointID, fmt.Errorf("failed to publish task: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, endpointID, ctx.Err()
		case result := <-resultCh:

			if result.EndpointID != "" && result.EndpointID != endpointID {
				r.logger.DebugContext(ctx, "ignoring stale result from previous attempt",
					"current_endpoint", endpointID,
					"result_endpoint", result.EndpointID,
				)
				ReleaseResultMessage(result)

				continue
			}

			result.EndpointID = endpointID

			return result, endpointID, nil
		}
	}
}

func (r *RetryExecutor) parseResult(body []byte) (*ResultMessage, error) {
	result := AcquireResultMessage()
	err := json.Unmarshal(body, result)
	if err != nil {
		ReleaseResultMessage(result)

		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

func (r *RetryExecutor) getPoolTiers(rule *domain.RoutingRule) []int {
	if len(rule.EndpointPools) == 0 {
		return nil
	}

	tiers := make([]int, 0, len(rule.EndpointPools))
	for _, pool := range rule.EndpointPools {
		tiers = append(tiers, pool.Tier)
	}

	for i := 0; i < len(tiers)-1; i++ {
		for j := i + 1; j < len(tiers); j++ {
			if tiers[i] > tiers[j] {
				tiers[i], tiers[j] = tiers[j], tiers[i]
			}
		}
	}

	return tiers
}

func (r *RetryExecutor) getMaxRetries(poolConfig *domain.EndpointPool, poolTier int) int {
	if poolConfig != nil && poolConfig.MaxRetries > 0 {
		return poolConfig.MaxRetries
	}

	if poolTier >= 4 {
		return DefaultLastExitRetries
	}

	return DefaultMaxRetries
}

func (r *RetryExecutor) calculateBackoff(attempt int) time.Duration {
	backoff := r.baseBackoff
	for i := 1; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * r.backoffFactor)
		if backoff > r.maxBackoff {
			backoff = r.maxBackoff

			break
		}
	}

	return backoff
}

func (r *RetryExecutor) getFailureMessage(result *ResultMessage) string {
	if result.Error != nil {
		return result.Error.Message
	}

	return fmt.Sprintf("HTTP %d", result.StatusCode)
}

var ErrNoEndpointsAvailable = errors.New("no endpoints available in any pool")

var ErrAllPoolsExhausted = errors.New("all endpoint pools exhausted")

func (r *RetryExecutor) handleResult(ctx context.Context, body []byte) error {
	res, err := r.parseResult(body)
	if err != nil {
		r.logger.Error("failed to parse result message", "error", err)

		return nil
	}

	requestID := res.RequestID
	if requestID == "" {
		r.logger.Warn("received result without request_id")
		ReleaseResultMessage(res)

		return nil
	}

	val, ok := r.responseChans.Load(requestID)
	if !ok {
		r.logger.Debug("received result for unknown or timed-out request", "request_id", requestID)
		ReleaseResultMessage(res)

		return nil
	}

	ch := val.(chan *ResultMessage)

	select {
	case ch <- res:
	default:
		r.logger.Warn("result channel full", "request_id", requestID)
		ReleaseResultMessage(res)
	}

	return nil
}
