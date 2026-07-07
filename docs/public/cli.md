# CLI Reference

The `straw` CLI is a thin command-line wrapper around the Go SDK for exercising the Control plane's REST API from a terminal or scripts — sending requests, managing configuration, and running admin actions without hand-writing `curl` calls.

## Installation

Build it from the repository (there is no separate published binary yet):

```bash
go build -o straw ./cmd/straw
```

## Global Configuration

Every subcommand accepts `--base-url` and `--api-key` flags, which default to the environment variables below if unset:

| Flag | Environment Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `--base-url` | `STRAW_BASE_URL` | `http://localhost:8080` | Control plane REST API base URL. |
| `--api-key` | `STRAW_API_KEY` | *(none)* | API key sent as the `Authorization: Bearer` token. |

```bash
export STRAW_BASE_URL=http://localhost:8080
export STRAW_API_KEY=sk_example_requester_secret
```

---

## `straw request` — Forward an Egress Request

Submits a request to [`POST /api/v1/requests`](api/requests.md) (or the streaming variant with `--stream`).

```bash
straw request --method GET --url https://example.com [--stream] [--header 'Name: value'] [--body-file path]
straw request --json request.json
```

| Flag | Description |
| :--- | :--- |
| `--method` | HTTP method. Required unless supplied via `--json`. |
| `--url` | Absolute upstream URL. Required unless supplied via `--json`. |
| `--json` | Path to a JSON file containing a full request envelope (or `-` for stdin). Flags below override fields it sets. |
| `--header` | Repeatable. `Name: value` pair, forwarded as a request header. |
| `--body-file` | Path to a file whose contents become the base64-encoded request body. |
| `--timeout-ms` | Request execution timeout in milliseconds. |
| `--fingerprint-profile` | One of the [seeded fingerprint profiles](api/config.md#9-fingerprint-profiles-read-only). |
| `--capture-hint` | Payload capture hint; see [Payload Capture Policy](api/config.md#12-payload-capture-policy). |
| `--stream` | Use the [streaming transport](api/requests.md#streaming-transport-post-apiv1requestsstream) and print frames as they arrive instead of waiting for the full response. |

Example:

```bash
straw request --method GET --url https://api.github.com/users/octocat \
  --header 'User-Agent: straw-cli'
```

---

## `straw config` — Manage Tenant Configuration

Wraps the [Configuration APIs](api/config.md). Resource names match the REST path segment (e.g. `tenants`, `routing-rules`, `executor-pools`, `deny-rules`, `injection-policies`, `quotas`, `rate-limits`, `fingerprint-profiles`, `changes`, `payload-capture`).

```bash
straw config list [--limit n] [--offset n] <resource>
straw config get tenants <id>|quotas|rate-limits|fingerprint-profiles|changes
straw config create --json body.json <resource>
straw config update --json body.json <resource-path>
straw config delete <resource-path>
straw config revoke platform-api-keys|api-keys|worker-credentials <id>
straw config rollback --json body.json
```

Examples:

```bash
# List all routing rules for the caller's tenant
straw config list routing-rules

# Create a routing rule from a JSON file
straw config create --json rule.json routing-rules

# Update rate limits in place
straw config update --json limits.json rate-limits

# Roll back to an earlier config version — see api/config.md#11-config-rollback
straw config rollback --json '{"expected_config_version":12,"target_config_version":8,"reason":"bad rule"}'
```

`create`/`update` bodies are the same JSON payloads documented per-resource in the [Configuration API reference](api/config.md); `--json -` reads the body from stdin.

---

## `straw admin` — Runtime Administration

Wraps the [Runtime Administration APIs](api/admin.md).

```bash
straw admin workers
straw admin worker <worker_id> disable|enable|drain|undrain|tenant-disable|tenant-enable|tenant-drain|tenant-undrain
straw admin cancel <request_id>
```

```bash
straw admin workers
straw admin worker egress-us-west-01 tenant-drain
straw admin cancel req_1783260685717525503
```

---

## `straw healthz` / `readyz` / `metrics`

Convenience wrappers that `GET` the Control plane's [health, readiness, and Prometheus endpoints](operations.md#1-health--readiness-probes) at `--base-url`:

```bash
straw healthz
straw readyz
straw metrics
```

---

## Output & Exit Codes

All commands print the raw JSON response body (or PEM/text for endpoints that return one) to stdout and exit non-zero on any transport or API error, making the CLI safe to script with `set -e` and pipe into `jq`.
