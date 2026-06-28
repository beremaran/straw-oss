package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

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

type RelayHandlerOption func(*RelayHandler)

func WithAllowPrivateIPs() RelayHandlerOption {
	return func(h *RelayHandler) {
		h.allowPrivateIPs = true
	}
}

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

func (h *RelayHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var reqDTO dto.RelayRequest
	if err := helper.ReadJSON(r, &reqDTO); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "request body too large") {
			helper.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")

			return
		}
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	req, err := reqDTO.ToProtocolRequest()
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	if req.URL == "" {
		helper.WriteError(w, http.StatusBadRequest, "missing url")

		return
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
	if err := validator.ValidateTargetURL(req.URL, validationOpts...); err != nil {
		slog.WarnContext(ctx, "target url validation failed", "url", req.URL, "error", err)
		helper.WriteError(w, http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))

		return
	}

	var apiKey *domain.ApiKey
	if val := middleware.GetAPIKey(r); val != nil {
		if k, ok := val.(*domain.ApiKey); ok {
			apiKey = k
		}
	}

	parseResult, err := h.tagParser.ParseTags(r, apiKey)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid tags")

		return
	}

	for _, warn := range parseResult.Warnings {
		w.Header().Add("X-Relay-Warning", warn)
	}

	if apiKey != nil && len(apiKey.Scopes) > 0 {
		for _, tag := range parseResult.Tags {
			if !apiKey.HasScope(tag) {
				slog.WarnContext(ctx, "tag denied by api key scope",
					"key_id", apiKey.ID,
					"tag", tag.String(),
				)
				helper.WriteError(w, http.StatusForbidden, fmt.Sprintf("tag '%s' not allowed by api key scopes", tag.String()))

				return
			}
		}
	}

	tracer := otel.Tracer("router")
	_, span := tracer.Start(ctx, "router.resolve")
	rule := h.matcher.Match(parseResult.Tags)

	if rule != nil {
		span.SetAttributes(
			attribute.String("matched_rule.id", rule.ID),
			attribute.String("fingerprint", rule.FingerprintPreset),
			attribute.String("quota_key", rule.QuotaKey),
		)
		slog.InfoContext(ctx, "rule matched",
			"rule_id", rule.ID,
			"fingerprint", rule.FingerprintPreset,
		)
	}
	span.End()
	if rule == nil {
		helper.WriteError(w, http.StatusNotFound, "no matching routing rule found")

		return
	}

	req.Fingerprint = rule.FingerprintPreset

	limitPerSecond := rule.RateLimitPerSecond
	if apiKey != nil && apiKey.RateLimitOverride != nil {
		limitPerSecond = *apiKey.RateLimitOverride
	}

	if limitPerSecond > 0 || rule.RateLimitPerMinute > 0 {
		quotaKey := rule.QuotaKey
		if quotaKey == "" {
			quotaKey = rule.ID
		}

		allowed, result, err := h.rateLimiter.Allow(ctx, quotaKey, limitPerSecond, rule.RateLimitPerMinute)
		if err != nil {
			slog.ErrorContext(ctx, "rate limit check failed", "error", err)
			helper.WriteError(w, http.StatusInternalServerError, "rate limit check failed")

			return
		}

		if result.Limit > 0 {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%.0f", result.Reset.Seconds()))
		}

		if !allowed {
			slog.WarnContext(ctx, "rate limit exceeded",
				"rule_id", rule.ID,
				"quota_key", quotaKey,
				"retry_after", result.Reset,
			)
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", result.Reset.Seconds()))
			helper.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")

			return
		}
	}

	filterReq := filter.NewFilterRequest(
		req.URL,
		req.Headers.Get("Host"),
		req.Headers.Get("Accept"),
		req.Method,
	)

	shouldBlock, err := h.filter.ShouldBlock(ctx, filterReq, rule.RequestFilters)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "filter check failed")

		return
	}

	if shouldBlock.Blocked {
		helper.WriteError(w, http.StatusForbidden, fmt.Sprintf("request blocked: %s", shouldBlock.Reason))

		return
	}

	sessionID := req.SessionID
	var preferredEndpointID string
	var currentSession *domain.Session

	if existingSession := middleware.GetSessionFromContext(ctx); existingSession != nil {
		currentSession = existingSession
		if sessionID == "" || sessionID == existingSession.ID {
			preferredEndpointID = existingSession.EndpointID
			req.SessionID = existingSession.ID
			sessionID = existingSession.ID
		}
	}

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

	if currentSession != nil && result.Success && result.Response != nil {
		if result.Response.EndpointID != "" && result.Response.EndpointID != currentSession.EndpointID {
			slog.InfoContext(ctx, "migrating session to new endpoint",
				"session_id", currentSession.ID,
				"old_endpoint", currentSession.EndpointID,
				"new_endpoint", result.Response.EndpointID,
			)

			updatedSession, err := h.sessionService.MigrateSession(ctx, currentSession.ID, result.Response.EndpointID)
			if err != nil {
				if errors.Is(err, domain.ErrSessionMigrationLimit) {
					slog.WarnContext(ctx, "session migration limit reached", "session_id", currentSession.ID)
				} else {
					slog.ErrorContext(ctx, "failed to migrate session", "error", err)
				}
			} else {
				w.Header().Set(middleware.HeaderSessionMigrated, "true")
				w.Header().Set(middleware.HeaderSessionPreviousEndpoint, currentSession.EndpointID)
				w.Header().Set(middleware.HeaderSessionMigrationCount, fmt.Sprintf("%d", updatedSession.MigrationCount))
			}
		}
	} else if currentSession == nil && result.Success && result.Response != nil && result.Response.EndpointID != "" {
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

	if !result.Success {
		if result.Response != nil {
			meta := &orchestrator.RelayMetadata{
				Retries:       result.TotalRetries,
				Pool:          fmt.Sprintf("%d", result.FinalPool),
				Timing:        result.Response.Timing,
				EndpointID:    result.Response.EndpointID,
				SessionID:     result.Response.SessionID,
				AttemptErrors: result.AttemptErrors,
			}
			_ = h.responseBuilder.WriteResponse(w, result.Response, meta)

			return
		}

		msg := "request failed"
		if len(result.AttemptErrors) > 0 {
			msg = result.AttemptErrors[len(result.AttemptErrors)-1].Message
		}
		helper.WriteError(w, http.StatusBadGateway, msg)

		return
	}

	res := result.Response

	if res.BodyCompressed && len(res.CompressedBody) > 0 {
		decompressed, err := protocol.Decompress(res.CompressedBody)
		if err == nil {
			res.CompressedBody = decompressed
			res.BodyCompressed = false
		} else {
			slog.WarnContext(ctx, "failed to decompress response body", "error", err)
			helper.WriteError(w, http.StatusBadGateway, "failed to decompress response")

			return
		}
	}

	meta := &orchestrator.RelayMetadata{
		Retries:    result.TotalRetries,
		Pool:       fmt.Sprintf("%d", result.FinalPool),
		Timing:     res.Timing,
		EndpointID: res.EndpointID,
		SessionID:  res.SessionID,
	}

	_ = h.responseBuilder.WriteResponse(w, res, meta)
}
