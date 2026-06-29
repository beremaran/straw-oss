# Management API Reference

The Management API serves as the control gateway for managing API keys, routing rules, fingerprint presets, and inspecting worker health and billing statistics.

> [!TIP]
> **Interactive API Reference (OpenAPI)**
> We provide a comprehensive, interactive API reference generated with Redocly.
> * View the interactive docs: **[API Reference (Redoc)](api-reference.html)**
> * Download the OpenAPI YAML specification: **[openapi.yaml](openapi.yaml)**


## 🔒 Authentication

All requests targeting the Management API endpoints (`/management/*`) must be authenticated by supplying the `MANAGEMENT_API_KEY` configured in your Relay environment as a Bearer token in the `Authorization` header:

```http
Authorization: Bearer <MANAGEMENT_API_KEY>
```

---

## 🛠️ API Key Management

### Create API Key
Creates a new API client key. The response contains a `raw_key` which is **only returned once** upon creation. Save it securely.

* **URL**: `POST /management/api-keys`
* **Request Body**:
  ```json
  {
    "name": "Production Crawler Key",
    "scopes": ["target:*", "type:residential", "region:us"],
    "rate_limit_override": 100
  }
  ```
* **Response (Status 201 Created)**:
  ```json
  {
    "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "name": "Production Crawler Key",
    "scopes": ["target:*", "type:residential", "region:us"],
    "rate_limit_override": 100,
    "is_active": true,
    "raw_key": "4cf5df...3a2f8c" 
  }
  ```

### List API Keys
Retrieves a paginated list of all API keys (active and inactive).

* **URL**: `GET /management/api-keys`
* **Query Params**:
  * `page` (default: `1`)
  * `limit` (default: `20`, max: `100`)
* **Response (Status 200 OK)**:
  ```json
  {
    "data": [
      {
        "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
        "name": "Production Crawler Key",
        "scopes": ["target:*", "type:residential"],
        "rate_limit_override": 100,
        "is_active": true
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 20
  }
  ```

### Revoke API Key
Disables an API key immediately.

* **URL**: `DELETE /management/api-keys/{id}`
* **Response (Status 204 No Content)**: *(empty)*

### Get API Key Detail
Returns API key metadata plus token history metadata. Token hashes and raw secrets are never returned.

* **URL**: `GET /management/api-keys/{id}`
* **Response (Status 200 OK)**:
  ```json
  {
    "id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "name": "Production Crawler Key",
    "scopes": ["target:*", "type:residential"],
    "rate_limit_override": 100,
    "is_active": true,
    "tokens": [
      {
        "id": "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
        "status": "active",
        "created_at": "2026-06-29T12:00:00Z"
      }
    ]
  }
  ```

### Update API Key
Updates API key metadata under the same logical key ID. All fields are optional. Set `expires_at` to `""` or `null` to clear it.

* **URL**: `PATCH /management/api-keys/{id}`
* **Request Body**:
  ```json
  {
    "name": "Production Crawler Key",
    "scopes": ["target:*", "type:residential"],
    "rate_limit_override": 120,
    "expires_at": "2026-12-31T23:59:59Z",
    "is_active": true
  }
  ```

### Rotate API Key
Generates a new token secret for the same logical key ID. The returned `raw_key` is shown once. Previous accepted tokens are either revoked immediately or placed into grace until the supplied deadline.

* **URL**: `POST /management/api-keys/{id}/rotate`
* **Request Body**:
  ```json
  {
    "grace_period": "24h",
    "revoke_existing": false
  }
  ```
* **Response (Status 200 OK)**:
  ```json
  {
    "api_key_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    "raw_key": "4cf5df...3a2f8c",
    "token_id": "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
    "previous_tokens_grace_until": "2026-06-30T12:00:00Z"
  }
  ```

### Reactivate API Key
Reactivates a logical API key if it has not expired.

* **URL**: `POST /management/api-keys/{id}/reactivate`
* **Response (Status 200 OK)**: Returns the same payload as `GET /management/api-keys/{id}`.

### Revoke API Key (Explicit Route)
Revokes the logical key and all stored token secrets. `DELETE /management/api-keys/{id}` remains as the compatibility alias.

* **URL**: `POST /management/api-keys/{id}/revoke`
* **Response (Status 204 No Content)**: *(empty)*

---

## 🗺️ Routing Rules Management

### Create Routing Rule
Defines how incoming client proxy requests map to endpoint workers.

* **URL**: `POST /management/rules`
* **Request Body**:
  ```json
  {
    "name": "US Residential Egress",
    "priority": 100,
    "required_tags": ["type:residential", "region:us"],
    "excluded_tags": ["type:datacenter"],
    "config": {
      "mode": "tag_match",
      "retry_limit": 3
    },
    "quota_key": "residential-us-quota",
    "is_active": true
  }
  ```
* **Response (Status 201 Created)**:
  ```json
  {
    "id": "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
    "name": "US Residential Egress",
    "priority": 100,
    "required_tags": ["type:residential", "region:us"],
    "excluded_tags": ["type:datacenter"],
    "config": {
      "mode": "tag_match",
      "retry_limit": 3
    },
    "quota_key": "residential-us-quota",
    "is_active": true,
    "version": 1
  }
  ```

### List Routing Rules
Retrieves routing rules sorted by priority in descending order.

* **URL**: `GET /management/rules`
* **Response (Status 200 OK)**:
  ```json
  {
    "data": [...],
    "total": 5,
    "page": 1,
    "limit": 20
  }
  ```

### Get Routing Rule Details
* **URL**: `GET /management/rules/{id}`
* **Response (Status 200 OK)**: Returns the matching rule configuration object.

### Update Routing Rule
Updates a routing rule's parameters. Utilizes optimistic locking via the `version` field.

* **URL**: `PUT /management/rules/{id}`
* **Request Body**: Same schema as Create Rule, plus `"version": 1`.
* **Response (Status 200 OK)**: Returns the updated rule.

### Delete Routing Rule
* **URL**: `DELETE /management/rules/{id}`
* **Response (Status 204 No Content)**: *(empty)*

---

## 🖥️ Endpoint Monitoring & Management

### List Active Endpoints
Inspects heartbeats and health status of all workers connected to the message broker.

* **URL**: `GET /management/endpoints`
* **Response (Status 200 OK)**:
  ```json
  [
    {
      "endpoint_id": "worker-us-residential-01",
      "state": "healthy",
      "tags": ["type:residential", "region:us"],
      "version": "1.0.3",
      "active_tasks": 2,
      "last_seen": "2026-06-28T12:00:35Z"
    }
  ]
  ```

### Drain Endpoint
Instructs a worker to complete its active tasks but refuse any new incoming tasks, allowing for graceful worker recycling or node migration.

* **URL**: `POST /management/endpoints/{id}/drain`
* **Response (Status 200 OK)**: *(empty)*

---

## 🕵️ Client Fingerprint Presets

### List Fingerprint Presets
Lists preset user agents, HTTP/2 configurations, and TLS/JA3 spoofing parameters.

* **URL**: `GET /management/fingerprints`
* **Response (Status 200 OK)**: List of presets.

### Create Preset
Adds a new browser profile fingerprint.

* **URL**: `POST /management/fingerprints`
* **Request Body**:
  ```json
  {
    "id": "chrome-131",
    "name": "Chrome 131 Desktop Preset",
    "config": {
      "user_agent": "Mozilla/5.0 ... Chrome/131.0.0.0 Safari/537.36",
      "ja3": "771,4865-4866-4867...",
      "h2_settings": {
        "settings": { "1": 65536 },
        "connection_flow": 15663105
      }
    }
  }
  ```
* **Response (Status 200 OK)**: Created preset object.

### Broadcast Presets
Triggers a broadcast via NATS to push all registered fingerprint presets directly into memory on active worker nodes.

* **URL**: `POST /management/fingerprints/broadcast`
* **Response (Status 200 OK)**: *(empty)*

---

---

## 📜 Audit Viewer APIs

### List Audit Events
Lists management audit events (e.g., API key created, endpoint drained).

* **URL**: `GET /management/audit/events`
* **Query Params**:
  * `page` (default: 1)
  * `limit` (default: 20, max: 500)
  * `start_date` (RFC3339 format)
  * `end_date` (RFC3339 format)
  * `action` (e.g., `create`, `revoke`)
  * `actor_id`
* **Response (Status 200 OK)**:
  Returns a paginated list. Note: `old_value` and `new_value` bodies are only included if the requester holds the `Owner` or `Security auditor` role.

### Get Audit Event Details
Retrieves details for a specific audit event by ID.

* **URL**: `GET /management/audit/events/{id}`
* **Response (Status 200 OK)**: Returns the audit event object.

### List Audit Requests
Lists raw HTTP requests made to the Management API.

* **URL**: `GET /management/audit/requests`
* **Query Params**: Same as List Audit Events (except `action` is not used).
* **Response (Status 200 OK)**: Returns a paginated list of requests. The `body` is redacted unless the requester is an `Owner` or `Security auditor`.

### Export Audit Events
Exports audit events within a bounded date range (max 31 days).

* **URL**: `GET /management/audit/export`
* **Query Params**:
  * `start_date` (required)
  * `end_date` (required)
  * `format` (`csv` or `ndjson`, default: `csv`)
* **Response (Status 200 OK)**: Downloads the export file.

## 📊 Usage & Billing

### Get Daily Usage Summary
Returns daily logs of requests and transfer size.

* **URL**: `GET /management/usage/summary`
* **Query Params**:
  * `start` (date format: `YYYY-MM-DD`, default: 30 days ago)
  * `end` (date format: `YYYY-MM-DD`, default: today)
  * `api_key_id` (optional filter)
* **Response (Status 200 OK)**:
  ```json
  {
    "data": [
      {
        "api_key_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
        "date": "2026-06-28",
        "total_requests": 14022,
        "total_bytes": 104857600,
        "cost_units": 140220.00,
        "breakdown": {
          "residential": 14022
        }
      }
    ],
    "start": "2026-05-29",
    "end": "2026-06-28"
  }
  ```

### Get Billing Estimate
Aggregates daily summaries and calculates estimated costs in USD.

* **URL**: `GET /management/billing/estimate`
* **Query Params**:
  * `start` (default: start of the current month)
  * `end` (default: today)
  * `api_key_id` (optional filter)
* **Response (Status 200 OK)**:
  ```json
  {
    "total_cost_units": 140220.00,
    "estimated_usd": 14.022,
    "currency": "USD",
    "start": "2026-06-01",
    "end": "2026-06-28"
  }
  ```

---

## ⚡ Cache Management

### Clear Redis Cache
Cleans in-memory caches on Redis. Forces the Relay Server to pull the latest rules and keys from PostgreSQL.

* **URL**: `POST /management/cache/clear`
* **Response (Status 200 OK)**: *(empty)*

### Get Cache Statistics
Reports current hit rates and size estimates.

* **URL**: `GET /management/cache/stats`
* **Response (Status 200 OK)**:
  ```json
  {
    "keys_cached": 12,
    "hit_rate": 0.98
  }
  ```
