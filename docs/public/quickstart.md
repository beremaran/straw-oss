# Quickstart Guide

This guide helps you spin up a complete local development deployment of Straw using Docker Compose, provision a tenant API key, and route your first outbound HTTP request.

## Prerequisites

Ensure you have the following installed on your machine:
- [Docker](https://www.docker.com/) and [Docker Compose](https://docs.docker.com/compose/)
- [curl](https://curl.se/) for sending test HTTP requests

---

## Step 1: Start the Docker Compose Stack

The repository includes a ready-to-run `docker-compose.yml` that sets up:
- **Control plane**: Core REST and configuration API server.
- **Egress worker**: Outbound HTTP/HTTPS execution daemon.
- **Postgres**: Store for tenant identities, pools, routes, and policies.
- **Redis**: Backend for tenant rate-limits, quotas, and sticky session tracking.
- **NATS**: Real-time communication and work scheduling between Control and Egress.
- **ClickHouse**: Storage for structured metadata events (e.g. audit history, request summaries).

To start the stack and provision redacted local credentials, run:

```bash
# 1. From the Straw repository root, boot the cluster
cd /path/to/straw
make infra-up
```

You can verify that the Control plane is healthy by querying its readiness endpoint:

```bash
# 2. Check readiness status
curl -fsS http://localhost:9090/readyz && echo "Control plane ready"
```

Once the stack is up, the Egress worker automatically registers itself with the Control plane over NATS, matching a pre-seeded development credential.

---

## Step 2: Load the Local Tenant API Key

A default development tenant `22222222-2222-4222-8222-222222222222` is bootstrapped at startup. `make infra-up`
provisions a tenant-scoped `requester` key once and stores it with owner-only permissions.

Load the local environment without printing its values:

```bash
set -a
source deploy/local/.dev/local.env
set +a
```

---

## Step 3: Route Your First Request

Now that you have a tenant-scoped API key, you can send an HTTP request to the forwarding endpoint. The request will be validated, assigned to the eligible local Egress worker over NATS, executed, and returned.

Send a `GET` request to `https://example.com/`:

```bash
curl -s -H "Authorization: Bearer ${STRAW_API_KEY}" \
  -H 'Content-Type: application/json' \
  -d '{"method":"GET","url":"https://example.com/","timeout_ms":15000}' \
  http://localhost:8080/api/v1/requests
```

To exercise the named transport, add `"fingerprint_profile":"chrome_120"` to the request. Omit the field or use
`"fingerprint_profile":"default"` to preserve the baseline transport. The named request is accepted only when the
registered Egress worker advertises `chrome_120`.

The response contains a wrapper envelope carrying the upstream status, headers, and base64-encoded body:

```json
{
  "request_id": "req_1783260685717525503",
  "status": 200,
  "headers": [
    {
      "name": "Age",
      "value_base64": "MTIwMDU="
    },
    {
      "name": "Allow",
      "value_base64": "R0VULCBIRUFE"
    },
    {
      "name": "Cf-Cache-Status",
      "value_base64": "SElU"
    },
    {
      "name": "Cf-Ray",
      "value_base64": "YTE2NmY1Nzg4OWY3MTIwYy1QRVI="
    },
    {
      "name": "Content-Type",
      "value_base64": "dGV4dC9odG1s"
    },
    {
      "name": "Date",
      "value_base64": "U3VuLCAwNSBKdWwgMjAyNiAxNDoxMToyNiBHTVQ="
    },
    {
      "name": "Last-Modified",
      "value_base64": "V2VkLCAwMSBKdWwgMjAyNiAxNzo1MjozNyBHTVQ="
    },
    {
      "name": "Server",
      "value_base64": "Y2xvdWRmbGFyZQ=="
    }
  ],
  "body": {
    "mode": "inline_base64",
    "data_base64": "PCFkb2N0eXBlIGh0bWw+PGh0bWwgbGFuZz0iZW4iPjxoZWFkPjx0aXRsZT5FeGFtcGxlIERvbWFpbjwvdGl0bGU+PGxpbmsgcmVsPSJpY29uIiBocmVmPSJkYXRhOiwiPjxtZXRhIG5hbWU9InZpZXdwb3J0IiBjb250ZW50PSJ3aWR0aD1kZXZpY2Utd2lkdGgsIGluaXRpYWwtc2NhbGU9MSI+PHN0eWxlPmJvZHl7YmFja2dyb3VuZDojZWVlO3dpZHRoOjYwdnc7bWFyZ2luOjE1dmggYXV0bztmb250LWZhbWlseTpzeXN0ZW0tdWksc2Fucy1zZXJpZn1oMXtmb250LXNpemU6MS41ZW19ZGl2e29wYWNpdHk6MC44fWE6bGluayxhOnZpc2l0ZWR7Y29sb3I6IzM0OH08L3N0eWxlPjwvaGVhZD48Ym9keT48ZGl2PjxoMT5FeGFtcGxlIERvbWFpbjwvaDE+PHA+VGhpcyBkb21haW4gaXMgZm9yIHVzZSBpbiBkb2N1bWVudGF0aW9uIGV4YW1wbGVzIHdpdGhvdXQgbmVlZGluZyBwZXJtaXNzaW9uLiBBdm9pZCB1c2UgaW4gb3BlcmF0aW9ucy48L3A+PHA+PGEgaHJlZj0iaHR0cHM6Ly9pYW5hLm9yZy9kb21haW5zL2V4YW1wbGUiPkxlYXJuIG1vcmU8L2E+PC9wPjwvZGl2PjwvYm9keT48L2h0bWw+Cg==","truncated":false},
  "timing": {
    "routing_ms": 0,
    "assignment_ms": 3,
    "egress_ms": 290,
    "total_ms": 301
  }
}
```

> [!NOTE]
> The outer HTTP status code is `200 OK` as long as Straw successfully routes and retrieves the response from the upstream server. If the upstream server returned a `404 Not Found` or `500 Internal Error`, that status code would be contained inside the envelope's `"status"` field, while the outer response status would remain `200`.

---

## Step 4: Verify Telemetry In ClickHouse

The control plane buffers metadata about request outcomes and drains it asynchronously to ClickHouse. You can query the database directly to verify the event was logged:

```bash
curl -s http://localhost:8123/ \
  --data-binary 'SELECT * FROM straw.request_events FORMAT Vertical'
```

For the profile proof, inspect only the redacted evidence columns:

```bash
curl -fsS http://localhost:8123/ --data-binary \
  "SELECT request_id, requested_fingerprint_profile, selected_fingerprint_profile, executed_fingerprint_profile, upstream_status, error_code FROM straw.request_events ORDER BY timestamp DESC LIMIT 1 FORMAT Vertical"
```

The repository-level verification commands are `make check`, `make postgres-migrations-check`, and
`make clickhouse-migrations-check`.
