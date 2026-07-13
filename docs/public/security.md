# Security

Straw can make network requests on behalf of clients. Treat Control access as sensitive and place it behind the same
controls you would apply to an internal egress gateway.

## Required production controls

- Set a long random `STRAW_AUTH_TOKEN` and send it only over TLS.
- If runtime administration is enabled, set a different long random `STRAW_ADMIN_TOKEN`; restrict `/admin/` and
  `/api/v1/admin/*` more tightly than the request endpoint.
- Authenticate NATS and keep it off the public internet.
- In the HA profile, authenticate Redis, prefer `rediss://` outside a private host network, restrict it to Controls,
  and use a high-availability Redis service appropriate to your recovery objectives.
- Restrict Control, metrics, NATS monitoring, and worker health listeners with network policy or firewalls.
- Run separate deployments for workloads that must not share policy or credentials.
- Pin container images and review configuration changes.
- Keep destination DNS and outbound network controls appropriate to your environment.
- When receipts are enabled, keep `STRAW_RECEIPT_SIGNING_KEY` and S3 credentials in a secret manager, require TLS at
  Control, restrict bucket access, and enable configured server-side encryption.

An empty Control token is intentionally supported for the default local stack. Do not use that setting on an
untrusted network.

The forward proxy uses `Proxy-Authorization: Bearer <token>` so an end-destination `Authorization` header can pass
through decoded HTTP requests. Proxy credentials are stripped before dispatch. CONNECT traffic is opaque after the
tunnel is established: destination and resolved-address policy still applies when Egress dials, but Straw cannot
inspect application data inside the tunnel. Apply outbound network controls as defense in depth.

Proxy routing hints use the reserved `X-Straw-*` namespace. Control authenticates first, validates and normalizes the
bounded tags/country/region/IP-type/sticky-session contract, and strips every `X-Straw-*` header before decoded
upstream forwarding. CONNECT strips the same control headers before tunnel establishment and never applies header
injection or TLS fingerprinting to tunneled bytes. Unknown `X-Straw-*` headers are also stripped rather than forwarded.

## Profile hardening and verification checklist

| Profile | Required review before production | Owned verification |
| --- | --- | --- |
| Default | random request token; TLS at the ingress; NATS account limited to the documented subjects; Control/metrics/NATS listeners firewalled; structured logs exported without request data | `make profile-smoke PROFILE=default`, destination-policy tests, and a synthetic-secret log review |
| Runtime administration | all default controls; distinct admin token; `/admin/` and `/api/v1/admin/*` on a tighter network; JetStream account limited to the configured bucket; reviewed history/backup retention | `make profile-smoke PROFILE=admin` and `make state-backup-smoke PROFILE=admin` |
| HA Control | all default/admin controls in use; authenticated Redis ACL scoped to the Straw key prefix; `rediss://` or an equivalent private encrypted network; Redis/NATS reachable only by intended components; fencing and degraded readiness monitored | `make ha-smoke`, including Redis outage/recovery and Control loss |
| Receipts | all default controls; distinct 32-byte-or-longer signing key; least-privilege S3 bucket/prefix or UID-restricted local volume; TLS to S3; server-side encryption permissions; explicit object/part retention and cleanup alerts | `make profile-smoke PROFILE=receipts` and `make state-backup-smoke PROFILE=receipts`, including checksum rejection |
| Custom workers | distinct NATS credentials and least-privilege subjects; compatible protocol/SDK tags; outbound network policy; no long-lived object-store credential; bounded/redacted worker logs | `make conformance` plus the worker implementation's admission test |

Record the image digest, configuration revision, verification timestamp, and reviewer. Do not promote a profile when
its owned verification is skipped or when logs contain synthetic canary tokens, URLs, headers, or bodies.

## Request behavior

Straw accepts only absolute HTTP/HTTPS URLs, rejects URL user information, validates headers, limits bodies and
timeouts, and manages hop-by-hop headers. The deployment policy rejects destinations that violate built-in safety
rules, including post-DNS resolved-address checks and CNAME suffix checks. See [Destination policy and egress
safety](architecture.md#destination-policy-and-egress-safety) for the built-in denied ranges, override precedence, and
redirect behavior. TLS is verified by the worker's HTTP stack.

Bearer tokens, NATS passwords, and the Redis URL are environment values in the production examples. URL-encode Redis
credentials when necessary. Do not commit `.env` files or place secret values in JSON configuration.

Receipt object URLs are short-lived bearer capabilities scoped to one deployment/request assignment. Egress never
receives long-lived object-storage credentials and re-checks the declared size and SHA-256. Treat Control access,
signed URLs in logs, and stored response download authorization as sensitive. Receipt expiry is retention, not a
substitute for storage encryption or secure deletion guarantees.

Runtime administration is deployment-scoped authorization, not RBAC. Anyone holding the admin token can change all
deployment policy, control every worker, view active request IDs, cancel requests, and roll back configuration.

## Reporting vulnerabilities

Do not open a public issue for a suspected vulnerability. Follow the private reporting instructions in the
repository's `SECURITY.md`.
