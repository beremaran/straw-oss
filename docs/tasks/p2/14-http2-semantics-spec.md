# 14 - HTTP/2 Semantics Spec

Status: not started

## Objective

Specify HTTP/2 semantics before any outbound or ingress HTTP/2 implementation begins.

## Required Planning Docs

- `docs/planning/15-http-semantics.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/17-mitm-design-p2.md`

## Prerequisites

- P1 task 16 completed.

## Out of Scope

- Do not implement HTTP/2.
- Do not enable upstream connection reuse.
- Do not change P0 HTTP/1.1 behavior.

## Expected Files

- Create: `docs/planning/c-http2-semantics.md`
- Modify: `docs/planning/32-open-decisions.md` only if the spec records a decision proposal.

## Steps

- [ ] Read all required planning docs.
- [ ] Specify one `request_id` per HTTP/2 stream.
- [ ] Specify cancellation mapping.
- [ ] Specify HTTP/2 flow control interaction with NATS credit.
- [ ] Specify pseudo-header normalization.
- [ ] Specify trailer behavior.
- [ ] Specify connection-level error fanout.
- [ ] Specify MITM ALPN behavior.
- [ ] Specify HTTP/1.1 to HTTP/2 downgrade rules.
- [ ] Add required HTTP/2 test rows.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Documentation/spec review.
- `make check`

## Acceptance Criteria

- All eight HTTP/2 prerequisites from Section 15 are resolved in a written spec.
- No HTTP/2 code is enabled.

## Handoff Notes

- Link the spec and name tasks 15/16 as consumers.

## Stop Conditions

- Stop before implementing HTTP/2.
- Stop if any Section 15 prerequisite remains undefined.
- Stop if a deferral would have no owning task file.
