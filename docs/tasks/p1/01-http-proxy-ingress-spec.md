# 01 - HTTP Proxy Ingress Spec

Status: done

## Objective

Specify the P1 HTTP forward proxy contract before implementation: proxy authentication, raw-socket error rendering,
request parsing, response streaming, and backpressure.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md`
- `docs/planning/07-public-api-surface.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/09-canonical-request-lifecycle.md`
- `docs/planning/28-deployment.md`

## Prerequisites

- P0 completed.

## Out of Scope

- Do not implement the proxy listener.
- Do not implement CONNECT.
- Do not add SDK, CLI, or UI work.

## Expected Files

- Create: `docs/planning/b-http-proxy-ingress.md`
- Modify: `docs/planning/32-open-decisions.md` only if the spec records a decision proposal.

## Steps

- [x] Read all required planning docs.
- [x] Define how API keys are carried on proxy requests and ensure `Proxy-Authorization` is never forwarded upstream.
- [x] Define how proxy client requests map into the existing decoded internal request model.
- [x] Define public error rendering on a raw proxy socket without the REST JSON success envelope.
- [x] Define Control-to-client response streaming and backpressure behavior for proxy responses.
- [x] Define trailer handling and what is dropped or metadata-captured when the client protocol cannot carry trailers.
- [x] Add the minimum test matrix rows needed before implementation starts.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Documentation/spec review.
- `make check`

## Acceptance Criteria

- The spec removes the proxy-authentication and raw-response ambiguity called out by the audit.
- The spec names all implementation tasks that will consume it.
- No production code is changed.

## Handoff Notes

- Link the new planning appendix.
- List any new open decisions.

## Stop Conditions

- Stop before implementing listener code.
- Stop if proxy response framing remains ambiguous.
- Stop if a deferral would have no owning task file.
