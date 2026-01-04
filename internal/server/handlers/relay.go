package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/server/dto"
	"github.com/kwilabs/straw-proxy-server/internal/server/middleware"
	"github.com/kwilabs/straw-proxy-server/internal/service/filter"
	"github.com/kwilabs/straw-proxy-server/internal/service/orchestrator"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
	"github.com/kwilabs/straw-proxy-server/pkg/validator"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// RelayHandler handles incoming proxy requests.
type RelayHandler struct {
	matcher         *router.Matcher
	filter          *filter.Service
	executor        *orchestrator.RetryExecutor
	tagParser       *router.TagParser
	responseBuilder *orchestrator.ResponseBuilder
	rateLimiter     *ratelimit.RateLimiter
	sessionService  *session.Service
	allowPrivateIPs bool // Allow localhost/private IPs (for testing only)
}

// RelayHandlerOption is a functional option for RelayHandler.
type RelayHandlerOption func(*RelayHandler)

// WithAllowPrivateIPs allows URLs that resolve to private IPs.
// WARNING: Only use for testing. This disables SSRF protection.
func WithAllowPrivateIPs() RelayHandlerOption {
	return func(h *RelayHandler) {
		h.allowPrivateIPs = true
	}
}

// NewRelayHandler creates a new RelayHandler.
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

// Handle processes an incoming proxy request.
//
//	@Summary		Relay HTTP Request
//	@Description	Proxies an HTTP request through a matched endpoint based on routing rules.
//	@Description	The request is validated, filtered, rate-limited, and executed through the endpoint pool.
//	@Tags			relay
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.RelayRequest	true	"HTTP request to proxy"
//	@Success		200		{object}	dto.RelayResponse	"Successful proxy response"
//	@Failure		400		{object}	echo.HTTPError		"Invalid request body or URL"
//	@Failure		401		{object}	echo.HTTPError		"Authentication required"
//	@Failure		403		{object}	echo.HTTPError		"Request blocked by filter or scope"
//	@Failure		404		{object}	echo.HTTPError		"No matching routing rule found"
//	@Failure		429		{object}	echo.HTTPError		"Rate limit exceeded"
//	@Failure		502		{object}	echo.HTTPError		"Upstream execution failed"
//	@Security		ApiKeyAuth
//	@Router			/v1/request [post]
//	@Router			/v2/request [post]
func (h *RelayHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	// 1. Parse incoming request body using DTO
	var reqDTO dto.RelayRequest
	if err := c.Bind(&reqDTO); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body").SetInternal(err)
	}

	// Convert DTO to protocol.Request
	req, err := reqDTO.ToProtocolRequest()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Validate basic fields
	if req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing url")
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	// Generate a request ID if not provided
	if req.ID == "" {
		req.ID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}

	// Validate URL (SSRF Protection)
	validationOpts := []validator.ValidationOption{}
	if h.allowPrivateIPs {
		validationOpts = append(validationOpts, validator.WithAllowPrivateIPs())
	}
	if err := validator.ValidateTargetURL(req.URL, validationOpts...); err != nil {
		slog.WarnContext(ctx, "target url validation failed", "url", req.URL, "error", err)
		return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("invalid target url: %v", err))
	}

	// Try to get API key from context
	var apiKey *domain.ApiKey
	if val := middleware.GetAPIKey(c); val != nil {
		if k, ok := val.(*domain.ApiKey); ok {
			apiKey = k
		}
	}

	// 2. Parse tags
	parseResult, err := h.tagParser.ParseTags(c.Request(), apiKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid tags").SetInternal(err)
	}

	// Add any warnings to response headers
	for _, w := range parseResult.Warnings {
		c.Response().Header().Add("X-Relay-Warning", w)
	}

	// 2a. Validate Scopes
	// If the API key has scopes defined, we must ensure that all parsed tags are allowed by the scopes.
	if apiKey != nil && len(apiKey.Scopes) > 0 {
		for _, tag := range parseResult.Tags {
			if !apiKey.HasScope(tag) {
				slog.WarnContext(ctx, "tag denied by api key scope",
					"key_id", apiKey.ID,
					"tag", tag.String(),
				)
				return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("tag '%s' not allowed by api key scopes", tag.String()))
			}
		}
	}

	// 3. Find matching rule
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
		return echo.NewHTTPError(http.StatusNotFound, "no matching routing rule found")
	}

	// Apply fingerprint from rule to request
	req.Fingerprint = rule.FingerprintPreset

	// 3a. Rate Limit Check
	limitPerSecond := rule.RateLimitPerSecond
	// Apply override if present
	if apiKey != nil && apiKey.RateLimitOverride != nil {
		limitPerSecond = *apiKey.RateLimitOverride
	}

	if limitPerSecond > 0 || rule.RateLimitPerMinute > 0 {
		quotaKey := rule.QuotaKey
		if quotaKey == "" {
			// Default to rule ID if no quota key defined (global limit for rule)
			quotaKey = rule.ID
		}
		// Append client ID if available to make it per-client?
		// For now, respect rule config. If rule wants per-client, it should use variable (not implemented yet).
		// But logically, if we want per-client rate limit, we should append clientID.
		// However, requirements say "Implement Rate Limiting".
		// Let's stick to simple rule-based key for now.

		allowed, result, err := h.rateLimiter.Allow(ctx, quotaKey, limitPerSecond, rule.RateLimitPerMinute)
		if err != nil {
			slog.ErrorContext(ctx, "rate limit check failed", "error", err)
			// Fail open or closed? Closed for security.
			return echo.NewHTTPError(http.StatusInternalServerError, "rate limit check failed")
		}

		// Set Rate Limit Headers
		if result.Limit > 0 {
			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			c.Response().Header().Set("X-RateLimit-Reset", fmt.Sprintf("%.0f", result.Reset.Seconds()))
		}

		if !allowed {
			slog.WarnContext(ctx, "rate limit exceeded",
				"rule_id", rule.ID,
				"quota_key", quotaKey,
				"retry_after", result.Reset,
			)
			c.Response().Header().Set("Retry-After", fmt.Sprintf("%.0f", result.Reset.Seconds()))
			return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
		}
	}

	// 4. Check Filter (Blocklist) using the rule's filter policy
	filterReq := filter.NewFilterRequest(
		req.URL,
		req.Headers.Get("Host"),
		req.Headers.Get("Accept"),
		req.Method,
	)

	shouldBlock, err := h.filter.ShouldBlock(ctx, filterReq, rule.RequestFilters)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "filter check failed").SetInternal(err)
	}

	if shouldBlock.Blocked {
		return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("request blocked: %s", shouldBlock.Reason))
	}

	// 5. Execute Request
	// Retrieve SessionID from header or request body (if sticky sessions used)
	sessionID := req.SessionID
	var preferredEndpointID string
	var currentSession *domain.Session

	// Check if session exists in context (populated by middleware)
	if existingSession := middleware.GetSessionFromContext(ctx); existingSession != nil {
		currentSession = existingSession
		// If session ID matches request (it should), use its endpoint
		if sessionID == "" || sessionID == existingSession.ID {
			preferredEndpointID = existingSession.EndpointID
			// Ensure sessionID is set on request for logging/tracing
			req.SessionID = existingSession.ID
			sessionID = existingSession.ID
		}
	}

	result, err := h.executor.Execute(ctx, req, rule, sessionID, preferredEndpointID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "execution failed").SetInternal(err)
	}

	// 5a. Handle Session Migration
	// If we had a preferred endpoint (sticky session) but the result used a different endpoint,
	// and it was successful (or at least valid response), we should migrate the session.
	if currentSession != nil && result.Success && result.Response != nil {
		if result.Response.EndpointID != "" && result.Response.EndpointID != currentSession.EndpointID {
			slog.InfoContext(ctx, "migrating session to new endpoint",
				"session_id", currentSession.ID,
				"old_endpoint", currentSession.EndpointID,
				"new_endpoint", result.Response.EndpointID,
			)

			// Perform migration
			updatedSession, err := h.sessionService.MigrateSession(ctx, currentSession.ID, result.Response.EndpointID)
			if err != nil {
				if errors.Is(err, domain.ErrSessionMigrationLimit) {
					slog.WarnContext(ctx, "session migration limit reached", "session_id", currentSession.ID)
					// We don't fail the request, but we might want to notify client
				} else {
					slog.ErrorContext(ctx, "failed to migrate session", "error", err)
				}
			} else {
				// Migration successful
				c.Response().Header().Set(middleware.HeaderSessionMigrated, "true")
				c.Response().Header().Set(middleware.HeaderSessionPreviousEndpoint, currentSession.EndpointID)
				c.Response().Header().Set(middleware.HeaderSessionMigrationCount, fmt.Sprintf("%d", updatedSession.MigrationCount))
			}
		}
	} else if currentSession == nil && result.Success && result.Response != nil && result.Response.EndpointID != "" {
		// 5b. Create New Session if one doesn't exist
		// Note: We always create a session for successful requests if one wasn't provided.
		// This enables sticky sessions for subsequent requests.
		tagStrings := make([]string, len(parseResult.Tags))
		for i, t := range parseResult.Tags {
			tagStrings[i] = t.String()
		}
		newSession, err := h.sessionService.CreateSession(ctx, result.Response.EndpointID, rule.ID, tagStrings)
		if err != nil {
			slog.ErrorContext(ctx, "failed to create session", "error", err)
		} else {
			// Set session ID header
			c.Response().Header().Set(middleware.HeaderSessionID, newSession.ID)

			// Also update the metadata for the response builder if needed,
			// though ResponseBuilder takes sessionID from result.Response.SessionID usually?
			// The result.Response came from endpoint, so it has no session ID (unless endpoint generated it, which it doesn't).
			// We should probably inject it into meta?
			// But headers are already set on 'c'.
		}
	}

	// 6. Return Response
	if !result.Success {
		// Even if Success is false, we might have a response (e.g. valid HTTP 4xx/5xx from target)
		if result.Response != nil {
			meta := &orchestrator.RelayMetadata{
				Retries:       result.TotalRetries,
				Pool:          fmt.Sprintf("%d", result.FinalPool),
				Timing:        result.Response.Timing,
				EndpointID:    result.Response.EndpointID,
				SessionID:     result.Response.SessionID,
				AttemptErrors: result.AttemptErrors,
			}
			return h.responseBuilder.WriteResponse(c, result.Response, meta)
		}

		// If no response at all (e.g. network error)
		msg := "request failed"
		if len(result.AttemptErrors) > 0 {
			msg = result.AttemptErrors[len(result.AttemptErrors)-1].Message
		}
		// Return 502 Bad Gateway
		return echo.NewHTTPError(http.StatusBadGateway, msg)
	}

	// Success
	res := result.Response

	// Check for compression (RetryExecutor currently doesn't decompress automatically)
	if res.BodyCompressed && len(res.CompressedBody) > 0 {
		decompressed, err := protocol.Decompress(res.CompressedBody)
		if err == nil {
			res.CompressedBody = decompressed
			res.BodyCompressed = false
		} else {
			// Log error but proceed? using compressed body might result in garbage for client
			// better to error out or try to send as is if configured?
			// but we are writing Blob(..., compressedBody).
			// If client expects uncompressed, we must decompress.
			// If client expects uncompressed, we must decompress.
			slog.WarnContext(ctx, "failed to decompress response body", "error", err)
			return echo.NewHTTPError(http.StatusBadGateway, "failed to decompress response")
		}
	}

	meta := &orchestrator.RelayMetadata{
		Retries:    result.TotalRetries,
		Pool:       fmt.Sprintf("%d", result.FinalPool),
		Timing:     res.Timing,
		EndpointID: res.EndpointID,
		SessionID:  res.SessionID,
		// No errors to report on success usually
	}

	return h.responseBuilder.WriteResponse(c, res, meta)
}
