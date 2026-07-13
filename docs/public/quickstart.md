---
sidebar_position: 2
---

# Quickstart

## Requirements

- Git
- Docker with Compose v2
- `make`

No language toolchain or database is required to run the local stack.

## Start Straw

```sh
git clone https://github.com/beremaran/straw-oss.git
cd straw-oss
make dev
```

The command builds and starts NATS, Control, and one Egress worker. It waits for Control to become ready. Local
development does not require credentials or provisioning.

Expected final line: `local stack: ready`. Every command on this page is exercised by the maintained
`make quickstart-smoke` check.

## Send a request

```sh
curl -sS \
  -H 'Content-Type: application/json' \
  -d '{"method":"GET","url":"https://example.com"}' \
  http://localhost:8080/api/v1/requests
```

A successful result has outer HTTP status `200`. Its `status` field is the upstream response status. Decode the
upstream body with:

```sh
curl -sS \
  -H 'Content-Type: application/json' \
  -d '{"method":"GET","url":"https://example.com"}' \
  http://localhost:8080/api/v1/requests \
  | jq -r '.body.data_base64' | base64 --decode
```

## Check the stack

```sh
make dev-status
curl -fsS http://localhost:9090/healthz
curl -fsS http://localhost:9090/readyz
curl -fsS http://localhost:9090/metrics | grep '^straw_'
```

Use `make dev-logs`, `make dev-down`, or `make dev-reset` to inspect, stop, or rebuild the stack. If the default host
ports are occupied, see [local port overrides](deployment.md#local-development).

Next, read the [request API](api/requests.md) or try the [CLI and SDKs](sdk.md).
