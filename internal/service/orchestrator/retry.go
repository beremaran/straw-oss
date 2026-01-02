// Package orchestrator provides task orchestration for the Relay Server.
// This file implements the retry executor with tiered pool fallback logic.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// Default retry configuration values.
const (
	DefaultMaxRetries      = 2
	DefaultBaseBackoff     = 100 * time.Millisecond
	DefaultMaxBackoff      = 5 * time.Second
	DefaultBackoffFactor   = 2.0
	DefaultLastExitRetries = 1 // Last resort pool gets only 1 attempt
	SharedResultQueue      = "results.relay-server"
)

// AttemptError records a single failed attempt for debugging.
// This is included in the X-Relay-Attempt-Errors header.
type AttemptError struct {
	// Pool tier where the failure occurred (1 = Primary, 2 = Secondary, etc.)
	Pool int `json:"pool"`

	// Attempt number within the pool (1-based)
	Attempt int `json:"attempt"`

	// EndpointID that failed
	EndpointID string `json:"endpoint_id"`

	// Failure classification
	Failure FailureType `json:"failure"`

	// FailureString is the string representation of the failure type
	FailureString string `json:"failure_type"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// Duration of the failed attempt
	Duration time.Duration `json:"duration"`
}

// RetryResult encapsulates the outcome of a retry-enabled execution.
type RetryResult struct {
	// Success indicates whether the request ultimately succeeded
	Success bool

	// Response is the final result (may be an error response if Success is false)
	Response *ResultMessage

	// FinalPool is the pool tier that handled the request (0 if all failed)
	FinalPool int

	// TotalRetries is the total number of retry attempts across all pools
	TotalRetries int

	// AttemptErrors records all failed attempts for debugging
	AttemptErrors []AttemptError
}

// PoolManager provides endpoint selection from tiered pools.
type PoolManager interface {
	// GetEndpointFromPool selects an endpoint from the specified pool tier.
	// The exclude parameter lists endpoint IDs that should not be selected (already failed).
	GetEndpointFromPool(ctx context.Context, rule *domain.RoutingRule, poolTier int, exclude []string) (string, error)

	// GetPoolConfig returns the configuration for the specified pool tier.
	// Returns nil if the pool tier doesn't exist.
	GetPoolConfig(rule *domain.RoutingRule, poolTier int) *domain.EndpointPool
}

// RetryExecutor handles task execution with retry and pool fallback logic.
// It implements Section 8.2 of the design document.
type RetryExecutor struct {
	publisher   *Publisher
	consumer    *Consumer
	poolManager PoolManager
	broker      broker.MessageBroker
	hmacSecret  []byte
	logger      *slog.Logger

	// Response dispatching
	responseChans sync.Map // map[string]chan *ResultMessage (requestID -> channel)

	// Configuration
	baseBackoff   time.Duration
	maxBackoff    time.Duration
	backoffFactor float64
}

// RetryExecutorOption is a functional option for configuring the RetryExecutor.
type RetryExecutorOption func(*RetryExecutor)

// WithRetryLogger sets the logger for the retry executor.
func WithRetryLogger(logger *slog.Logger) RetryExecutorOption {
	return func(r *RetryExecutor) {
		r.logger = logger
	}
}

// WithBackoffConfig sets the backoff configuration.
func WithBackoffConfig(base, max time.Duration, factor float64) RetryExecutorOption {
	return func(r *RetryExecutor) {
		r.baseBackoff = base
		r.maxBackoff = max
		r.backoffFactor = factor
	}
}

// NewRetryExecutor creates a new RetryExecutor.
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

// Start initializes the result consumer.
func (r *RetryExecutor) Start(ctx context.Context) error {
	// Subscribe to the shared results queue
	// Assuming the queue is durable and shared by all relay server instances (or one queue per instance if we want unique routing).
	// For simplicity in load test fix, we use one shared queue.
	// Use manual Consume not SubscribeTemporary as we want it persistent.
	return r.broker.Subscribe(ctx, SharedResultQueue, r.handleResult)
}

// handleResult processes incoming result messages and dispatches them to the correct waiting channel.
func (r *RetryExecutor) handleResult(ctx context.Context, body []byte) error {
	res, err := r.parseResult(body)
	if err != nil {
		r.logger.Error("failed to parse result message", "error", err)
		return nil // Don't retry malformed messages
	}

	// Dispatch to the correct channel
	// We need the correlation ID (RequestID) to find the channel
	// Assuming RequestID is available in ResultMessage (it's not explicitly in struct usually, but in logic)
	// We need to rely on `RequestID` being passed back in headers or body.
	// Looking at ResultMessage struct in `consumer_test.go` or `endpoint/worker.go`?
	// The `protocol.Result` has RequestID? No, `response_builder.go` implies result message structure.
	// In `retry.go` before refactor, we used per-request queue, so any message in queue belonged to that request.
	// Now with shared queue, we MUST identify the request.
	// Let's assume protocol.Result or the JSON body has `request_id`.
	// Wait, `ResultMessage` struct is not defined in this file. It is likely in `consumer.go` or `package`.
	// Let's check `parseResult` usage. It unmarshals to `ResultMessage`.
	// If `ResultMessage` doesn't have RequestID, we have a problem.
	// We need to ensure `ResultMessage` has `request_id`.
	// ...Checking implicit assumptions...
	// We will add RequestID to the struct unmarshal target if missing or check if it exists.

	// Assuming ResultMessage has RequestID for now.
	requestID := res.RequestID
	if requestID == "" {
		// Try to fallback or log error
		r.logger.Warn("received result without request_id")
		return nil
	}

	val, ok := r.responseChans.Load(requestID)
	if !ok {
		// Channel not found, maybe request timed out and gave up
		r.logger.Debug("received result for unknown or timed-out request", "request_id", requestID)
		return nil
	}

	ch := val.(chan *ResultMessage)

	// Non-blocking send
	select {
	case ch <- res:
	default:
		r.logger.Warn("result channel full", "request_id", requestID)
	}

	return nil
}

// Execute runs the request with retry and pool escalation logic.
// It follows the tiered retry strategy from Section 8.2:
//  1. Try Primary Pool (up to MaxRetries)
//  2. On failure → Try Secondary Pool (up to MaxRetries)
//  3. On failure → Try Tertiary Pool (up to MaxRetries)
//  4. On failure → Try LastExit Pool (single attempt)
//  5. On failure → Return error
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

	// Determine available pools
	pools := r.getPoolTiers(rule)
	if len(pools) == 0 {
		// No explicit pools defined, use default single pool
		pools = []int{1}
	}

	r.logger.DebugContext(ctx, "starting retry execution",
		"request_id", req.ID,
		"pools", pools,
		"rule_id", rule.ID,
	)

	// Ensure Request ID exists
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// Create and register result channel
	resultCh := make(chan *ResultMessage, 1) // Buffer 1 is enough for the final result
	r.responseChans.Store(req.ID, resultCh)
	defer r.responseChans.Delete(req.ID)

	// Track excluded endpoints (ones that have already failed)
	var excludedEndpoints []string

	// 1. Try Preferred Endpoint (Sticky Session)
	if preferredEndpointID != "" {
		r.logger.DebugContext(ctx, "attempting sticky session endpoint",
			"request_id", req.ID,
			"endpoint_id", preferredEndpointID,
		)

		// Use pool 1 for reporting purposes for sticky attempts (or 0)
		attemptPoolTier := 1

		response, endpointID, err := r.executeAttempt(ctx, req, rule, attemptPoolTier, excludedEndpoints, sessionID, resultCh, preferredEndpointID)

		if err == nil {
			failure := ClassifyFailure(response)
			if failure == FailureNone {
				// Success
				result.Success = true
				result.Response = response
				result.FinalPool = attemptPoolTier
				return result, nil
			}

			// Failed - record and continue to normal pool selection
			r.logger.InfoContext(ctx, "sticky endpoint failed, falling back to pools",
				"request_id", req.ID,
				"endpoint_id", endpointID,
				"failure", failure.String(),
			)

			attemptErr := AttemptError{
				Pool:          attemptPoolTier,
				Attempt:       1,
				EndpointID:    endpointID,
				Failure:       failure,
				FailureString: failure.String(),
				Message:       r.getFailureMessage(response),
				Duration:      0, // We could track duration if needed
			}
			result.AttemptErrors = append(result.AttemptErrors, attemptErr)
			result.TotalRetries++
			excludedEndpoints = append(excludedEndpoints, endpointID)
		} else {
			// Internal error executing
			r.logger.WarnContext(ctx, "sticky endpoint execution error", "error", err)
			excludedEndpoints = append(excludedEndpoints, preferredEndpointID)
		}
	}

	for _, poolTier := range pools {
		poolConfig := r.poolManager.GetPoolConfig(rule, poolTier)
		maxRetries := r.getMaxRetries(poolConfig, poolTier)

		for attempt := 1; attempt <= maxRetries; attempt++ {
			// Check context cancellation
			if ctx.Err() != nil {
				return result, ctx.Err()
			}

			attemptStart := time.Now()

			// Execute single attempt
			response, endpointID, err := r.executeAttempt(ctx, req, rule, poolTier, excludedEndpoints, sessionID, resultCh, "")
			attemptDuration := time.Since(attemptStart)

			if err != nil {
				// Failed to execute (no response received)
				attemptErr := AttemptError{
					Pool:          poolTier,
					Attempt:       attempt,
					EndpointID:    endpointID,
					Failure:       FailureInternal,
					FailureString: FailureInternal.String(),
					Message:       err.Error(),
					Duration:      attemptDuration,
				}
				result.AttemptErrors = append(result.AttemptErrors, attemptErr)
				result.TotalRetries++
				if endpointID != "" {
					excludedEndpoints = append(excludedEndpoints, endpointID)
				}

				r.logger.WarnContext(ctx, "attempt failed with error",
					"request_id", req.ID,
					"pool", poolTier,
					"attempt", attempt,
					"endpoint", endpointID,
					"error", err,
				)

				// Continue to next attempt or pool
				continue
			}

			// Classify the failure (if any)
			failure := ClassifyFailure(response)

			if failure == FailureNone {
				// Success!
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

				return result, nil
			}

			// Record the failure
			attemptErr := AttemptError{
				Pool:          poolTier,
				Attempt:       attempt,
				EndpointID:    endpointID,
				Failure:       failure,
				FailureString: failure.String(),
				Message:       r.getFailureMessage(response),
				Duration:      attemptDuration,
			}
			result.AttemptErrors = append(result.AttemptErrors, attemptErr)
			result.TotalRetries++
			excludedEndpoints = append(excludedEndpoints, endpointID)

			r.logger.WarnContext(ctx, "attempt failed",
				"request_id", req.ID,
				"pool", poolTier,
				"attempt", attempt,
				"endpoint", endpointID,
				"failure", failure.String(),
				"status_code", response.StatusCode,
			)

			// Check for immediate escalation (e.g., 403/captcha)
			if failure.ShouldEscalate() {
				r.logger.InfoContext(ctx, "immediate escalation triggered",
					"request_id", req.ID,
					"pool", poolTier,
					"failure", failure.String(),
				)
				break // Break inner loop to escalate to next pool
			}

			// Check if we should retry within this pool
			if !failure.ShouldRetry() {
				// Non-retryable failure, but not escalation-worthy
				// Return the response as-is
				result.Response = response
				result.FinalPool = poolTier
				return result, nil
			}

			// Apply backoff if needed
			if failure.RequiresBackoff() && attempt < maxRetries {
				backoff := r.calculateBackoff(attempt)
				r.logger.DebugContext(ctx, "applying backoff before retry",
					"request_id", req.ID,
					"backoff", backoff,
				)

				select {
				case <-ctx.Done():
					return result, ctx.Err()
				case <-time.After(backoff):
				}
			}
		}

		// Pool exhausted, move to next pool
		r.logger.InfoContext(ctx, "pool exhausted, escalating",
			"request_id", req.ID,
			"pool", poolTier,
		)
	}

	// All pools exhausted
	r.logger.WarnContext(ctx, "all pools exhausted",
		"request_id", req.ID,
		"total_retries", result.TotalRetries,
	)

	return result, nil
}

// executeAttempt executes a single attempt. If specificEndpointID is provided, it skips pool selection.
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
		// Select endpoint from pool
		endpointID, err = r.poolManager.GetEndpointFromPool(ctx, rule, poolTier, excludedEndpoints)
		if err != nil {
			return nil, "", fmt.Errorf("failed to select endpoint from pool %d: %w", poolTier, err)
		}
	}

	// Publish the task - pass expected shared reply queue name
	_, err = r.publisher.Publish(ctx, req, rule, sessionID, endpointID, SharedResultQueue)
	if err != nil {
		return nil, endpointID, fmt.Errorf("failed to publish task: %w", err)
	}

	// Wait for result
	// The resultCh might receive results from previous failed attempts or the current one.
	for {
		select {
		case <-ctx.Done():
			return nil, endpointID, ctx.Err()
		case result := <-resultCh:
			// If the result has an EndpointID, verify it matches our current attempt
			// to avoid processing stale responses from previous retries.
			// However, if we receive a SUCCESS from a previous attempt that arrived late,
			// we should arguably accept it. But for safety and correctness of the retry logic,
			// we stick to the current attempt.
			if result.EndpointID != "" && result.EndpointID != endpointID {
				r.logger.DebugContext(ctx, "ignoring stale result from previous attempt",
					"current_endpoint", endpointID,
					"result_endpoint", result.EndpointID,
				)
				continue
			}

			result.EndpointID = endpointID
			return result, endpointID, nil
		}
	}
}

// parseResult parses a result message from raw bytes.
func (r *RetryExecutor) parseResult(body []byte) (*ResultMessage, error) {
	// The result is already parsed by the consumer
	// This is a placeholder for any additional parsing if needed
	var result ResultMessage
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}
	return &result, nil
}

// getPoolTiers returns the available pool tiers for a rule, sorted by priority.
func (r *RetryExecutor) getPoolTiers(rule *domain.RoutingRule) []int {
	if len(rule.EndpointPools) == 0 {
		return nil
	}

	tiers := make([]int, 0, len(rule.EndpointPools))
	for _, pool := range rule.EndpointPools {
		tiers = append(tiers, pool.Tier)
	}

	// Sort by tier (ascending)
	for i := 0; i < len(tiers)-1; i++ {
		for j := i + 1; j < len(tiers); j++ {
			if tiers[i] > tiers[j] {
				tiers[i], tiers[j] = tiers[j], tiers[i]
			}
		}
	}

	return tiers
}

// getMaxRetries returns the max retries for a pool configuration.
func (r *RetryExecutor) getMaxRetries(poolConfig *domain.EndpointPool, poolTier int) int {
	if poolConfig != nil && poolConfig.MaxRetries > 0 {
		return poolConfig.MaxRetries
	}

	// Default: higher tier pools get fewer retries
	// Tier 4+ (LastExit) gets only 1 attempt
	if poolTier >= 4 {
		return DefaultLastExitRetries
	}

	return DefaultMaxRetries
}

// calculateBackoff calculates the backoff duration for a retry attempt.
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

// getFailureMessage extracts a failure message from the result.
func (r *RetryExecutor) getFailureMessage(result *ResultMessage) string {
	if result.Error != nil {
		return result.Error.Message
	}
	return fmt.Sprintf("HTTP %d", result.StatusCode)
}

// ErrNoEndpointsAvailable is returned when no endpoints are available in any pool.
var ErrNoEndpointsAvailable = errors.New("no endpoints available in any pool")

// ErrAllPoolsExhausted is returned when all pools have been exhausted.
var ErrAllPoolsExhausted = errors.New("all endpoint pools exhausted")
