package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/server/metrics"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/labstack/echo/v4"
)

const (
	ContextKeyRoutingRule = "routing_rule"
	ContextKeyTags        = "tags"
)

// RateLimitMiddleware creates a middleware that enforces rate limits.
func RateLimitMiddleware(limiter *ratelimit.RateLimiter, matcher *router.Matcher) echo.MiddlewareFunc {
	// Stateless parser
	tagParser := router.NewTagParser()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 1. Get API Key (from previous AuthMiddleware)
			var apiKey *domain.ApiKey
			if key := GetAPIKey(c); key != nil {
				if k, ok := key.(*domain.ApiKey); ok {
					apiKey = k
				}
			}

			// 2. Parse Tags
			// We pass the API key so scopes can be merged as tags
			parseResult, err := tagParser.ParseTags(c.Request(), apiKey)
			if err != nil {
				// If tag parsing fails, we probably can't route. Return 400?
				c.Logger().Warnf("Tag parsing failed: %v", err)
				return echo.NewHTTPError(http.StatusBadRequest, "invalid tags")
			}

			// Store tags in context for downstream use (e.g. analytics)
			c.Set(ContextKeyTags, parseResult.Tags)

			// 3. Match Routing Rule
			rule := matcher.Match(parseResult.Tags)
			if rule == nil {
				// Check for deprecation warnings even if no rule matched?
				// Usually, if no rule matches, we should error.
				return echo.NewHTTPError(http.StatusServiceUnavailable, "no matching routing rule found")
			}

			// Store rule in context for downstream use (Orchestrator)
			c.Set(ContextKeyRoutingRule, rule)

			// 4. Determine Limits
			// Default to rule limits
			limitPerMinute := rule.RateLimitPerMinute
			limitPerSecond := rule.RateLimitPerSecond

			// Apply API Key override if present (assuming it overrides per-minute/general capacity)
			// The design doc says "Per-API-key rate limit override" in Task 3.6 description
			// and in Schema 10.5.1 "rate_limit_override INT -- optional per-key rate limit".
			// It implies a global override for that key? Or overrides the rule's minute limit?
			// Let's assume it overrides the minute limit.
			if apiKey != nil && apiKey.RateLimitOverride != nil && *apiKey.RateLimitOverride > 0 {
				limitPerMinute = *apiKey.RateLimitOverride
			}

			// If no limits defined, proceed
			if limitPerMinute <= 0 && limitPerSecond <= 0 {
				return next(c)
			}

			// 5. Generate Quota Key
			// Format: quota:{rule_quota_key}:{api_key_id}
			// If no API key (e.g. public access? - not supported by AuthMiddleware), use IP?
			// But AuthMiddleware enforces a key.
			apiKeyID := "anon"
			if apiKey != nil {
				apiKeyID = apiKey.ID
			}
			quotaKey := ratelimit.GenerateQuotaKey(rule, apiKeyID)

			// 6. Check Rate Limit
			allowed, res, err := limiter.Allow(c.Request().Context(), quotaKey, limitPerSecond, limitPerMinute)
			if err != nil {
				// Fail open or closed?
				c.Logger().Errorf("Rate limit check failed: %v", err)
				// Design choice: Fail Open (allow) to avoid outage during Redis blip?
				// Or Fail Closed?
				// Given "High Performance" proxy, maybe fail closed if we can't ensure quota?
				// Let's return 500 for now.
				return echo.NewHTTPError(http.StatusInternalServerError, "internal rate limit error")
			}

			// 7. Set Headers
			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			c.Response().Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(res.Reset).Unix(), 10))

			if !allowed {
				if metrics.RateLimitExceeded != nil {
					metrics.RateLimitExceeded.WithLabelValues(quotaKey).Inc()
				}
				retryAfter := int(res.Reset.Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))

				// Standard error response
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error": map[string]interface{}{
						"code":                "RATE_LIMIT_EXCEEDED",
						"message":             fmt.Sprintf("Rate limit exceeded for quota key '%s'", quotaKey),
						"retryable":           true,
						"retry_after_seconds": retryAfter,
					},
				})
			}

			return next(c)
		}
	}
}

// GetRoutingRule retrieves the matched routing rule from the context.
func GetRoutingRule(c echo.Context) *domain.RoutingRule {
	if rule, ok := c.Get(ContextKeyRoutingRule).(*domain.RoutingRule); ok {
		return rule
	}
	return nil
}
