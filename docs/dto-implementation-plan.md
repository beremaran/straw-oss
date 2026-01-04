# DTO Layer Implementation Plan

This document outlines the introduction of Data Transfer Objects (DTOs) across all API handlers to separate the API layer from domain/protocol models.

## Problem Statement

Currently, the codebase uses domain models (`domain.RoutingRule`, `domain.ApiKey`, etc.) and protocol types (`protocol.Request`, `protocol.Response`) directly in API handlers:

| Handler | Model Used Directly | Issue |
|---------|---------------------|-------|
| `internal/server/handlers/relay.go` | `protocol.Request`, `protocol.Response` | Protocol types leaked to API |
| `internal/server/admin/handlers/routing_rule.go` | `domain.RoutingRule` | Domain model in request/response |
| `internal/server/admin/handlers/fingerprints.go` | `domain.FingerprintPreset` | Domain model in request/response |
| `internal/server/admin/handlers/api_key.go` | `domain.ApiKey` embedded in response | Leaks internal fields |

### Issues with Current Approach

1. **API contract tied to domain** - Changing domain models breaks API compatibility
2. **Leaking internal fields** - Fields like `TokenHash`, `Version`, internal timestamps exposed
3. **No input validation layer** - Validation scattered across handlers
4. **No field filtering** - Cannot control which fields are exposed per endpoint
5. **Swagger docs show internal types** - Generated API docs reference internal package paths

---

## Proposed Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   HTTP Layer    │    │   DTO Layer     │    │  Domain Layer   │
│   (handlers)    │───▸│   (dto pkg)     │───▸│  (domain pkg)   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
        │                      │                      │
   c.Bind(&dto)          dto.ToDomain()         Business Logic
   c.JSON(dto)           dto.FromDomain()       Repository
```

### Directory Structure

```
internal/server/
├── dto/                              # NEW: All DTOs and mappers
│   ├── relay.go                      # Relay API DTOs
│   ├── routing_rule.go               # Routing rule DTOs
│   ├── fingerprint.go                # Fingerprint DTOs
│   ├── api_key.go                    # API key DTOs
│   ├── usage.go                      # Usage DTOs (refactored)
│   ├── common.go                     # Shared types (pagination, errors)
│   └── mappers.go                    # Conversion functions
├── handlers/
│   └── relay.go                      # MODIFY: Use dto.RelayRequest
└── admin/handlers/
    ├── routing_rule.go               # MODIFY: Use dto.CreateRuleRequest
    ├── fingerprints.go               # MODIFY: Use dto.CreateFingerprintRequest
    ├── api_key.go                    # MODIFY: Use dto.CreateApiKeyRequest
    ├── usage.go                      # MODIFY: Use dto types
    └── types.go                      # DELETE (move to dto package)
```

---

## Proposed Changes

### Component 1: Common DTOs

#### [NEW] `internal/server/dto/common.go`

Shared types used across multiple handlers.

```go
package dto

// PaginatedResponse wraps paginated list responses
type PaginatedResponse[T any] struct {
    Data  []T `json:"data"`
    Total int `json:"total"`
    Page  int `json:"page"`
    Limit int `json:"limit"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
    Error   string `json:"error"`
    Code    string `json:"code,omitempty"`
    Details any    `json:"details,omitempty"`
}

// HeaderDTO represents an HTTP header for API transport
type HeaderDTO struct {
    Key   string `json:"key"`
    Value string `json:"value"`
}
```

---

### Component 2: Relay API DTOs

#### [NEW] `internal/server/dto/relay.go`

```go
package dto

import "time"

// RelayRequest is the API request for proxying HTTP requests
// @Description HTTP request to be proxied through the relay
type RelayRequest struct {
    // ID is an optional client-provided request ID for correlation
    ID string `json:"id,omitempty"`

    // Method is the HTTP method (defaults to GET)
    Method string `json:"method,omitempty"`

    // URL is the target URL to proxy (required)
    URL string `json:"url" validate:"required,url"`

    // Headers are the HTTP headers to send
    Headers map[string]string `json:"headers,omitempty"`

    // Body is the request body (base64 encoded for binary)
    Body []byte `json:"body,omitempty"`

    // Timeout is the request timeout (e.g., "30s")
    Timeout string `json:"timeout,omitempty"`

    // SessionID for sticky session affinity
    SessionID string `json:"session_id,omitempty"`

    // TraceID for distributed tracing
    TraceID string `json:"trace_id,omitempty"`
}

// RelayResponse is the API response from a proxied request
// @Description Response from a proxied HTTP request
type RelayResponse struct {
    // RequestID correlates to the original request
    RequestID string `json:"request_id"`

    // StatusCode is the HTTP status from the target
    StatusCode int `json:"status_code"`

    // Headers from the target response
    Headers map[string]string `json:"headers,omitempty"`

    // Body is the response body
    Body []byte `json:"body,omitempty"`

    // SessionID if a session was created or used
    SessionID string `json:"session_id,omitempty"`

    // Timing contains request timing breakdown
    Timing *TimingDTO `json:"timing,omitempty"`

    // Meta contains relay metadata
    Meta *RelayMetaDTO `json:"meta,omitempty"`
}

// TimingDTO contains request timing details
type TimingDTO struct {
    DNSLookup    string `json:"dns_lookup,omitempty"`
    TCPConnect   string `json:"tcp_connect,omitempty"`
    TLSHandshake string `json:"tls_handshake,omitempty"`
    FirstByte    string `json:"first_byte,omitempty"`
    Total        string `json:"total"`
}

// RelayMetaDTO contains relay-specific metadata
type RelayMetaDTO struct {
    Retries    int      `json:"retries,omitempty"`
    Pool       string   `json:"pool,omitempty"`
    EndpointID string   `json:"endpoint_id,omitempty"`
    Errors     []string `json:"errors,omitempty"`
}
```

> **IMPORTANT**: Breaking Change - The `headers` field changes from `[{key, value}]` array format to `{"key": "value"}` object format. The existing `HeaderMap.UnmarshalJSON` already supports both formats for backwards compatibility on input, but output will change.

---

### Component 3: Routing Rule DTOs

#### [NEW] `internal/server/dto/routing_rule.go`

```go
package dto

import "time"

// CreateRoutingRuleRequest is the request to create a routing rule
type CreateRoutingRuleRequest struct {
    Name                 string              `json:"name" validate:"required"`
    RequiredTags         []string            `json:"required_tags"`
    ExcludedTags         []string            `json:"excluded_tags,omitempty"`
    Priority             int                 `json:"priority"`
    HardTimeout          string              `json:"hard_timeout,omitempty"` // Duration string
    RateLimitPerMinute   int                 `json:"rate_limit_per_minute,omitempty"`
    RateLimitPerSecond   int                 `json:"rate_limit_per_second,omitempty"`
    AllowedEndpointTypes []string            `json:"allowed_endpoint_types,omitempty"`
    RequiredEndpointCaps []string            `json:"required_endpoint_caps,omitempty"`
    FingerprintPreset    string              `json:"fingerprint_preset,omitempty"`
    FingerprintABTest    *ABConfigDTO        `json:"fingerprint_ab_test,omitempty"`
    QuotaKey             string              `json:"quota_key,omitempty"`
    AllowInsecureTLS     bool                `json:"allow_insecure_tls,omitempty"`
    RequestFilters       *RequestFilterDTO   `json:"request_filters,omitempty"`
    EndpointPools        []EndpointPoolDTO   `json:"endpoint_pools,omitempty"`
    IsActive             bool                `json:"is_active"`
}

// UpdateRoutingRuleRequest is the request to update a routing rule
type UpdateRoutingRuleRequest struct {
    CreateRoutingRuleRequest
    Version int `json:"version" validate:"required"` // For optimistic locking
}

// RoutingRuleResponse is the API response for a routing rule
type RoutingRuleResponse struct {
    ID                   string              `json:"id"`
    Name                 string              `json:"name"`
    RequiredTags         []string            `json:"required_tags"`
    ExcludedTags         []string            `json:"excluded_tags,omitempty"`
    Priority             int                 `json:"priority"`
    HardTimeout          string              `json:"hard_timeout,omitempty"`
    RateLimitPerMinute   int                 `json:"rate_limit_per_minute,omitempty"`
    RateLimitPerSecond   int                 `json:"rate_limit_per_second,omitempty"`
    AllowedEndpointTypes []string            `json:"allowed_endpoint_types,omitempty"`
    RequiredEndpointCaps []string            `json:"required_endpoint_caps,omitempty"`
    FingerprintPreset    string              `json:"fingerprint_preset,omitempty"`
    FingerprintABTest    *ABConfigDTO        `json:"fingerprint_ab_test,omitempty"`
    QuotaKey             string              `json:"quota_key,omitempty"`
    AllowInsecureTLS     bool                `json:"allow_insecure_tls,omitempty"`
    RequestFilters       *RequestFilterDTO   `json:"request_filters,omitempty"`
    EndpointPools        []EndpointPoolDTO   `json:"endpoint_pools,omitempty"`
    IsActive             bool                `json:"is_active"`
    Version              int                 `json:"version"`
    CreatedAt            time.Time           `json:"created_at"`
    UpdatedAt            time.Time           `json:"updated_at"`
}

// ABConfigDTO represents A/B test configuration
type ABConfigDTO struct {
    Variants []ABVariantDTO `json:"variants"`
    Strategy string         `json:"strategy"`
}

// ABVariantDTO represents an A/B test variant
type ABVariantDTO struct {
    Fingerprint string `json:"fingerprint"`
    Weight      int    `json:"weight"`
}

// RequestFilterDTO represents request filtering configuration
type RequestFilterDTO struct {
    BlockContentTypes []string `json:"block_content_types,omitempty"`
    BlockURLPatterns  []string `json:"block_url_patterns,omitempty"`
    BlockDomains      []string `json:"block_domains,omitempty"`
    EnableAdblock     bool     `json:"enable_adblock,omitempty"`
    AdblockLists      []string `json:"adblock_lists,omitempty"`
}

// EndpointPoolDTO represents an endpoint pool tier
type EndpointPoolDTO struct {
    Tier       int      `json:"tier"`
    Endpoints  []string `json:"endpoints"`
    MaxRetries int      `json:"max_retries"`
}
```

---

### Component 4: API Key DTOs

#### [NEW] `internal/server/dto/api_key.go`

```go
package dto

import "time"

// CreateApiKeyRequest is the request to create an API key
type CreateApiKeyRequest struct {
    Name              string   `json:"name" validate:"required"`
    Scopes            []string `json:"scopes"`
    RateLimitOverride *int     `json:"rate_limit_override,omitempty"`
}

// ApiKeyResponse is the API response for an API key (without sensitive data)
type ApiKeyResponse struct {
    ID                string     `json:"id"`
    Name              string     `json:"name"`
    Scopes            []string   `json:"scopes"`
    RateLimitOverride *int       `json:"rate_limit_override,omitempty"`
    IsActive          bool       `json:"is_active"`
    CreatedAt         time.Time  `json:"created_at"`
    ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

// CreateApiKeyResponse includes the raw key (shown only once)
type CreateApiKeyResponse struct {
    ApiKeyResponse
    RawKey string `json:"raw_key"`
}

// ListApiKeysResponse is the paginated list of API keys
type ListApiKeysResponse = PaginatedResponse[ApiKeyResponse]
```

> **NOTE**: The `TokenHash` field is intentionally **excluded** from `ApiKeyResponse` as it should never be exposed via API.

---

### Component 5: Fingerprint DTOs

#### [NEW] `internal/server/dto/fingerprint.go`

```go
package dto

import "time"

// CreateFingerprintRequest is the request to create/update a fingerprint preset
type CreateFingerprintRequest struct {
    ID     string                 `json:"id" validate:"required"`
    Name   string                 `json:"name" validate:"required"`
    Config map[string]interface{} `json:"config"`
}

// FingerprintResponse is the API response for a fingerprint preset
type FingerprintResponse struct {
    ID        string                 `json:"id"`
    Name      string                 `json:"name"`
    Config    map[string]interface{} `json:"config"`
    CreatedAt time.Time              `json:"created_at"`
    UpdatedAt time.Time              `json:"updated_at"`
}
```

---

### Component 6: Usage DTOs (Refactor Existing)

#### [NEW] `internal/server/dto/usage.go`

```go
package dto

// UsageSummaryDTO represents daily usage data
type UsageSummaryDTO struct {
    Date          string           `json:"date"`
    TotalRequests int64            `json:"total_requests"`
    TotalBytes    int64            `json:"total_bytes"`
    CostUnits     float64          `json:"cost_units"`
    Breakdown     map[string]int64 `json:"breakdown"`
}

// UsageSummaryResponse is the response for usage summaries
type UsageSummaryResponse struct {
    Data  []UsageSummaryDTO `json:"data"`
    Start string            `json:"start"`
    End   string            `json:"end"`
}

// BillingEstimateResponse is the response for billing estimates
type BillingEstimateResponse struct {
    TotalCostUnits float64 `json:"total_cost_units"`
    EstimatedUSD   float64 `json:"estimated_usd"`
    Currency       string  `json:"currency"`
    Start          string  `json:"start"`
    End            string  `json:"end"`
}
```

---

### Component 7: Mappers

#### [NEW] `internal/server/dto/mappers.go`

```go
package dto

import (
    "time"
    
    "github.com/kwilabs/straw-proxy-server/internal/domain"
    "github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// --- Relay Mappers ---

// ToProtocolRequest converts RelayRequest DTO to protocol.Request
func (r *RelayRequest) ToProtocolRequest() (*protocol.Request, error) {
    var timeout time.Duration
    if r.Timeout != "" {
        var err error
        timeout, err = time.ParseDuration(r.Timeout)
        if err != nil {
            return nil, err
        }
    }
    
    headers := make(protocol.HeaderMap, 0, len(r.Headers))
    for k, v := range r.Headers {
        headers = append(headers, protocol.Header{Key: k, Value: v})
    }
    
    return &protocol.Request{
        ID:        r.ID,
        Method:    r.Method,
        URL:       r.URL,
        Headers:   headers,
        Body:      r.Body,
        Timeout:   timeout,
        SessionID: r.SessionID,
        TraceID:   r.TraceID,
    }, nil
}

// FromProtocolResponse converts protocol.Response to RelayResponse DTO
func FromProtocolResponse(resp *protocol.Response, meta *RelayMetaDTO) *RelayResponse {
    headers := make(map[string]string, len(resp.Headers))
    for _, h := range resp.Headers {
        headers[h.Key] = h.Value
    }
    
    var timing *TimingDTO
    if resp.Timing != nil {
        timing = &TimingDTO{
            DNSLookup:    resp.Timing.DNSLookup.String(),
            TCPConnect:   resp.Timing.TCPConnect.String(),
            TLSHandshake: resp.Timing.TLSHandshake.String(),
            FirstByte:    resp.Timing.FirstByte.String(),
            Total:        resp.Timing.Total.String(),
        }
    }
    
    return &RelayResponse{
        RequestID:  resp.RequestID,
        StatusCode: resp.StatusCode,
        Headers:    headers,
        Body:       resp.Body,
        SessionID:  resp.SessionID,
        Timing:     timing,
        Meta:       meta,
    }
}

// --- Routing Rule Mappers ---

// ToDomain converts CreateRoutingRuleRequest to domain.RoutingRule
func (r *CreateRoutingRuleRequest) ToDomain() (*domain.RoutingRule, error) {
    var hardTimeout time.Duration
    if r.HardTimeout != "" {
        var err error
        hardTimeout, err = time.ParseDuration(r.HardTimeout)
        if err != nil {
            return nil, err
        }
    }
    
    return &domain.RoutingRule{
        Name:                 r.Name,
        RequiredTags:         r.RequiredTags,
        ExcludedTags:         r.ExcludedTags,
        Priority:             r.Priority,
        HardTimeout:          hardTimeout,
        RateLimitPerMinute:   r.RateLimitPerMinute,
        RateLimitPerSecond:   r.RateLimitPerSecond,
        AllowedEndpointTypes: r.AllowedEndpointTypes,
        RequiredEndpointCaps: r.RequiredEndpointCaps,
        FingerprintPreset:    r.FingerprintPreset,
        FingerprintABTest:    r.FingerprintABTest.ToDomain(),
        QuotaKey:             r.QuotaKey,
        AllowInsecureTLS:     r.AllowInsecureTLS,
        RequestFilters:       r.RequestFilters.ToDomain(),
        EndpointPools:        EndpointPoolsDTOToDomain(r.EndpointPools),
        IsActive:             r.IsActive,
    }, nil
}

// FromRoutingRule converts domain.RoutingRule to RoutingRuleResponse DTO
func FromRoutingRule(rule *domain.RoutingRule) *RoutingRuleResponse {
    // Implementation converts domain to DTO
    // ...
}

// --- API Key Mappers ---

// FromApiKey converts domain.ApiKey to ApiKeyResponse DTO
func FromApiKey(key *domain.ApiKey) *ApiKeyResponse {
    return &ApiKeyResponse{
        ID:                key.ID,
        Name:              key.Name,
        Scopes:            key.Scopes,
        RateLimitOverride: key.RateLimitOverride,
        IsActive:          key.IsActive,
        CreatedAt:         key.CreatedAt,
        ExpiresAt:         key.ExpiresAt,
    }
}

// FromApiKeys converts a slice of domain.ApiKey to ApiKeyResponse DTOs
func FromApiKeys(keys []domain.ApiKey) []ApiKeyResponse {
    result := make([]ApiKeyResponse, len(keys))
    for i, k := range keys {
        result[i] = *FromApiKey(&k)
    }
    return result
}

// --- Fingerprint Mappers ---

// ToDomain converts CreateFingerprintRequest to domain.FingerprintPreset
func (r *CreateFingerprintRequest) ToDomain() *domain.FingerprintPreset {
    return &domain.FingerprintPreset{
        ID:     r.ID,
        Name:   r.Name,
        Config: domain.ConfigMap(r.Config),
    }
}

// FromFingerprintPreset converts domain.FingerprintPreset to FingerprintResponse
func FromFingerprintPreset(p *domain.FingerprintPreset) *FingerprintResponse {
    return &FingerprintResponse{
        ID:        p.ID,
        Name:      p.Name,
        Config:    map[string]interface{}(p.Config),
        CreatedAt: p.CreatedAt,
        UpdatedAt: p.UpdatedAt,
    }
}
```

---

### Handler Modifications

#### [MODIFY] `internal/server/handlers/relay.go`

```diff
-import "github.com/kwilabs/straw-proxy-server/pkg/protocol"
+import "github.com/kwilabs/straw-proxy-server/internal/server/dto"

 func (h *RelayHandler) Handle(c echo.Context) error {
-    var req protocol.Request
+    var reqDTO dto.RelayRequest
-    if err := c.Bind(&req); err != nil {
+    if err := c.Bind(&reqDTO); err != nil {
         return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
     }
+
+    req, err := reqDTO.ToProtocolRequest()
+    if err != nil {
+        return echo.NewHTTPError(http.StatusBadRequest, err.Error())
+    }
     // ... rest of handler
 }
```

#### [MODIFY] `internal/server/admin/handlers/routing_rule.go`

```diff
+import "github.com/kwilabs/straw-proxy-server/internal/server/dto"

 func (h *RoutingRuleHandler) HandleCreateRoutingRule(c echo.Context) error {
-    var rule domain.RoutingRule
-    if err := c.Bind(&rule); err != nil {
+    var req dto.CreateRoutingRuleRequest
+    if err := c.Bind(&req); err != nil {
         return c.JSON(http.StatusBadRequest, ...)
     }
+
+    rule, err := req.ToDomain()
+    if err != nil {
+        return c.JSON(http.StatusBadRequest, ...)
+    }
     // ... rest of handler
-    return c.JSON(http.StatusCreated, rule)
+    return c.JSON(http.StatusCreated, dto.FromRoutingRule(rule))
 }
```

#### [MODIFY] `internal/server/admin/handlers/api_key.go`

```diff
+import "github.com/kwilabs/straw-proxy-server/internal/server/dto"

 func (h *ApiKeyHandler) HandleListApiKeys(c echo.Context) error {
     keys, total, err := h.repo.List(...)
     // ...
-    return c.JSON(http.StatusOK, ListApiKeysResponse{
-        Data:  keys,  // Exposes domain.ApiKey with TokenHash!
+    return c.JSON(http.StatusOK, dto.ListApiKeysResponse{
+        Data:  dto.FromApiKeys(keys),  // Filters out TokenHash
         Total: total,
         Page:  page,
         Limit: limit,
     })
 }
```

#### [DELETE] `internal/server/admin/handlers/types.go`

Move all types to `internal/server/dto/` package and delete this file.

---

## Verification Plan

### Automated Tests

1. **Unit Tests for Mappers**

   ```bash
   go test ./internal/server/dto/... -v
   ```

2. **Handler Tests Update**
   - Update all existing handler tests to use DTOs
   - Verify JSON serialization matches expected format

   ```bash
   go test ./internal/server/handlers/... -v
   go test ./internal/server/admin/handlers/... -v
   ```

3. **Integration Tests**

   ```bash
   go test ./test/integration/... -v
   ```

4. **Swagger Regeneration**

   ```bash
   make swagger
   # Verify generated docs reference dto types, not domain types
   ```

### Manual Verification

1. **Swagger UI Check**
   - Start server and visit `/swagger/index.html`
   - Verify request/response schemas show clean DTO types
   - Confirm no `internal/domain` or `pkg/protocol` paths in schema

2. **API Contract Test**

   ```bash
   # Verify relay request works with new DTO format
   curl -X POST http://localhost:8080/v1/request \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{"url": "https://httpbin.org/ip", "headers": {"Accept": "application/json"}}'
   ```

---

## Migration Strategy

1. **Phase 1: Create DTO Package** (No breaking changes)
   - Add `internal/server/dto/` with all types and mappers
   - Add comprehensive tests for mappers

2. **Phase 2: Update Admin Handlers**
   - Migrate admin handlers one at a time
   - Update corresponding tests
   - Delete old `types.go`

3. **Phase 3: Update Relay Handler**
   - Update relay handler to use DTOs
   - This is the most critical path - test thoroughly

4. **Phase 4: Regenerate Swagger**
   - Run `make swagger`
   - Review and verify generated docs
   - Update any documentation

---

## Effort Estimate

| Phase | Task | Estimated Hours |
|-------|------|-----------------|
| 1 | Create DTO package with types | 2-3 |
| 1 | Create mapper functions | 2-3 |
| 1 | Mapper unit tests | 1-2 |
| 2 | Update admin handlers | 2-3 |
| 2 | Update admin handler tests | 2-3 |
| 3 | Update relay handler | 1-2 |
| 3 | Update relay handler tests | 1-2 |
| 4 | Regenerate Swagger, review | 1 |
| 4 | Integration testing | 1-2 |
| **Total** | | **13-20 hours** |

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking API changes | High | Maintain backwards-compatible input parsing (already done for headers) |
| Mapper bugs | Medium | Comprehensive unit tests for all mappers |
| Swagger generation issues | Low | Review generated docs before release |
| Test failures | Medium | Update tests incrementally per phase |
