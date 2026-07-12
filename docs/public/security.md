---
sidebar_position: 10
---

# Security

Straw can make network requests on behalf of clients. Treat Control access as sensitive and place it behind the same
controls you would apply to an internal egress gateway.

## Required production controls

- Set a long random `STRAW_AUTH_TOKEN` and send it only over TLS.
- If runtime administration is enabled, set a different long random `STRAW_ADMIN_TOKEN`; restrict `/admin/` and
  `/api/v1/admin/*` more tightly than the request endpoint.
- Authenticate NATS and keep it off the public internet.
- Restrict Control, metrics, NATS monitoring, and worker health listeners with network policy or firewalls.
- Run separate deployments for workloads that must not share policy or credentials.
- Pin container images and review configuration changes.
- Keep destination DNS and outbound network controls appropriate to your environment.

An empty Control token is intentionally supported for the default local stack. Do not use that setting on an
untrusted network.

## Request behavior

Straw accepts only absolute HTTP/HTTPS URLs, rejects URL user information, validates headers, limits bodies and
timeouts, and manages hop-by-hop headers. The deployment policy rejects destinations that violate built-in safety
rules. TLS is verified by the worker's HTTP stack.

Bearer tokens and NATS passwords are environment values in the production example. Do not commit `.env` files or
place secret values in JSON configuration.

Runtime administration is deployment-scoped authorization, not RBAC. Anyone holding the admin token can change all
deployment policy, control every worker, view active request IDs, cancel requests, and roll back configuration.

## Reporting vulnerabilities

Do not open a public issue for a suspected vulnerability. Follow the private reporting instructions in the
repository's `SECURITY.md`.
