---
name: update-straw-documentation
description: Write or update Straw's public-facing documentation under docs/public/ — user guides, API references, deployment/quickstart docs intended for publication on the web. Use when the user asks for public docs, a docs site page, API reference updates, or a quickstart/README aimed at external users.
---

# Update Straw Documentation

Public docs live under `docs/public/` (create it on first use, with an `index.md` table of contents). They target
external users of Straw — people who did not read the planning docs and never will. Everything else under `docs/`
(planning, tasks, handoffs, agent templates) is internal and is never published or linked from public docs.

## Source-of-truth rules

1. **Document only shipped, verified behavior.** Before describing any endpoint, flag, field, or behavior, verify it
   in current code (`cmd/`, `internal/`, `migrations/`, `deploy/docker/`) — not in planning docs. Planning docs state
   intent; several planned P0 surfaces shipped late or narrower than specced (deny-rule taxonomy, pool capability
   fields). If a planning doc and the code disagree, the code is what users get: document the code, and flag the
   divergence to the user separately instead of publishing the aspiration.
2. **Phase-gate the content.** Features from open SpecKit tasks, legacy task archives (`docs/tasks/*`), or later
   phases (`docs/planning/02-phase-boundaries.md`) do not appear in public docs — not even as "coming soon" unless
   the user explicitly asks for a roadmap page.
3. **Run every example.** curl/API examples must be executed against the local compose stack
   (`deploy/docker/README.md` has bootstrap/startup) before publication; paste real (sanitized) responses. An
   example you did not run is a guess wearing a code fence.
4. **No internal leakage.** Public docs never reference task numbers, handoffs, audits, agent workflows, internal
   file paths, or `docs/planning/*`. They never contain credentials, peppers, example API keys that look real
   (use `sk_example_...`-style placeholders), or internal hostnames.

## What to cover (typical pages)

- `index.md` — what Straw is (proxy control plane + egress workers), current capability summary, TOC.
- `quickstart.md` — compose up, bootstrap the first platform key, create a tenant + tenant key, send the first
  request via `POST /api/v1/requests`, read the response envelope.
- `api/` — one page per surface: requests, config resources (`/api/v1/config/...`: tenants, routing rules, deny
  rules, injection policies, fingerprint profiles, executor pools, changes), admin (`/api/v1/admin/...`: workers,
  request cancel), auth model (API keys, scopes, roles). Document request/response shapes from the actual handlers,
  RBAC per endpoint, and the canonical ErrorResponse envelope with its error codes
  (`internal/control/errors.go` is the registry).
- `operations.md` — health endpoints (`/healthz`, `/readyz`, `/metrics` on both binaries' ports), configuration env
  vars (`internal/config`), logging format, the P0 operational limits (buffered bodies, size caps, single-attempt
  dispatch, REST-only ingress) stated plainly as current limitations.

## Style

- Second person, present tense, task-oriented headings ("Create a tenant", not "Tenant creation").
- Every endpoint documented as: method + path, required role, request fields (name, type, required, meaning),
  example request, example response, error codes it can return.
- State limits and failure behavior honestly (rate-limit responses, quota rejections, what happens when a worker is
  unavailable). Users trust docs that admit limits.
- Keep pages self-contained; link between public pages only.

## Workflow

1. Verify the feature set in code (grep routes in `cmd/control/main.go` / `cmd/egress/main.go` — the route
   registrations are the authoritative public surface list).
2. Draft or update the page(s) under `docs/public/`.
3. Run the examples against the compose stack; paste sanitized output.
4. Update `docs/public/index.md` TOC.
5. Re-read as a stranger: does any sentence require internal context? Remove or explain it.
