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
			var apiKey *domain.ApiKey
			if key := GetAPIKey(r); key != nil {
				if k, ok := key.(*domain.ApiKey); ok {
					apiKey = k
				}
			}

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

			limitPerMinute := rule.RateLimitPerMinute
			limitPerSecond := rule.RateLimitPerSecond

			if apiKey != nil && apiKey.RateLimitOverride != nil && *apiKey.RateLimitOverride > 0 {
				limitPerMinute = *apiKey.RateLimitOverride
			}

			if limitPerMinute <= 0 && limitPerSecond <= 0 {
				next.ServeHTTP(w, r.WithContext(ctx))

				return
			}

			apiKeyID := "anon"
			if apiKey != nil {
				apiKeyID = apiKey.ID
			}
			quotaKey := ratelimit.GenerateQuotaKey(rule, apiKeyID)

			allowed, res, err := limiter.Allow(r.Context(), quotaKey, limitPerSecond, limitPerMinute)
			if err != nil {
				helper.WriteError(w, http.StatusInternalServerError, "internal rate limit error")

				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(res.Reset).Unix(), 10))

			if !allowed {
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

				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetRoutingRule(r *http.Request) *domain.RoutingRule {
	if rule, ok := r.Context().Value(ContextRoutingRuleKey{Value: "routing_rule"}).(*domain.RoutingRule); ok {
		return rule
	}

	return nil
}
