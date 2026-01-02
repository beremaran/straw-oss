# Routing & Tags

Straw uses a flexible **Tag-Based Routing System**. This allows for granular control over how requests are processed based on their characteristics.

## The Tag System

A "Tag" is a simple `key:value` identifier attached to a request.

* **Examples**: `target:amazon`, `region:eu`, `type:search`, `capability:stealth`.

**Tag Sources**:
Tags are aggregated from multiple sources:

1. **Client Headers**: `X-Relay-Tags: target=amazon, type=search`
2. **Header Mapping**: `X-Straw-Retailer: amazon` maps to `target:amazon`
3. **JWT Claims**: Authenticated clients may have embedded tags.

## Routing Rules

Routing Rules are stored in the database and matched against the tags of an incoming request.

```go
type RoutingRule struct {
    ID               string
    Name             string
    
    // Match Criteria
    RequiredTags     []string // AND logic
    ExcludedTags     []string // NOT logic

    // Action / Config
    Priority             int
    RateLimitPerMinute   int
    AllowedEndpointTypes []string   // e.g., ["residential"]
    FingerprintPreset    string     // e.g., "chrome-130"
    QuotaKey             string     // e.g., "target:amazon"
}
```

### Rate Limiting

Rate limiting is applied based on the matched rule's `QuotaKey`. We use a **Dual-Bucket Sliding Window** algorithm to enforce both per-second and per-minute limits, ensuring burst control and long-term fairness.

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests allowed in window |
| `X-RateLimit-Remaining` | Requests remaining in current window |
| `X-RateLimit-Reset` | Unix timestamp when the window resets |

## Session Management

Sessions enable **sticky** routing, ensuring that a sequence of requests uses the same endpoint (and thus potentially the same IP/TLS session).

* **Creation**: Client sends a request without a session ID. Server creates one and enables stickiness.
* **Continuation**: Client sends `X-Session-ID`. Server attempts to route to the originally assigned endpoint.
* **Migration**: If the assigned endpoint is unhealthy, the server automatically migrates the session to a new endpoint and sets the `X-Session-Migrated: true` header.
