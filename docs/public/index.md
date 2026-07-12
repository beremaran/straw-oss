---
sidebar_position: 1
slug: /
---

# Straw documentation

Straw is a self-hosted HTTP/HTTPS egress proxy. Applications send an HTTP request to Control, Control assigns it to
an Egress worker over NATS, and that worker makes the outbound request. The response comes back as JSON with the body
encoded as base64.

Straw is designed for one trusted deployment boundary. It does not include tenants, accounts, billing, quotas, or an
analytics database. NATS is the only required backing service. An optional JetStream profile stores runtime policy.

## Start here

1. Follow the [quickstart](quickstart.md) to run Straw locally and send a request.
2. Read the [architecture](architecture.md) to understand the three services.
3. Use the [request API](api/requests.md), [CLI](cli.md), or [SDKs](sdk.md) from your application.
4. Review [configuration](configuration.md), [deployment](deployment.md), and [security](security.md) before exposing
   Control outside a development machine. Use [runtime administration](runtime-administration.md) when policy must
   change without restarts.

## What Straw does

- forwards HTTP and HTTPS requests through independently scalable workers;
- keeps workers off the public API network;
- preserves duplicate and ordered request/response headers;
- enforces request size and timeout limits;
- supports an optional deployment-wide bearer token;
- exposes Prometheus metrics, health endpoints, and structured JSON logs;
- supports selectable outbound TLS fingerprint profiles;
- optionally provides a durable Config/Admin API and API-parity dashboard.

## What Straw does not do

Straw is not a forward-proxy socket, browser automation system, hosted proxy network, tenant management platform, or
durable traffic archive. The request interface is `POST /api/v1/requests`; optional administration remains
deployment-scoped and belongs to the operator.

The source is available under the [MIT License](https://github.com/beremaran/straw-oss/blob/master/LICENSE).
