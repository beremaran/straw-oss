// Package handlers provides HTTP request handlers for the relay server.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/server/middleware"
	"github.com/beremaran/straw/internal/service/filter"
	"github.com/beremaran/straw/internal/service/orchestrator"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/internal/service/session"
	"github.com/beremaran/straw/pkg/protocol"
	"github.com/beremaran/straw/pkg/validator"
)

// RelayHandler processes incoming relay HTTP requests, routing them to appropriate endpoints
// based on tag matching, filtering, and rate limiting rules.
type RelayHandler struct {
	matcher         *router.Matcher
	filter          *filter.Service
	executor        *orchestrator.RetryExecutor
	tagParser       *router.TagParser
	responseBuilder *orchestrator.ResponseBuilder
	rateLimiter     *ratelimit.RateLimiter
	sessionService  *session.Service
	allowPrivateIPs bool
}

// RelayHandlerOption configures a RelayHandler instance.
type RelayHandlerOption func(*RelayHandler)

// WithAllowPrivateIPs permits relay requests to target private IP addresses.
func WithAllowPrivateIPs() RelayHandlerOption {
	return func(h *RelayHandler) {
		h.allowPrivateIPs = true
	}
}

// NewRelayHandler creates a new RelayHandler with the provided dependencies and options.
func NewRelayHandler(
	matcher *router.Matcher,
	filter *filter.Service,
	executor *orchestrator.RetryExecutor,
	rateLimiter *ratelimit.RateLimiter,
	sessionService *session.Service,
	opts ...RelayHandlerOption,
) *RelayHandler {
	h := &RelayHandler{
		matcher:         matcher,
		filter:          filter,
		executor:        executor,
		rateLimiter:     rateLimiter,
		sessionService:  sessionService,
		tagParser:       router.NewTagParser(),
		responseBuilder: orchestrator.NewResponseBuilder(),
	}
	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Handle processes an incoming relay HTTP request through validation, tag parsing, rule matching,
// rate limiting, filtering, and execution before writing the response.
func (h *RelayHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, ok := h.readRelayRequest(w, r)
	if !ok {
		return
	}

	if !h.prepareRelayRequest(ctx, w, req) {
		return
	}

	apiKey := relayAPIKey(r)

	parseResult, ok := h.parseRelayTags(ctx, w, r, apiKey)
	if !ok {
		return
	}

	rule, ok := h.matchRelayRule(ctx, w, parseResult)
	if !ok {
		return
	}

	req.Fingerprint = rule.FingerprintPreset
	if !h.applyRelayRateLimit(ctx, w, rule, apiKey) {
		return
	}

	if !h.allowByFilters(ctx, w, req, rule) {
		return
	}

	sessionID, preferredEndpointID, currentSession := sessionPreference(ctx, req)

	result, err := h.executor.Execute(ctx, req, rule, sessionID, preferredEndpointID)
	if err != nil {
		helper.WriteError(w, http.StatusBadGateway, "execution failed")

		return
	}

	defer func() {
		if result.Response != nil {
			orchestrator.ReleaseResultMessage(result.Response)
		}
	}()

	h.manageSession(w, r, currentSession, result, parseResult, rule)
	h.writeRelayResult(ctx, w, result)
}

func (h *RelayHandler) readRelayRequest(w http.ResponseWriter, r *http.Request) (*protocol.Request, bool) {
	var reqDTO dto.RelayRequest

	err := helper.ReadJSON(r, &reqDTO)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "request body too large") {
			helper.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")

			return nil, false
		}

		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return nil, false
	}

	req, err := reqDTO.ToProtocolRequest()
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return nil, false
	}

	return req, true
}

func (h *RelayHandler) prepareRelayRequest(ctx context.Context, w http.ResponseWriter, req *protocol.Request) bool {
	if req.URL == "" {
		helper.WriteError(w, http.StatusBadRequest, "missing url")

		return false
	}

	if req.Method == "" {
		req.Method = http.MethodGet
	}

	if req.ID == "" {
		req.ID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}

	validationOpts := []validator.ValidationOption{}
	if h.allowPrivateIPs {
		validationOpts = append(validationOpts, validator.WithAllowPrivateIPs())
	}

	err := validator.ValidateTargetURL(ctx, req.URL, validationOpts...)
	if err != nil {
		slog.WarnContext(ctx, "target url validation failed", "url", req.URL, "error", err)
		helper.WriteError(w, http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))

		return false
	}

	return true
}

func relayAPIKey(r *http.Request) *domain.APIKey {
	val := middleware.GetAPIKey(r)
	if val == nil {
		return nil
	}

	apiKey, _ := val.(*domain.APIKey)

	return apiKey
}

func (h *RelayHandler) parseRelayTags(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	apiKey *domain.APIKey,
) (*router.ParseResult, bool) {
	parseResult, err := h.tagParser.ParseTags(r, apiKey)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid tags")

		return nil, false
	}

	for _, warn := range parseResult.Warnings {
		w.Header().Add("X-Relay-Warning", warn)
	}

	if !apiKeyAllowsTags(ctx, w, apiKey, parseResult.Tags) {
		return nil, false
	}

	return parseResult, true
}

func apiKeyAllowsTags(ctx context.Context, w http.ResponseWriter, apiKey *domain.APIKey, tags []domain.Tag) bool {
	if apiKey == nil || len(apiKey.Scopes) == 0 {
		return true
	}

	for _, tag := range tags {
		if !apiKey.HasScope(tag) {
			slog.WarnContext(ctx, "tag denied by api key scope",
				"key_id", apiKey.ID,
				"tag", tag.String(),
			)
			helper.WriteError(w, http.StatusForbidden, fmt.Sprintf("tag '%s' not allowed by api key scopes", tag.String()))

			return false
		}
	}

	return true
}

func (h *RelayHandler) matchRelayRule(
	ctx context.Context,
	w http.ResponseWriter,
	parseResult *router.ParseResult,
) (*domain.RoutingRule, bool) {
	tracer := otel.Tracer("router")

	_, span := tracer.Start(ctx, "router.resolve")
	defer span.End()

	rule := h.matcher.Match(parseResult.Tags)
	if rule == nil {
		helper.WriteError(w, http.StatusNotFound, "no matching routing rule found")

		return nil, false
	}

	span.SetAttributes(
		attribute.String("matched_rule.id", rule.ID),
		attribute.String("fingerprint", rule.FingerprintPreset),
		attribute.String("quota_key", rule.QuotaKey),
	)
	slog.InfoContext(ctx, "rule matched",
		"rule_id", rule.ID,
		"fingerprint", rule.FingerprintPreset,
	)

	return rule, true
}

func (h *RelayHandler) applyRelayRateLimit(
	ctx context.Context,
	w http.ResponseWriter,
	rule *domain.RoutingRule,
	apiKey *domain.APIKey,
) bool {
	limitPerSecond := relayLimitPerSecond(rule, apiKey)
	if limitPerSecond <= 0 && rule.RateLimitPerMinute <= 0 {
		return true
	}

	quotaKey := relayQuotaKey(rule)

	allowed, result, err := h.rateLimiter.Allow(ctx, quotaKey, limitPerSecond, rule.RateLimitPerMinute)
	if err != nil {
		slog.ErrorContext(ctx, "rate limit check failed", "error", err)
		helper.WriteError(w, http.StatusInternalServerError, "rate limit check failed")

		return false
	}

	writeRelayRateLimitHeaders(w, result)

	if !allowed {
		writeRelayRateLimitExceeded(ctx, w, rule, quotaKey, result)

		return false
	}

	return true
}

func relayLimitPerSecond(rule *domain.RoutingRule, apiKey *domain.APIKey) int {
	if apiKey != nil && apiKey.RateLimitOverride != nil {
		return *apiKey.RateLimitOverride
	}

	return rule.RateLimitPerSecond
}

func relayQuotaKey(rule *domain.RoutingRule) string {
	if rule.QuotaKey != "" {
		return rule.QuotaKey
	}

	return rule.ID
}

func writeRelayRateLimitHeaders(w http.ResponseWriter, result ratelimit.Result) {
	if result.Limit <= 0 {
		return
	}

	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%.0f", result.Reset.Seconds()))
}

func writeRelayRateLimitExceeded(
	ctx context.Context,
	w http.ResponseWriter,
	rule *domain.RoutingRule,
	quotaKey string,
	result ratelimit.Result,
) {
	slog.WarnContext(ctx, "rate limit exceeded",
		"rule_id", rule.ID,
		"quota_key", quotaKey,
		"retry_after", result.Reset,
	)
	w.Header().Set("Retry-After", fmt.Sprintf("%.0f", result.Reset.Seconds()))
	helper.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
}

func (h *RelayHandler) allowByFilters(
	ctx context.Context,
	w http.ResponseWriter,
	req *protocol.Request,
	rule *domain.RoutingRule,
) bool {
	filterReq := filter.NewFilterRequest(
		req.URL,
		req.Headers.Get("Host"),
		req.Headers.Get("Accept"),
		req.Method,
	)

	shouldBlock, err := h.filter.ShouldBlock(ctx, filterReq, rule.RequestFilters)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "filter check failed")

		return false
	}

	if shouldBlock.Blocked {
		helper.WriteError(w, http.StatusForbidden, "request blocked: "+shouldBlock.Reason)

		return false
	}

	return true
}

func sessionPreference(ctx context.Context, req *protocol.Request) (string, string, *domain.Session) {
	sessionID := req.SessionID

	existingSession := middleware.GetSessionFromContext(ctx)
	if existingSession == nil {
		return sessionID, "", nil
	}

	if sessionID == "" || sessionID == existingSession.ID {
		req.SessionID = existingSession.ID

		return existingSession.ID, existingSession.EndpointID, existingSession
	}

	return sessionID, "", existingSession
}

func (h *RelayHandler) writeRelayResult(
	ctx context.Context,
	w http.ResponseWriter,
	result *orchestrator.RetryResult,
) {
	if !result.Success {
		h.writeFailedRelayResult(w, result)

		return
	}

	res := result.Response
	if !decompressRelayResponse(ctx, w, res) {
		return
	}

	meta := relayMetadata(result, res)
	_ = h.responseBuilder.WriteResponse(w, res, meta)
}

func (h *RelayHandler) writeFailedRelayResult(w http.ResponseWriter, result *orchestrator.RetryResult) {
	if result.Response != nil {
		meta := relayMetadata(result, result.Response)
		meta.AttemptErrors = result.AttemptErrors
		_ = h.responseBuilder.WriteResponse(w, result.Response, meta)

		return
	}

	msg := "request failed"
	if len(result.AttemptErrors) > 0 {
		msg = result.AttemptErrors[len(result.AttemptErrors)-1].Message
	}

	helper.WriteError(w, http.StatusBadGateway, msg)
}

func decompressRelayResponse(ctx context.Context, w http.ResponseWriter, res *orchestrator.ResultMessage) bool {
	if !res.BodyCompressed || len(res.CompressedBody) == 0 {
		return true
	}

	decompressed, err := protocol.Decompress(res.CompressedBody)
	if err != nil {
		slog.WarnContext(ctx, "failed to decompress response body", "error", err)
		helper.WriteError(w, http.StatusBadGateway, "failed to decompress response")

		return false
	}

	res.CompressedBody = decompressed
	res.BodyCompressed = false

	return true
}

func relayMetadata(result *orchestrator.RetryResult, res *orchestrator.ResultMessage) *orchestrator.RelayMetadata {
	return &orchestrator.RelayMetadata{
		Retries:    result.TotalRetries,
		Pool:       strconv.Itoa(result.FinalPool),
		Timing:     res.Timing,
		EndpointID: res.EndpointID,
		SessionID:  res.SessionID,
	}
}

func (h *RelayHandler) manageSession(
	w http.ResponseWriter,
	r *http.Request,
	currentSession *domain.Session,
	result *orchestrator.RetryResult,
	parseResult *router.ParseResult,
	rule *domain.RoutingRule,
) {
	ctx := r.Context()

	if !result.Success || result.Response == nil {
		return
	}

	if currentSession != nil {
		h.migrateSession(ctx, w, currentSession, result.Response.EndpointID)

		return
	}

	if result.Response.EndpointID != "" {
		tagStrings := make([]string, len(parseResult.Tags))
		for i, t := range parseResult.Tags {
			tagStrings[i] = t.String()
		}

		newSession, err := h.sessionService.CreateSession(ctx, result.Response.EndpointID, rule.ID, tagStrings)
		if err != nil {
			slog.ErrorContext(ctx, "failed to create session", "error", err)
		} else {
			w.Header().Set(middleware.HeaderSessionID, newSession.ID)
		}
	}
}

func (h *RelayHandler) migrateSession(ctx context.Context, w http.ResponseWriter, currentSession *domain.Session, newEndpointID string) {
	if newEndpointID == "" || newEndpointID == currentSession.EndpointID {
		return
	}

	slog.InfoContext(ctx, "migrating session to new endpoint",
		"session_id", currentSession.ID,
		"old_endpoint", currentSession.EndpointID,
		"new_endpoint", newEndpointID,
	)

	updatedSession, err := h.sessionService.MigrateSession(ctx, currentSession.ID, newEndpointID)
	if err != nil {
		if errors.Is(err, domain.ErrSessionMigrationLimit) {
			slog.WarnContext(ctx, "session migration limit reached", "session_id", currentSession.ID)
		} else {
			slog.ErrorContext(ctx, "failed to migrate session", "error", err)
		}

		return
	}

	w.Header().Set(middleware.HeaderSessionMigrated, "true")
	w.Header().Set(middleware.HeaderSessionPreviousEndpoint, currentSession.EndpointID)
	w.Header().Set(middleware.HeaderSessionMigrationCount, strconv.Itoa(updatedSession.MigrationCount))
}
