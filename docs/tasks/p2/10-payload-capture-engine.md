# 10 - Payload Capture Engine

Status: not started

## Objective

Implement the non-mutating payload capture tee with storage-only redaction and bounded capture decisions.

## Required Planning Docs

- `docs/planning/19-payload-capture-p2.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/33-risks.md`

## Prerequisites

- Task 09 completed.

## Out of Scope

- Do not mutate forwarded request or response bytes.
- Do not add body regex/JSONPath redaction.
- Do not decompress bodies unless a later task explicitly owns it.

## Expected Files

- Create or modify: payload capture engine.
- Test: payload capture engine tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement capture decisions `NONE`, `METADATA_ONLY`, `HEADERS`, `BODY_TRUNCATED`, and bounded `BODY_FULL`.
- [ ] Tee request/response data without mutating forwarded bytes.
- [ ] Apply header redaction to stored copies.
- [ ] Implement raw-body truncation for stored copies.
- [ ] Treat compressed bodies as raw compressed bytes or metadata-only according to policy.
- [ ] Enforce capture size limits and disabled-by-default behavior.
- [ ] Add tests for every capture decision, non-mutation, redaction, truncation, compressed body policy, and limit
      enforcement.
- [ ] Run focused capture engine tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused payload capture engine tests.
- `make check`

## Acceptance Criteria

- Capture never changes forwarded traffic.
- Full capture is still bounded.
- Baseline P2 supports header redaction and raw-body truncation only.

## Handoff Notes

- Document compression behavior and unsupported redaction modes.

## Stop Conditions

- Stop before adding live body mutation.
- Stop if a deferral would have no owning task file.
