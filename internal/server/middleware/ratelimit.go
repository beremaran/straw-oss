package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/server/metrics"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
)

type ContextTagKey struct {
	Value string
}

type ContextRoutingRuleKey struct {
	Value string
}

func RateLimitMiddleware(limiter *ratelimit.RateLimiter, matcher *router.Matcher) func(http.Handler) http.Handler {
	tagParser := router.NewTagParser()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleRateLimitRequest(limiter, matcher, tagParser, next, w, r)
		})
	}
}

func handleRateLimitRequest(
	limiter *ratelimit.RateLimiter,
	matcher *router.Matcher,
	tagParser *router.TagParser,
	next http.Handler,
	w http.ResponseWriter,
	r *http.Request,
) {
	apiKey := apiKeyFromRequest(r)
	parseResult, err := tagParser.ParseTags(r, apiKey)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid tags")

		return
	}

	ctx := context.WithValue(r.Context(), ContextTagKey{Value: "tags"}, parseResult.Tags)
	rule := matcher.Match(parseResult.Tags)
	if rule == nil {
		helper.WriteError(w, http.StatusServiceUnavailable, "no matching routing rule found")

		return
	}
	ctx = context.WithValue(ctx, ContextRoutingRuleKey{Value: "routing_rule"}, rule)

	limitPerSecond, limitPerMinute := rateLimits(rule, apiKey)
	if limitPerMinute <= 0 && limitPerSecond <= 0 {
		next.ServeHTTP(w, r.WithContext(ctx))

		return
	}

	quotaKey := ratelimit.GenerateQuotaKey(rule, apiKeyID(apiKey))
	allowed, res, err := limiter.Allow(r.Context(), quotaKey, limitPerSecond, limitPerMinute)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "internal rate limit error")

		return
	}

	setRateLimitHeaders(w, res)
	if !allowed {
		writeRateLimitExceeded(w, quotaKey, res)

		return
	}

	next.ServeHTTP(w, r.WithContext(ctx))
}

func apiKeyFromRequest(r *http.Request) *domain.ApiKey {
	key := GetAPIKey(r)
	if key == nil {
		return nil
	}

	apiKey, _ := key.(*domain.ApiKey)

	return apiKey
}

func rateLimits(rule *domain.RoutingRule, apiKey *domain.ApiKey) (int, int) {
	limitPerSecond := rule.RateLimitPerSecond
	limitPerMinute := rule.RateLimitPerMinute
	if apiKey != nil && apiKey.RateLimitOverride != nil && *apiKey.RateLimitOverride > 0 {
		limitPerMinute = *apiKey.RateLimitOverride
	}

	return limitPerSecond, limitPerMinute
}

func apiKeyID(apiKey *domain.ApiKey) string {
	if apiKey == nil {
		return "anon"
	}

	return apiKey.ID
}

func setRateLimitHeaders(w http.ResponseWriter, res ratelimit.Result) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(res.Reset).Unix(), 10))
}

func writeRateLimitExceeded(w http.ResponseWriter, quotaKey string, res ratelimit.Result) {
	if metrics.RateLimitExceeded != nil {
		metrics.RateLimitExceeded.WithLabelValues(quotaKey).Inc()
	}
	retryAfter := int(res.Reset.Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))

	helper.WriteJSON(w, http.StatusTooManyRequests, map[string]interface{}{
		"error": map[string]interface{}{
			"code":                "RATE_LIMIT_EXCEEDED",
			"message":             fmt.Sprintf("Rate limit exceeded for quota key '%s'", quotaKey),
			"retryable":           true,
			"retry_after_seconds": retryAfter,
		},
	})
}

func GetRoutingRule(r *http.Request) *domain.RoutingRule {
	if rule, ok := r.Context().Value(ContextRoutingRuleKey{Value: "routing_rule"}).(*domain.RoutingRule); ok {
		return rule
	}

	return nil
}
